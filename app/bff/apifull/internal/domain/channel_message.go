package domain

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
)

var (
	ErrChannelMissing        = errors.New("channel missing")
	ErrNotCreator            = errors.New("not channel creator")
	ErrMessageMissing        = errors.New("channel message missing")
	ErrInvalidMessageID      = errors.New("invalid channel message id")
	ErrInvalidLocation       = errors.New("invalid channel location")
	ErrChannelWriteForbidden = errors.New("channel write forbidden")
	ErrDuplicateMessageID    = errors.New("duplicate channel message id")
	ErrRandomIDConflict      = errors.New("channel random id reused with different content")
	ErrRandomIDMissing       = errors.New("channel random id is required")
)

type ChannelMessage struct {
	ChannelID    int64
	MessageID    int32
	Pts          int32
	Sender       int64
	Date         int64
	Text         string
	Edited       bool
	EditedAt     int64
	Pinned       bool
	ReplyToMsgID int32
	ReplyToTopID int32
	Content      ChannelMessageContent
}

type ChannelMessageContent struct {
	Media          *mtproto.MessageMedia    `json:"media,omitempty"`
	Entities       []*mtproto.MessageEntity `json:"entities,omitempty"`
	ReplyMarkup    *mtproto.ReplyMarkup     `json:"reply_markup,omitempty"`
	GroupedID      int64                    `json:"grouped_id,omitempty"`
	AlbumRandomIDs []int64                  `json:"album_random_ids,omitempty"`
}

func (c ChannelMessageContent) empty() bool {
	return c.Media == nil && len(c.Entities) == 0 && c.ReplyMarkup == nil &&
		c.GroupedID == 0 && len(c.AlbumRandomIDs) == 0
}

// ChannelMessageInput is one item in an atomic channel album write.
// RequestFingerprint is calculated by the API boundary from the original
// InputMedia and the complete album random-id list.
type ChannelMessageInput struct {
	Text               string
	ReplyToMsgID       int32
	ReplyToTopID       int32
	RandomID           int64
	Content            ChannelMessageContent
	RequestFingerprint string
}

const (
	channelEventNew    = "new"
	channelEventEdit   = "edit"
	channelEventDelete = "delete"
	channelEventPin    = "pin"
)

func InsertChannelMessage(channelID, sender, date int64, text string) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, 0, 0, 0, ChannelMessageContent{}, "", 0)
}

// InsertChannelMessageWithReply persists the reply index used by channel
// discussions. The root is validated inside the same writer transaction.
func InsertChannelMessageWithReply(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, replyToMsgID, replyToTopID, 0, ChannelMessageContent{}, "", 0)
}

func InsertChannelMessageWithReplyAndRandomID(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32, randomID int64) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, replyToMsgID, replyToTopID, randomID, ChannelMessageContent{}, "", 0)
}

func InsertChannelMessageWithContentAndRandomID(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32, randomID int64, content ChannelMessageContent, requestFingerprint string) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, replyToMsgID, replyToTopID, randomID, content, requestFingerprint, 0)
}

func InsertChannelMessageWithContentAndRandomIDForDelivery(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32, randomID int64, content ChannelMessageContent, requestFingerprint string, excludeAuthKeyID int64) (ChannelMessage, error) {
	return insertChannelMessage(channelID, sender, date, text, replyToMsgID, replyToTopID, randomID, content, requestFingerprint, excludeAuthKeyID)
}

// InsertChannelMessagesBatch inserts every album item and its channel events
// in one transaction. A retry is accepted only when every item is already
// present with the same request fingerprint and complete album random-id list.
func InsertChannelMessagesBatch(channelID, sender, date int64, inputs []ChannelMessageInput) ([]ChannelMessage, error) {
	return insertChannelMessagesBatch(channelID, sender, date, inputs, 0)
}

func InsertChannelMessagesBatchForDelivery(channelID, sender, date int64, inputs []ChannelMessageInput, excludeAuthKeyID int64) ([]ChannelMessage, error) {
	return insertChannelMessagesBatch(channelID, sender, date, inputs, excludeAuthKeyID)
}

func insertChannelMessagesBatch(channelID, sender, date int64, inputs []ChannelMessageInput, excludeAuthKeyID int64) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if len(inputs) == 0 {
		return nil, ErrInvalidMessageID
	}
	if date == 0 {
		date = time.Now().Unix()
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = validateChannelMessageWriteTx(tx, channelID, sender); err != nil {
		return nil, err
	}

	type preparedInput struct {
		input       ChannelMessageInput
		contentJSON string
		hash        [32]byte
		row         ChannelMessage
	}
	prepared := make([]preparedInput, 0, len(inputs))
	seenRandomIDs := make(map[int64]struct{}, len(inputs))
	existingCount := 0
	for _, input := range inputs {
		if input.RandomID == 0 {
			return nil, ErrRandomIDMissing
		}
		if _, ok := seenRandomIDs[input.RandomID]; ok {
			return nil, ErrRandomIDConflict
		}
		seenRandomIDs[input.RandomID] = struct{}{}
		if input.ReplyToMsgID < 0 || input.ReplyToTopID < 0 {
			return nil, ErrInvalidMessageID
		}
		if input.ReplyToTopID > 0 && input.ReplyToMsgID == 0 {
			input.ReplyToMsgID = input.ReplyToTopID
		}
		contentJSON, encodeErr := encodeChannelMessageContent(input.Content)
		if encodeErr != nil {
			return nil, encodeErr
		}
		requestData, marshalErr := json.Marshal(struct {
			Text               string
			ReplyToMsgID       int32
			ReplyToTopID       int32
			RequestFingerprint string
		}{input.Text, input.ReplyToMsgID, input.ReplyToTopID, input.RequestFingerprint})
		if marshalErr != nil {
			return nil, marshalErr
		}
		preparedRow := preparedInput{input: input, contentJSON: contentJSON, hash: sha256.Sum256(requestData)}
		var messageID, pts int32
		var storedHash []byte
		lookupErr := tx.QueryRow(`SELECT message_id, pts, request_hash FROM apifull_channel_message_request
			WHERE channel_id=? AND sender_user_id=? AND random_id=? FOR UPDATE`, channelID, sender, input.RandomID).
			Scan(&messageID, &pts, &storedHash)
		if lookupErr == nil {
			if !bytes.Equal(storedHash, preparedRow.hash[:]) {
				return nil, ErrRandomIDConflict
			}
			var edited, pinned int
			var storedContent sql.NullString
			if lookupErr = tx.QueryRow(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
				FROM apifull_channel_message WHERE channel_id=? AND message_id=?`, channelID, messageID).
				Scan(&preparedRow.row.MessageID, &preparedRow.row.Sender, &preparedRow.row.Date, &preparedRow.row.Text, &edited, &preparedRow.row.EditedAt, &pinned, &preparedRow.row.ReplyToMsgID, &preparedRow.row.ReplyToTopID, &storedContent); lookupErr != nil {
				if lookupErr == sql.ErrNoRows {
					return nil, ErrMessageMissing
				}
				return nil, lookupErr
			}
			if err = decodeChannelMessageContent(storedContent.String, &preparedRow.row.Content); err != nil {
				return nil, err
			}
			if !sameAlbumRandomIDs(preparedRow.row.Content.AlbumRandomIDs, input.Content.AlbumRandomIDs) {
				return nil, ErrRandomIDConflict
			}
			preparedRow.row.ChannelID, preparedRow.row.Pts = channelID, pts
			preparedRow.row.Edited, preparedRow.row.Pinned = edited != 0, pinned != 0
			existingCount++
		} else if lookupErr != sql.ErrNoRows {
			return nil, lookupErr
		}
		prepared = append(prepared, preparedRow)
	}
	if existingCount != 0 {
		if existingCount != len(prepared) {
			return nil, ErrRandomIDConflict
		}
		rows := make([]ChannelMessage, len(prepared))
		for i := range prepared {
			rows[i] = prepared[i].row
		}
		if err = insertChannelDeliveryTx(tx, channelID, sender, excludeAuthKeyID, rows); err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return rows, nil
	}

	lastID, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return nil, err
	}
	rows := make([]ChannelMessage, 0, len(prepared))
	for i := range prepared {
		lastID++
		pts++
		input := prepared[i].input
		if _, err = tx.Exec(`INSERT INTO apifull_channel_message
			(channel_id, message_id, sender_user_id, date, message, edited, edited_at, reply_to_msg_id, reply_to_top_id, content_json)
			VALUES (?,?,?,?,?,0,0,?,?,?)`, channelID, lastID, sender, date, input.Text, input.ReplyToMsgID, input.ReplyToTopID, prepared[i].contentJSON); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(`INSERT INTO apifull_channel_message_request
			(channel_id, sender_user_id, random_id, message_id, pts, request_hash, created_at)
			VALUES (?,?,?,?,?,?,?)`, channelID, sender, input.RandomID, lastID, pts, prepared[i].hash[:], date); err != nil {
			return nil, err
		}
		if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventNew, []int32{lastID}, sender, date, input.Text, prepared[i].contentJSON, 0, false); err != nil {
			return nil, err
		}
		rows = append(rows, ChannelMessage{ChannelID: channelID, MessageID: lastID, Pts: pts, Sender: sender, Date: date,
			Text: input.Text, ReplyToMsgID: input.ReplyToMsgID, ReplyToTopID: input.ReplyToTopID, Content: input.Content})
	}
	if err = saveChannelMessageSequence(tx, channelID, lastID, pts); err != nil {
		return nil, err
	}
	if err = insertChannelDeliveryTx(tx, channelID, sender, excludeAuthKeyID, rows); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return rows, nil
}

func sameAlbumRandomIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func ValidateChannelMessageWrite(channelID, sender int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = validateChannelMessageWriteTx(tx, channelID, sender); err != nil {
		return err
	}
	return tx.Commit()
}

func validateChannelMessageWriteTx(tx *sql.Tx, channelID, sender int64) error {
	var creator int64
	var broadcast int
	err := tx.QueryRow(`SELECT creator_user_id, broadcast FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator, &broadcast)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if creator == sender {
		return nil
	}
	var rawRights, rawBanned string
	err = tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member
		WHERE channel_id=? AND user_id=?`, channelID, sender).Scan(&rawRights, &rawBanned)
	if err == sql.ErrNoRows {
		return ErrNotChannelMember
	}
	if err != nil {
		return err
	}
	if rawBanned != "" {
		var banned ChannelBannedRights
		if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
			return err
		}
		if banned.Active(time.Now().Unix()) && (banned.ViewMessages || banned.SendMessages || banned.SendPlain) {
			return ErrChannelWriteForbidden
		}
	}
	if broadcast != 0 {
		var rights ChannelAdminRights
		if rawRights == "" || json.Unmarshal([]byte(rawRights), &rights) != nil || !rights.PostMessages {
			return ErrChannelWriteForbidden
		}
	}
	return nil
}

func insertChannelMessage(channelID, sender, date int64, text string, replyToMsgID, replyToTopID int32, randomID int64, content ChannelMessageContent, requestFingerprint string, excludeAuthKeyID int64) (ChannelMessage, error) {
	var row ChannelMessage
	if db == nil {
		return row, errors.New("domain PostgreSQL is not open")
	}
	if date == 0 {
		date = time.Now().Unix()
	}
	tx, err := db.Begin()
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	if replyToMsgID < 0 || replyToTopID < 0 {
		return row, ErrInvalidMessageID
	}
	if replyToTopID > 0 && replyToMsgID == 0 {
		replyToMsgID = replyToTopID
	}
	if err = validateChannelMessageWriteTx(tx, channelID, sender); err != nil {
		return row, err
	}
	contentJSON, err := encodeChannelMessageContent(content)
	if err != nil {
		return row, err
	}
	var requestHash [32]byte
	if randomID != 0 {
		var requestData []byte
		var marshalErr error
		if requestFingerprint == "" {
			requestData, marshalErr = json.Marshal(struct {
				Text         string
				ReplyToMsgID int32
				ReplyToTopID int32
			}{text, replyToMsgID, replyToTopID})
		} else {
			requestData, marshalErr = json.Marshal(struct {
				Text               string
				ReplyToMsgID       int32
				ReplyToTopID       int32
				RequestFingerprint string
			}{text, replyToMsgID, replyToTopID, requestFingerprint})
		}
		if marshalErr != nil {
			return row, marshalErr
		}
		requestHash = sha256.Sum256(requestData)
		var messageID, pts int32
		var storedHash []byte
		lookupErr := tx.QueryRow(`SELECT message_id, pts, request_hash FROM apifull_channel_message_request
			WHERE channel_id=? AND sender_user_id=? AND random_id=?`, channelID, sender, randomID).Scan(&messageID, &pts, &storedHash)
		if lookupErr == nil {
			if !bytes.Equal(storedHash, requestHash[:]) {
				return row, ErrRandomIDConflict
			}
			var edited, pinned int
			var contentJSON sql.NullString
			lookupErr = tx.QueryRow(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
				FROM apifull_channel_message WHERE channel_id=? AND message_id=?`, channelID, messageID).
				Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinned, &row.ReplyToMsgID, &row.ReplyToTopID, &contentJSON)
			if lookupErr == sql.ErrNoRows {
				return row, ErrMessageMissing
			}
			if lookupErr != nil {
				return row, lookupErr
			}
			row.ChannelID, row.Pts = channelID, pts
			row.Edited, row.Pinned = edited != 0, pinned != 0
			if err = decodeChannelMessageContent(contentJSON.String, &row.Content); err != nil {
				return ChannelMessage{}, err
			}
			if err = insertChannelDeliveryTx(tx, channelID, sender, excludeAuthKeyID, []ChannelMessage{row}); err != nil {
				return ChannelMessage{}, err
			}
			if err = tx.Commit(); err != nil {
				return ChannelMessage{}, err
			}
			return row, nil
		}
		if lookupErr != sql.ErrNoRows {
			return row, lookupErr
		}
	}
	lastID, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return row, err
	}
	next := lastID + 1
	pts++
	if err = saveChannelMessageSequence(tx, channelID, next, pts); err != nil {
		return row, err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_channel_message
		(channel_id, message_id, sender_user_id, date, message, edited, edited_at, reply_to_msg_id, reply_to_top_id, content_json)
		VALUES (?,?,?,?,?,0,0,?,?,?)`, channelID, next, sender, date, text, replyToMsgID, replyToTopID, contentJSON); err != nil {
		return row, err
	}
	if randomID != 0 {
		if _, err = tx.Exec(`INSERT INTO apifull_channel_message_request
			(channel_id, sender_user_id, random_id, message_id, pts, request_hash, created_at)
			VALUES (?,?,?,?,?,?,?)`, channelID, sender, randomID, next, pts, requestHash[:], date); err != nil {
			return row, err
		}
	}
	if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventNew, []int32{next}, sender, date, text, contentJSON, 0, false); err != nil {
		return row, err
	}
	row = ChannelMessage{ChannelID: channelID, MessageID: next, Pts: pts, Sender: sender, Date: date,
		Text: text, ReplyToMsgID: replyToMsgID, ReplyToTopID: replyToTopID, Content: content}
	if err = insertChannelDeliveryTx(tx, channelID, sender, excludeAuthKeyID, []ChannelMessage{row}); err != nil {
		return ChannelMessage{}, err
	}
	if err = tx.Commit(); err != nil {
		return row, err
	}
	return row, nil
}

func encodeChannelMessageContent(content ChannelMessageContent) (string, error) {
	if content.empty() {
		return "", nil
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeChannelMessageContent(encoded string, content *ChannelMessageContent) error {
	if encoded == "" || encoded == "null" {
		return nil
	}
	return json.Unmarshal([]byte(encoded), content)
}

func UpdateChannelMessage(channelID, sender int64, messageID int32, text string) (ChannelMessage, error) {
	var row ChannelMessage
	if db == nil {
		return row, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return row, ErrChannelMissing
	}
	if err != nil {
		return row, err
	}
	if creator != sender {
		return row, ErrNotCreator
	}
	var edited, pinned int
	var contentJSON sql.NullString
	err = tx.QueryRow(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE channel_id=? AND message_id=? FOR UPDATE`, channelID, messageID).
		Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinned, &row.ReplyToMsgID, &row.ReplyToTopID, &contentJSON)
	if err == sql.ErrNoRows {
		return ChannelMessage{}, ErrMessageMissing
	}
	if err != nil {
		return ChannelMessage{}, err
	}
	if err = decodeChannelMessageContent(contentJSON.String, &row.Content); err != nil {
		return ChannelMessage{}, err
	}
	pts, err := advanceChannelMessagePTS(tx, channelID, 1)
	if err != nil {
		return ChannelMessage{}, err
	}
	editedAt := time.Now().Unix()
	if _, err = tx.Exec(`UPDATE apifull_channel_message SET message=?, edited=1, edited_at=? WHERE channel_id=? AND message_id=?`,
		text, editedAt, channelID, messageID); err != nil {
		return ChannelMessage{}, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventEdit, []int32{messageID}, row.Sender, row.Date, text, contentJSON.String, editedAt, pinned != 0); err != nil {
		return ChannelMessage{}, err
	}
	row.ChannelID = channelID
	row.Pts = pts
	row.Text = text
	row.Edited = true
	row.EditedAt = editedAt
	row.Pinned = pinned != 0
	if err = insertChannelDeliveryEventTx(tx, channelID, sender, 0, channelEventEdit, []ChannelMessage{row}, []int32{messageID}, row.Pinned, pts, pts); err != nil {
		return ChannelMessage{}, err
	}
	if err = tx.Commit(); err != nil {
		return ChannelMessage{}, err
	}
	return row, nil
}

func SetChannelMessagePinned(channelID, sender int64, messageID int32, pinned bool) (ChannelMessage, error) {
	var row ChannelMessage
	if db == nil {
		return row, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return row, err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return row, ErrChannelMissing
	}
	if err != nil {
		return row, err
	}
	if creator != sender {
		return row, ErrNotCreator
	}
	var edited, pinFlag int
	var contentJSON sql.NullString
	err = tx.QueryRow(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE channel_id=? AND message_id=? FOR UPDATE`, channelID, messageID).
		Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinFlag, &row.ReplyToMsgID, &row.ReplyToTopID, &contentJSON)
	if err == sql.ErrNoRows {
		return ChannelMessage{}, ErrMessageMissing
	}
	if err != nil {
		return ChannelMessage{}, err
	}
	if err = decodeChannelMessageContent(contentJSON.String, &row.Content); err != nil {
		return ChannelMessage{}, err
	}
	flag := 0
	if pinned {
		flag = 1
	}
	if _, err = tx.Exec(`UPDATE apifull_channel_message SET pinned=? WHERE channel_id=? AND message_id=?`,
		flag, channelID, messageID); err != nil {
		return ChannelMessage{}, err
	}
	pts, err := advanceChannelMessagePTS(tx, channelID, 1)
	if err != nil {
		return ChannelMessage{}, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, 1, channelEventPin, []int32{messageID}, row.Sender, row.Date, row.Text, contentJSON.String, row.EditedAt, pinned); err != nil {
		return ChannelMessage{}, err
	}
	row.ChannelID = channelID
	row.Pts = pts
	row.Edited = edited != 0
	row.Pinned = pinned
	if err = insertChannelDeliveryEventTx(tx, channelID, sender, 0, channelEventPin, []ChannelMessage{row}, []int32{messageID}, pinned, pts, pts); err != nil {
		return ChannelMessage{}, err
	}
	if err = tx.Commit(); err != nil {
		return ChannelMessage{}, err
	}
	return row, nil
}

// ClearChannelMessagePins removes every pinned message in a channel as one
// atomic creator-authorized update and returns the affected message IDs.
func ClearChannelMessagePins(channelID, sender int64) ([]int32, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	_, err = lockChannelMessageWriter(tx, channelID, sender)
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(`SELECT message_id FROM apifull_channel_message WHERE channel_id=? AND pinned=1 ORDER BY message_id FOR UPDATE`, channelID)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()

	pts, err := advanceChannelMessagePTS(tx, channelID, int32(len(ids)))
	if err != nil {
		return nil, 0, err
	}
	if len(ids) > 0 {
		if _, err = tx.Exec(`UPDATE apifull_channel_message SET pinned=0 WHERE channel_id=? AND pinned=1`, channelID); err != nil {
			return nil, 0, err
		}
		if err = appendChannelEventTx(tx, channelID, pts, int32(len(ids)), channelEventPin, ids, 0, 0, "", "", 0, false); err != nil {
			return nil, 0, err
		}
		if err = insertChannelDeliveryEventTx(tx, channelID, sender, 0, channelEventPin, nil, ids, false, pts-int32(len(ids))+1, pts); err != nil {
			return nil, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return ids, pts, nil
}

func DeleteChannelMessages(channelID, sender int64, ids []int32) ([]int32, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain PostgreSQL is not open")
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, 0, ErrInvalidMessageID
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	pts, err := lockChannelMessageWriter(tx, channelID, sender)
	if err != nil {
		return nil, 0, err
	}
	deleted := []int32{}
	if len(ids) > 0 {
		deleted, err = selectChannelMessageIDs(tx, channelID, ids, 0)
		if err != nil {
			return nil, 0, err
		}
	}
	if err = deleteChannelMessagesTx(tx, channelID, deleted); err != nil {
		return nil, 0, err
	}
	pts, err = advanceChannelMessagePTS(tx, channelID, int32(len(deleted)))
	if err != nil {
		return nil, 0, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, int32(len(deleted)), channelEventDelete, deleted, 0, 0, "", "", 0, false); err != nil {
		return nil, 0, err
	}
	if len(deleted) > 0 {
		if err = insertChannelDeliveryEventTx(tx, channelID, sender, 0, channelEventDelete, nil, deleted, false, pts-int32(len(deleted))+1, pts); err != nil {
			return nil, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return deleted, pts, nil
}

func DeleteChannelHistory(channelID, sender int64, maxID int32) ([]int32, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain PostgreSQL is not open")
	}
	if maxID < 0 {
		return nil, 0, ErrInvalidMessageID
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	pts, err := lockChannelMessageWriter(tx, channelID, sender)
	if err != nil {
		return nil, 0, err
	}
	deleted, err := selectChannelMessageIDs(tx, channelID, nil, maxID)
	if err != nil {
		return nil, 0, err
	}
	if err = deleteChannelMessagesTx(tx, channelID, deleted); err != nil {
		return nil, 0, err
	}
	pts, err = advanceChannelMessagePTS(tx, channelID, int32(len(deleted)))
	if err != nil {
		return nil, 0, err
	}
	if err = appendChannelEventTx(tx, channelID, pts, int32(len(deleted)), channelEventDelete, deleted, 0, 0, "", "", 0, false); err != nil {
		return nil, 0, err
	}
	if len(deleted) > 0 {
		if err = insertChannelDeliveryEventTx(tx, channelID, sender, 0, channelEventDelete, nil, deleted, false, pts-int32(len(deleted))+1, pts); err != nil {
			return nil, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return deleted, pts, nil
}

func DeleteChannelParticipantHistory(channelID, actorID, participantID int64) (int32, int32, error) {
	if db == nil {
		return 0, 0, errors.New("domain PostgreSQL is not open")
	}
	if participantID <= 0 {
		return 0, 0, ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if err = lockChannelMessageDeletePermission(tx, channelID, actorID); err != nil {
		return 0, 0, err
	}
	if _, _, err = loadChannelMessageSequence(tx, channelID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_channel_message_content_read r
		WHERE EXISTS (SELECT 1 FROM apifull_channel_message m
			WHERE m.channel_id=r.channel_id AND m.message_id=r.message_id
			AND m.channel_id=? AND m.sender_user_id=?)`, channelID, participantID); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_channel_message_hidden h
		WHERE EXISTS (SELECT 1 FROM apifull_channel_message m
			WHERE m.channel_id=h.channel_id AND m.message_id=h.message_id
			AND m.channel_id=? AND m.sender_user_id=?)`, channelID, participantID); err != nil {
		return 0, 0, err
	}
	deleted, err := selectChannelMessageIDsBySender(tx, channelID, participantID)
	if err != nil {
		return 0, 0, err
	}
	result, err := tx.Exec(`DELETE FROM apifull_channel_message WHERE channel_id=? AND sender_user_id=?`, channelID, participantID)
	if err != nil {
		return 0, 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	ptsCount := int32(affected)
	pts, err := advanceChannelMessagePTS(tx, channelID, ptsCount)
	if err != nil {
		return 0, 0, err
	}
	if len(deleted) > 0 {
		if err = appendChannelEventTx(tx, channelID, pts, ptsCount, channelEventDelete, deleted, participantID, 0, "", "", 0, false); err != nil {
			return 0, 0, err
		}
		if err = insertChannelDeliveryEventTx(tx, channelID, actorID, 0, channelEventDelete, nil, deleted, false, pts-ptsCount+1, pts); err != nil {
			return 0, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return pts, ptsCount, nil
}

func lockChannelMessageDeletePermission(tx *sql.Tx, channelID, actorID int64) error {
	var creatorID int64
	err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creatorID)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if actorID == creatorID {
		return nil
	}
	var rawRights string
	err = tx.QueryRow(`SELECT admin_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, actorID).Scan(&rawRights)
	if err == sql.ErrNoRows {
		return ErrNotCreator
	}
	if err != nil {
		return err
	}
	var rights ChannelAdminRights
	if rawRights == "" || json.Unmarshal([]byte(rawRights), &rights) != nil || !rights.DeleteMessages {
		return ErrNotCreator
	}
	return nil
}

func HideChannelHistory(userID, channelID int64, maxID int32) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	if maxID < 0 {
		return 0, ErrInvalidMessageID
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var channelExists int64
	err = tx.QueryRow(`SELECT id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&channelExists)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	query := `INSERT INTO apifull_channel_message_hidden (user_id, channel_id, message_id)
		SELECT $1, channel_id, message_id FROM apifull_channel_message WHERE channel_id=$2`
	args := []any{userID, channelID}
	if maxID > 0 {
		query += ` AND message_id<=$3`
		args = append(args, maxID)
	}
	query += ` ON CONFLICT (user_id, channel_id, message_id) DO NOTHING`
	result, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int32(affected), nil
}

func lockChannelMessageWriter(tx *sql.Tx, channelID, sender int64) (int32, error) {
	var creator int64
	err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	if creator != sender {
		return 0, ErrNotCreator
	}
	_, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return 0, err
	}
	return pts, nil
}

func loadChannelMessageSequence(tx *sql.Tx, channelID int64) (int32, int32, error) {
	var lastID, pts int32
	err := tx.QueryRow(`SELECT last_message_id, pts FROM apifull_channel_message_seq WHERE channel_id=? FOR UPDATE`, channelID).Scan(&lastID, &pts)
	if err == nil {
		return lastID, pts, nil
	}
	if err != sql.ErrNoRows {
		return 0, 0, err
	}
	if err = tx.QueryRow(`SELECT COALESCE(MAX(message_id),0) FROM apifull_channel_message WHERE channel_id=?`, channelID).Scan(&lastID); err != nil {
		return 0, 0, err
	}
	pts = lastID
	if _, err = tx.Exec(`INSERT INTO apifull_channel_message_seq (channel_id, last_message_id, pts) VALUES (?,?,?)`, channelID, lastID, pts); err != nil {
		return 0, 0, err
	}
	return lastID, pts, nil
}

func saveChannelMessageSequence(tx *sql.Tx, channelID int64, lastID, pts int32) error {
	_, err := tx.Exec(`UPDATE apifull_channel_message_seq SET last_message_id=?, pts=? WHERE channel_id=?`, lastID, pts, channelID)
	return err
}

func advanceChannelMessagePTS(tx *sql.Tx, channelID int64, count int32) (int32, error) {
	lastID, pts, err := loadChannelMessageSequence(tx, channelID)
	if err != nil {
		return 0, err
	}
	pts += count
	if err = saveChannelMessageSequence(tx, channelID, lastID, pts); err != nil {
		return 0, err
	}
	return pts, nil
}

func appendChannelEventTx(tx *sql.Tx, channelID int64, pts, ptsCount int32, eventType string, messageIDs []int32, sender, date int64, text, contentJSON string, editedAt int64, pinned bool) error {
	if pts <= 0 || ptsCount <= 0 || eventType == "" {
		return nil
	}
	ids, err := json.Marshal(messageIDs)
	if err != nil {
		return err
	}
	pin := 0
	if pinned {
		pin = 1
	}
	_, err = tx.Exec(`INSERT INTO apifull_channel_event
		(channel_id, pts, pts_count, event_type, message_ids, sender_user_id, date, message, content_json, edited_at, pinned)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, channelID, pts, ptsCount, eventType, ids, sender, date, text, contentJSON, editedAt, pin)
	return err
}

func selectChannelMessageIDsBySender(tx *sql.Tx, channelID, sender int64) ([]int32, error) {
	rows, err := tx.Query(`SELECT message_id FROM apifull_channel_message WHERE channel_id=? AND sender_user_id=? ORDER BY message_id`, channelID, sender)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func selectChannelMessageIDs(tx *sql.Tx, channelID int64, ids []int32, maxID int32) ([]int32, error) {
	query := `SELECT message_id FROM apifull_channel_message WHERE channel_id=?`
	args := []any{channelID}
	if len(ids) > 0 {
		holders := make([]string, len(ids))
		for i, id := range ids {
			holders[i] = "?"
			args = append(args, id)
		}
		query += ` AND message_id IN (` + strings.Join(holders, ",") + `)`
	} else if maxID > 0 {
		query += ` AND message_id<=?`
		args = append(args, maxID)
	}
	query += ` ORDER BY message_id`
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deleted := make([]int32, 0, len(ids))
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		deleted = append(deleted, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return deleted, nil
}

func deleteChannelMessagesTx(tx *sql.Tx, channelID int64, ids []int32) error {
	if len(ids) == 0 {
		return nil
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	in := strings.Join(holders, ",")
	if _, err := tx.Exec(`DELETE FROM apifull_channel_message WHERE channel_id=? AND message_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM apifull_channel_message_hidden WHERE channel_id=? AND message_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM apifull_channel_message_content_read WHERE channel_id=? AND message_id IN (`+in+`)`, args...); err != nil {
		return err
	}
	return nil
}

func ListChannelMessages(userID, channelID int64, beforeID, limit int32) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	q := `SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE channel_id=? AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)`
	args := []any{channelID, userID}
	if beforeID > 0 {
		q += ` AND message_id<?`
		args = append(args, beforeID)
	}
	q += ` ORDER BY message_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelMessages(channelID, rows)
}

func ListChannelMessagesRange(userID, channelID int64, minID, maxID, limit int32) ([]ChannelMessage, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	where := `channel_id=? AND NOT EXISTS (
		SELECT 1 FROM apifull_channel_message_hidden h
		WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
		AND h.message_id=apifull_channel_message.message_id)`
	args := []any{channelID, userID}
	if minID > 0 {
		where += ` AND message_id>?`
		args = append(args, minID)
	}
	if maxID > 0 {
		where += ` AND message_id<?`
		args = append(args, maxID)
	}
	var count int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message WHERE `+where, args...).Scan(&count); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any(nil), args...), limit)
	rows, err := db.Query(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE `+where+` ORDER BY message_id DESC LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result, err := scanChannelMessages(channelID, rows)
	return result, count, err
}

func ListPinnedChannelMessages(userID, channelID int64, limit int32) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := db.Query(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE channel_id=? AND pinned=1 AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)
		ORDER BY message_id DESC LIMIT ?`, channelID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelMessages(channelID, rows)
}

func ChannelMessagesByID(userID, channelID int64, ids []int32) ([]ChannelMessage, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if len(ids) == 0 {
		return []ChannelMessage{}, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	args = append(args, userID)
	rows, err := db.Query(`SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE channel_id=? AND message_id IN (`+strings.Join(holders, ",")+`) AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelMessages(channelID, rows)
}

// ChannelMessageAuthor resolves the author using the channel-native message ID.
// The legacy channels tables remain authoritative for channels created by the
// core channel service; APIFull owns channels that exist only in its own store.
func ChannelMessageAuthor(userID, channelID, accessHash int64, messageID int32) (int64, bool, error) {
	if db == nil {
		return 0, false, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || channelID <= 0 || accessHash == 0 || messageID <= 0 {
		return 0, false, nil
	}

	var legacyAccessHash, creatorID int64
	var legacyDeleted int
	err := db.QueryRow(`SELECT access_hash, creator_user_id, deleted FROM channels WHERE id=?`, channelID).
		Scan(&legacyAccessHash, &creatorID, &legacyDeleted)
	if undefinedTable(err) {
		// The core channel service is optional on a fresh PostgreSQL install.
		// Continue with the APIFull-owned projection below.
		err = sql.ErrNoRows
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, false, err
	}
	if err == nil {
		if legacyDeleted != 0 || legacyAccessHash != accessHash {
			return 0, false, ErrChannelMissing
		}
		var member bool
		if creatorID != userID {
			if err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM channel_participants
				WHERE channel_id=? AND user_id=? AND state=0 AND left_at=0)`, channelID, userID).Scan(&member); err != nil {
				if undefinedTable(err) {
					return 0, false, ErrNotChannelMember
				}
				return 0, false, err
			}
			if !member {
				return 0, false, ErrNotChannelMember
			}
		}
		// message_data is JSON in the legacy schema. Read the document and
		// decode it in Go so the query remains valid on PostgreSQL (the old
		// JSON_UNQUOTE/JSON_EXTRACT functions are MySQL-only).
		var rawMessage []byte
		err = db.QueryRow(`SELECT message_data
			FROM channel_messages WHERE channel_id=? AND message_id=? AND deleted=0`, channelID, messageID).
			Scan(&rawMessage)
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		if undefinedTable(err) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, err
		}
		var messageData struct {
			FromID struct {
				PredicateName string `json:"predicate_name"`
				UserID        int64  `json:"user_id"`
			} `json:"from_id"`
		}
		if err = json.Unmarshal(rawMessage, &messageData); err != nil {
			return 0, false, err
		}
		fromPredicate := messageData.FromID.PredicateName
		authorID := messageData.FromID.UserID
		if fromPredicate != "peerUser" || authorID <= 0 {
			return 0, true, nil
		}
		return authorID, true, nil
	}

	channel, ok, err := LoadChannel(channelID)
	if err != nil {
		return 0, false, err
	}
	if !ok || channel.AccessHash != accessHash {
		return 0, false, ErrChannelMissing
	}
	member, err := ChannelIsMember(channelID, userID)
	if err != nil {
		return 0, false, err
	}
	if !member {
		return 0, false, ErrNotChannelMember
	}
	rows, err := ChannelMessagesByID(userID, channelID, []int32{messageID})
	if err != nil {
		return 0, false, err
	}
	if len(rows) == 0 {
		return 0, false, nil
	}
	return rows[0].Sender, true, nil
}

func likeContains(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 2)
	b.WriteByte('%')
	for _, r := range q {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func SearchChannelMessages(userID, channelID int64, q string, sender int64, beforeID, addOffset, minDate, maxDate, minID, maxID, limit int32) ([]ChannelMessage, int32, error) {
	if db == nil {
		return nil, 0, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	where := []string{"channel_id=?", `NOT EXISTS (
		SELECT 1 FROM apifull_channel_message_hidden h
		WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
		AND h.message_id=apifull_channel_message.message_id)`}
	args := []any{channelID, userID}
	if q != "" {
		where = append(where, `message LIKE ? ESCAPE '\\'`)
		args = append(args, likeContains(q))
	}
	if sender != 0 {
		where = append(where, "sender_user_id=?")
		args = append(args, sender)
	}
	if minID > 0 {
		where = append(where, "message_id>?")
		args = append(args, minID)
	}
	if maxID > 0 {
		where = append(where, "message_id<?")
		args = append(args, maxID)
	}
	if minDate > 0 {
		where = append(where, "date>=?")
		args = append(args, minDate)
	}
	if maxDate > 0 {
		where = append(where, "date<=?")
		args = append(args, maxDate)
	}
	clause := strings.Join(where, " AND ")
	var total int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	pageArgs := append([]any{}, args...)
	if beforeID > 0 {
		clause += " AND message_id<?"
		pageArgs = append(pageArgs, beforeID)
	}
	pageArgs = append(pageArgs, limit)
	query := `SELECT message_id, sender_user_id, date, message, edited, edited_at, pinned, reply_to_msg_id, reply_to_top_id, COALESCE(content_json,'')
		FROM apifull_channel_message WHERE ` + clause + ` ORDER BY message_id DESC LIMIT ?`
	if addOffset > 0 {
		query += ` OFFSET ?`
		pageArgs = append(pageArgs, addOffset)
	}
	rows, err := db.Query(query, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, err := scanChannelMessages(channelID, rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func TopChannelMessage(channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	var id sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(message_id) FROM apifull_channel_message WHERE channel_id=?`, channelID).Scan(&id); err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return int32(id.Int64), nil
}

func ChannelMessagePTS(channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	var pts int32
	err := db.QueryRow(`SELECT pts FROM apifull_channel_message_seq WHERE channel_id=?`, channelID).Scan(&pts)
	if err == sql.ErrNoRows {
		return TopChannelMessage(channelID)
	}
	return pts, err
}

func MarkChannelReadHistory(userID, channelID int64, maxID int32) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	if maxID <= 0 {
		return 0, ErrInvalidMessageID
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var channelExists int64
	err = tx.QueryRow(`SELECT id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&channelExists)
	if err == sql.ErrNoRows {
		return 0, ErrChannelMissing
	}
	if err != nil {
		return 0, err
	}
	var top int32
	if err = tx.QueryRow(`SELECT COALESCE(MAX(message_id),0) FROM apifull_channel_message WHERE channel_id=?`, channelID).Scan(&top); err != nil {
		return 0, err
	}
	if maxID > top {
		maxID = top
	}
	if _, err = tx.Exec(`INSERT INTO apifull_channel_read_state (user_id, channel_id, read_max_id)
		VALUES ($1,$2,$3) ON CONFLICT (user_id, channel_id) DO UPDATE
		SET read_max_id=GREATEST(apifull_channel_read_state.read_max_id, EXCLUDED.read_max_id)`,
		userID, channelID, maxID); err != nil {
		return 0, err
	}
	var readMax int32
	if err = tx.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		userID, channelID).Scan(&readMax); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return readMax, nil
}

// MarkChannelMessageContentsRead stores durable, per-user content-read
// receipts. Validation and all inserts happen in one transaction so an
// unknown message cannot result in a partial receipt batch.
func MarkChannelMessageContentsRead(userID, channelID, accessHash int64, ids []int32) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || channelID <= 0 || accessHash <= 0 {
		return ErrInvalidChannelAccessHash
	}
	if len(ids) == 0 {
		return ErrInvalidMessageID
	}
	seen := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return ErrInvalidMessageID
		}
		if _, ok := seen[id]; ok {
			return ErrDuplicateMessageID
		}
		seen[id] = struct{}{}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var storedAccessHash, creatorID int64
	err = tx.QueryRow(`SELECT access_hash, creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).
		Scan(&storedAccessHash, &creatorID)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if storedAccessHash != accessHash {
		return ErrInvalidChannelAccessHash
	}
	if userID != creatorID {
		var rawBanned string
		err = tx.QueryRow(`SELECT banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, userID).
			Scan(&rawBanned)
		if err == sql.ErrNoRows {
			return ErrNotChannelMember
		}
		if err != nil {
			return err
		}
		if rawBanned != "" {
			var banned ChannelBannedRights
			if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
				return err
			}
			if banned.Kicks(time.Now().Unix()) {
				return ErrNotChannelMember
			}
		}
	}

	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	rows, err := tx.Query(`SELECT message_id FROM apifull_channel_message WHERE channel_id=? AND message_id IN (`+strings.Join(holders, ",")+`) FOR UPDATE`, args...)
	if err != nil {
		return err
	}
	found := make(map[int32]struct{}, len(ids))
	for rows.Next() {
		var id int32
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		found[id] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(found) != len(ids) {
		return ErrMessageMissing
	}

	readAt := time.Now().Unix()
	for _, id := range ids {
		if _, err = tx.Exec(`INSERT INTO apifull_channel_message_content_read
			(user_id, channel_id, message_id, read_at) VALUES ($1,$2,$3,$4)
			ON CONFLICT (user_id, channel_id, message_id) DO NOTHING`, userID, channelID, id, readAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ChannelReadMaxID(userID, channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	var readMax int32
	err := db.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		userID, channelID).Scan(&readMax)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return readMax, err
}

// ChannelMessageViewCounts reports how many persisted channel read cursors
// include each requested message.
func ChannelMessageViewCounts(channelID int64, ids []int32) (map[int32]int32, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	counts := make(map[int32]int32, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}
	holders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, channelID)
	for i, id := range ids {
		holders[i] = "?"
		args = append(args, id)
	}
	rows, err := db.Query(`SELECT m.message_id, COUNT(r.user_id)
		FROM apifull_channel_message m
		LEFT JOIN apifull_channel_read_state r
			ON r.channel_id=m.channel_id AND r.read_max_id>=m.message_id
		WHERE m.channel_id=? AND m.message_id IN (`+strings.Join(holders, ",")+`)
		GROUP BY m.message_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, count int32
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

// ChannelMessageViewStats returns the persisted view count for every message
// in a channel. A read cursor contributes a view to all messages at or below
// its max id, matching ChannelMessageViewCounts and the Layer 229 message
// view semantics.
func ChannelMessageViewStats(channelID int64) (map[int32]int32, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT m.message_id, COUNT(r.user_id)
		FROM apifull_channel_message m
		LEFT JOIN apifull_channel_read_state r
			ON r.channel_id=m.channel_id AND r.read_max_id>=m.message_id
		WHERE m.channel_id=?
		GROUP BY m.message_id
		ORDER BY m.message_id`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int32]int32)
	for rows.Next() {
		var id, count int32
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func ChannelOutboxMaxID(userID, channelID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	var maxID int32
	err := db.QueryRow(`SELECT COALESCE(MAX(message_id),0) FROM apifull_channel_message
		WHERE channel_id=? AND sender_user_id=?`, channelID, userID).Scan(&maxID)
	return maxID, err
}

func scanChannelMessages(channelID int64, rows *sql.Rows) ([]ChannelMessage, error) {
	out := []ChannelMessage{}
	for rows.Next() {
		var row ChannelMessage
		var edited, pinned int
		var contentJSON sql.NullString
		if err := rows.Scan(&row.MessageID, &row.Sender, &row.Date, &row.Text, &edited, &row.EditedAt, &pinned, &row.ReplyToMsgID, &row.ReplyToTopID, &contentJSON); err != nil {
			return nil, err
		}
		if err := decodeChannelMessageContent(contentJSON.String, &row.Content); err != nil {
			return nil, err
		}
		row.ChannelID = channelID
		row.Edited = edited != 0
		row.Pinned = pinned != 0
		out = append(out, row)
	}
	return out, rows.Err()
}
