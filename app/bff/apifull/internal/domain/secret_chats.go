package domain

import (
	"bytes"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	SecretChatWaiting   = "waiting"
	SecretChatActive    = "active"
	SecretChatDiscarded = "discarded"
)

var (
	ErrSecretChatNotFound        = errors.New("secret chat not found")
	ErrSecretChatForbidden       = errors.New("secret chat access denied")
	ErrSecretChatConflict        = errors.New("secret chat id conflict")
	ErrSecretChatAlreadyAccepted = errors.New("secret chat already accepted")
	ErrSecretChatDeclined        = errors.New("secret chat declined")
	ErrSecretKeyReplay           = errors.New("secret device key replay")
	ErrSecretMessageConflict     = errors.New("secret message random id conflict")
	ErrSecretQTSInvalid          = errors.New("secret message qts invalid")
)

type SecretChat struct {
	ID             int32
	AccessHash     int64
	AdminID        int64
	ParticipantID  int64
	State          string
	GA             []byte
	GB             []byte
	KeyFingerprint int64
	CreatedAt      int32
	AcceptedAt     int32
	DiscardedAt    int32
	DiscardedBy    int64
	HistoryDeleted bool
}

type SecretFile struct {
	ID             int64
	AccessHash     int64
	Size           int64
	DCID           int32
	KeyFingerprint int32
}

type SecretMessage struct {
	ChatID      int32
	SenderID    int64
	RecipientID int64
	RandomID    int64
	QTS         int32
	Date        int32
	Data        []byte
	Service     bool
	File        *SecretFile
}

// SecretDeviceKey records a public key for one authenticated device. A user
// may have more than one active device, while a device may only advance its
// key epoch. Keeping this separate from the chat handshake lets a restarted
// process recover the device key history and reject stale replacements.
type SecretDeviceKey struct {
	ChatID      int32
	UserID      int64
	DeviceID    int64
	Epoch       int32
	PublicKey   []byte
	Fingerprint int64
	CreatedAt   int32
}

// SaveSecretDeviceKey durably binds a device/epoch to one public key. Retries
// with the exact same record are idempotent; a different key at the same or an
// older epoch is rejected so an old device cannot overwrite a newer key.
func SaveSecretDeviceKey(key SecretDeviceKey) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	// Auth key ids are signed int64 values on the wire; zero is the only
	// missing-device sentinel.
	if key.ChatID == 0 || key.UserID <= 0 || key.DeviceID == 0 || key.Epoch <= 0 || len(key.PublicKey) == 0 || len(key.PublicKey) > 256 {
		return ErrSecretKeyReplay
	}
	if key.CreatedAt == 0 {
		key.CreatedAt = int32(time.Now().Unix())
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = saveSecretDeviceKeyTx(tx, key); err != nil {
		return err
	}
	return tx.Commit()
}

// saveSecretDeviceKeyTx returns true when a new key row was inserted. An exact
// retry is successful but reports false, so callers can avoid treating a stale
// device retry as a newer cross-device key.
func saveSecretDeviceKeyTx(tx *sql.Tx, key SecretDeviceKey) (bool, error) {
	if key.CreatedAt == 0 {
		key.CreatedAt = int32(time.Now().Unix())
	}
	var adminID, participantID int64
	if err := tx.QueryRow(`SELECT admin_user_id, participant_user_id FROM apifull_secret_chat WHERE id=? FOR UPDATE`, key.ChatID).Scan(&adminID, &participantID); err != nil {
		if err == sql.ErrNoRows {
			return false, ErrSecretChatNotFound
		}
		return false, err
	}
	if key.UserID != adminID && key.UserID != participantID {
		return false, ErrSecretChatForbidden
	}
	var err error
	var maxEpoch int32
	if err = tx.QueryRow(`SELECT COALESCE(MAX(epoch),0) FROM apifull_secret_chat_device_key
		WHERE chat_id=? AND user_id=? AND device_id=?`, key.ChatID, key.UserID, key.DeviceID).Scan(&maxEpoch); err != nil {
		return false, err
	}
	var existingPublic []byte
	var existingFingerprint int64
	err = tx.QueryRow(`SELECT public_key, fingerprint FROM apifull_secret_chat_device_key
		WHERE chat_id=? AND user_id=? AND device_id=? AND epoch=? FOR UPDATE`,
		key.ChatID, key.UserID, key.DeviceID, key.Epoch).Scan(&existingPublic, &existingFingerprint)
	if err == nil {
		if bytes.Equal(existingPublic, key.PublicKey) && existingFingerprint == key.Fingerprint {
			return false, nil
		}
		return false, ErrSecretKeyReplay
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	if key.Epoch <= maxEpoch {
		return false, ErrSecretKeyReplay
	}
	_, err = tx.Exec(`INSERT INTO apifull_secret_chat_device_key
		(chat_id, user_id, device_id, epoch, public_key, fingerprint, created_at)
		VALUES (?,?,?,?,?,?,?)`, key.ChatID, key.UserID, key.DeviceID, key.Epoch,
		key.PublicKey, key.Fingerprint, key.CreatedAt)
	if err != nil {
		if duplicateKey(err) {
			return false, ErrSecretKeyReplay
		}
		return false, err
	}
	return true, nil
}

// CreateSecretChat stores the shared handshake. Repeating the same request is
// idempotent; reusing a chat id for different handshake data is rejected.
func CreateSecretChat(chat SecretChat) (SecretChat, bool, error) {
	if db == nil {
		return SecretChat{}, false, errors.New("domain PostgreSQL is not open")
	}
	if chat.ID == 0 || chat.AccessHash == 0 || chat.AdminID <= 0 || chat.ParticipantID <= 0 || chat.AdminID == chat.ParticipantID || len(chat.GA) == 0 {
		return SecretChat{}, false, ErrSecretChatConflict
	}
	if chat.CreatedAt == 0 {
		chat.CreatedAt = int32(time.Now().Unix())
	}
	chat.State = SecretChatWaiting

	tx, err := db.Begin()
	if err != nil {
		return SecretChat{}, false, err
	}
	defer tx.Rollback()

	existing, found, err := loadSecretChatTx(tx, chat.ID, true)
	if err != nil {
		return SecretChat{}, false, err
	}
	if found {
		if existing.State != SecretChatWaiting || existing.AdminID != chat.AdminID || existing.ParticipantID != chat.ParticipantID || !bytes.Equal(existing.GA, chat.GA) {
			return SecretChat{}, false, ErrSecretChatConflict
		}
		return existing, false, tx.Commit()
	}

	_, err = tx.Exec(`INSERT INTO apifull_secret_chat
		(id, access_hash, admin_user_id, participant_user_id, state, g_a, g_b, key_fingerprint,
		 created_at, accepted_at, discarded_at, discarded_by_user_id, history_deleted)
		VALUES (?,?,?,?,?,?,NULL,0,?,0,0,0,0)`,
		chat.ID, chat.AccessHash, chat.AdminID, chat.ParticipantID, chat.State, chat.GA, chat.CreatedAt)
	if err != nil {
		if duplicateKey(err) {
			return SecretChat{}, false, ErrSecretChatConflict
		}
		return SecretChat{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return SecretChat{}, false, err
	}
	return chat, true, nil
}

// AcceptSecretChat moves a waiting handshake to active. An identical retry is
// accepted so callers can recover after a lost response or update push.
func AcceptSecretChat(chatID int32, accessHash, userID int64, gb []byte, fingerprint int64) (SecretChat, bool, error) {
	return acceptSecretChatOnDevice(chatID, accessHash, userID, 0, 1, gb, fingerprint)
}

// AcceptSecretChatOnDevice is the device-aware handshake path. A participant
// can accept the same chat from another authenticated device; each device key
// is kept independently, while the chat row exposes the newest public value.
func AcceptSecretChatOnDevice(chatID int32, accessHash, userID, deviceID int64, epoch int32, gb []byte, fingerprint int64) (SecretChat, bool, error) {
	return acceptSecretChatOnDevice(chatID, accessHash, userID, deviceID, epoch, gb, fingerprint)
}

func acceptSecretChatOnDevice(chatID int32, accessHash, userID, deviceID int64, epoch int32, gb []byte, fingerprint int64) (SecretChat, bool, error) {
	tx, chat, err := lockSecretChat(chatID, accessHash, userID)
	if err != nil {
		return SecretChat{}, false, err
	}
	defer tx.Rollback()
	if chat.ParticipantID != userID {
		return SecretChat{}, false, ErrSecretChatForbidden
	}
	switch chat.State {
	case SecretChatDiscarded:
		return SecretChat{}, false, ErrSecretChatDeclined
	case SecretChatActive:
		if chat.KeyFingerprint == fingerprint && bytes.Equal(chat.GB, gb) {
			if deviceID != 0 {
				if epoch <= 0 {
					epoch = 1
				}
				if _, err = saveSecretDeviceKeyTx(tx, SecretDeviceKey{ChatID: chat.ID, UserID: userID, DeviceID: deviceID, Epoch: epoch, PublicKey: append([]byte(nil), gb...), Fingerprint: fingerprint}); err != nil {
					return SecretChat{}, false, err
				}
			}
			return chat, false, tx.Commit()
		}
		if deviceID == 0 || epoch <= 0 {
			return SecretChat{}, false, ErrSecretChatAlreadyAccepted
		}
		inserted, saveErr := saveSecretDeviceKeyTx(tx, SecretDeviceKey{ChatID: chat.ID, UserID: userID, DeviceID: deviceID, Epoch: epoch, PublicKey: append([]byte(nil), gb...), Fingerprint: fingerprint})
		if saveErr != nil {
			return SecretChat{}, false, saveErr
		}
		if !inserted {
			// The key was already persisted by this device. Keep the canonical
			// chat value unchanged so an older device retry cannot roll back a
			// newer device's public key.
			return chat, false, tx.Commit()
		}
		chat.GB = append([]byte(nil), gb...)
		chat.KeyFingerprint = fingerprint
		chat.AcceptedAt = int32(time.Now().Unix())
		if _, err = tx.Exec(`UPDATE apifull_secret_chat SET g_b=?, key_fingerprint=?, accepted_at=? WHERE id=?`, chat.GB, chat.KeyFingerprint, chat.AcceptedAt, chat.ID); err != nil {
			return SecretChat{}, false, err
		}
		if err = tx.Commit(); err != nil {
			return SecretChat{}, false, err
		}
		return chat, true, nil
	case SecretChatWaiting:
	default:
		return SecretChat{}, false, ErrSecretChatConflict
	}

	chat.GB = append([]byte(nil), gb...)
	chat.KeyFingerprint = fingerprint
	chat.State = SecretChatActive
	chat.AcceptedAt = int32(time.Now().Unix())
	if _, err = tx.Exec(`UPDATE apifull_secret_chat
		SET state=?, g_b=?, key_fingerprint=?, accepted_at=? WHERE id=?`,
		chat.State, chat.GB, chat.KeyFingerprint, chat.AcceptedAt, chat.ID); err != nil {
		return SecretChat{}, false, err
	}
	if deviceID != 0 {
		if epoch <= 0 {
			epoch = 1
		}
		if _, err = saveSecretDeviceKeyTx(tx, SecretDeviceKey{ChatID: chat.ID, UserID: userID, DeviceID: deviceID, Epoch: epoch, PublicKey: append([]byte(nil), gb...), Fingerprint: fingerprint}); err != nil {
			return SecretChat{}, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return SecretChat{}, false, err
	}
	return chat, true, nil
}

func AuthorizeSecretChat(chatID int32, accessHash, userID int64, requireActive bool) (SecretChat, error) {
	if db == nil {
		return SecretChat{}, errors.New("domain PostgreSQL is not open")
	}
	chat, found, err := loadSecretChatDB(db, chatID)
	if err != nil {
		return SecretChat{}, err
	}
	if !found {
		return SecretChat{}, ErrSecretChatNotFound
	}
	if chat.AccessHash != accessHash || (chat.AdminID != userID && chat.ParticipantID != userID) {
		return SecretChat{}, ErrSecretChatForbidden
	}
	if chat.State == SecretChatDiscarded {
		return SecretChat{}, ErrSecretChatDeclined
	}
	if requireActive && chat.State != SecretChatActive {
		return SecretChat{}, ErrSecretChatConflict
	}
	return chat, nil
}

func DiscardSecretChat(chatID int32, userID int64, deleteHistory bool) (SecretChat, int64, error) {
	if db == nil {
		return SecretChat{}, 0, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return SecretChat{}, 0, err
	}
	defer tx.Rollback()
	chat, found, err := loadSecretChatTx(tx, chatID, true)
	if err != nil {
		return SecretChat{}, 0, err
	}
	if !found {
		return SecretChat{}, 0, ErrSecretChatNotFound
	}
	if chat.AdminID != userID && chat.ParticipantID != userID {
		return SecretChat{}, 0, ErrSecretChatForbidden
	}
	peerID := secretPeer(chat, userID)
	if chat.State != SecretChatDiscarded {
		chat.State = SecretChatDiscarded
		chat.DiscardedAt = int32(time.Now().Unix())
		chat.DiscardedBy = userID
	}
	chat.HistoryDeleted = chat.HistoryDeleted || deleteHistory
	if _, err = tx.Exec(`UPDATE apifull_secret_chat
		SET state=?, discarded_at=?, discarded_by_user_id=?, history_deleted=? WHERE id=?`,
		chat.State, chat.DiscardedAt, chat.DiscardedBy, boolInt(chat.HistoryDeleted), chat.ID); err != nil {
		return SecretChat{}, 0, err
	}
	if chat.HistoryDeleted {
		if _, err = tx.Exec(`DELETE FROM apifull_secret_message WHERE chat_id=?`, chat.ID); err != nil {
			return SecretChat{}, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return SecretChat{}, 0, err
	}
	return chat, peerID, nil
}

// SaveSecretMessage allocates a recipient-scoped QTS and stores one canonical
// ciphertext. The same sender/random id and content returns the original row.
func SaveSecretMessage(chatID int32, accessHash, senderID, randomID int64, data []byte, service bool, file *SecretFile) (SecretMessage, bool, error) {
	if randomID == 0 {
		return SecretMessage{}, false, ErrSecretMessageConflict
	}
	tx, chat, err := lockSecretChat(chatID, accessHash, senderID)
	if err != nil {
		return SecretMessage{}, false, err
	}
	defer tx.Rollback()
	if chat.State == SecretChatDiscarded {
		return SecretMessage{}, false, ErrSecretChatDeclined
	}
	if chat.State != SecretChatActive {
		return SecretMessage{}, false, ErrSecretChatConflict
	}

	existing, found, err := loadSecretMessageTx(tx, senderID, randomID)
	if err != nil {
		return SecretMessage{}, false, err
	}
	if found {
		if existing.ChatID != chatID || existing.Service != service || !bytes.Equal(existing.Data, data) || !equalSecretFile(existing.File, file) {
			return SecretMessage{}, false, ErrSecretMessageConflict
		}
		return existing, false, tx.Commit()
	}

	recipientID := secretPeer(chat, senderID)
	if _, err = tx.Exec(`INSERT IGNORE INTO apifull_secret_user_state (user_id, last_qts, confirmed_qts) VALUES (?,0,0)`, recipientID); err != nil {
		return SecretMessage{}, false, err
	}
	var lastQTS int32
	if err = tx.QueryRow(`SELECT last_qts FROM apifull_secret_user_state WHERE user_id=? FOR UPDATE`, recipientID).Scan(&lastQTS); err != nil {
		return SecretMessage{}, false, err
	}
	if lastQTS == math.MaxInt32 {
		return SecretMessage{}, false, ErrSecretQTSInvalid
	}
	message := SecretMessage{
		ChatID: chatID, SenderID: senderID, RecipientID: recipientID, RandomID: randomID,
		QTS: lastQTS + 1, Date: int32(time.Now().Unix()), Data: append([]byte(nil), data...), Service: service,
	}
	if file != nil {
		copyFile := *file
		message.File = &copyFile
	}
	fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint := secretFileArgs(file)
	_, err = tx.Exec(`INSERT INTO apifull_secret_message
		(chat_id, sender_user_id, recipient_user_id, random_id, qts, date, encrypted_data, service,
		 file_id, file_access_hash, file_size, file_dc_id, file_key_fingerprint, acknowledged_at, read_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,0)`,
		message.ChatID, message.SenderID, message.RecipientID, message.RandomID, message.QTS, message.Date,
		message.Data, boolInt(message.Service), fileID, fileAccessHash, fileSize, fileDCID, fileFingerprint)
	if err != nil {
		if duplicateKey(err) {
			return SecretMessage{}, false, ErrSecretMessageConflict
		}
		return SecretMessage{}, false, err
	}
	if _, err = tx.Exec(`UPDATE apifull_secret_user_state SET last_qts=? WHERE user_id=?`, message.QTS, recipientID); err != nil {
		return SecretMessage{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return SecretMessage{}, false, err
	}
	return message, true, nil
}

func ConfirmSecretQueue(userID int64, maxQTS int32) ([]int64, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || maxQTS <= 0 {
		return nil, ErrSecretQTSInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var lastQTS, confirmedQTS int32
	err = tx.QueryRow(`SELECT last_qts, confirmed_qts FROM apifull_secret_user_state WHERE user_id=? FOR UPDATE`, userID).
		Scan(&lastQTS, &confirmedQTS)
	if err == sql.ErrNoRows || maxQTS > lastQTS {
		return nil, ErrSecretQTSInvalid
	}
	if err != nil {
		return nil, err
	}
	if maxQTS <= confirmedQTS {
		return []int64{}, tx.Commit()
	}
	rows, err := tx.Query(`SELECT random_id FROM apifull_secret_message
		WHERE recipient_user_id=? AND qts>? AND qts<=? ORDER BY qts`, userID, confirmedQTS, maxQTS)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	now := int32(time.Now().Unix())
	if _, err = tx.Exec(`UPDATE apifull_secret_message SET acknowledged_at=?
		WHERE recipient_user_id=? AND qts>? AND qts<=?`, now, userID, confirmedQTS, maxQTS); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE apifull_secret_user_state SET confirmed_qts=? WHERE user_id=?`, maxQTS, userID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

func ReadSecretHistory(chatID int32, accessHash, userID int64, maxDate int32) (SecretChat, int64, error) {
	tx, chat, err := lockSecretChat(chatID, accessHash, userID)
	if err != nil {
		return SecretChat{}, 0, err
	}
	defer tx.Rollback()
	if chat.State == SecretChatDiscarded {
		return SecretChat{}, 0, ErrSecretChatDeclined
	}
	if chat.State != SecretChatActive {
		return SecretChat{}, 0, ErrSecretChatConflict
	}
	if _, err = tx.Exec(`UPDATE apifull_secret_message SET read_at=?
		WHERE chat_id=? AND recipient_user_id=? AND date<=? AND read_at=0`,
		int32(time.Now().Unix()), chatID, userID, maxDate); err != nil {
		return SecretChat{}, 0, err
	}
	if err = tx.Commit(); err != nil {
		return SecretChat{}, 0, err
	}
	return chat, secretPeer(chat, userID), nil
}

func lockSecretChat(chatID int32, accessHash, userID int64) (*sql.Tx, SecretChat, error) {
	if db == nil {
		return nil, SecretChat{}, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, SecretChat{}, err
	}
	chat, found, err := loadSecretChatTx(tx, chatID, true)
	if err != nil {
		tx.Rollback()
		return nil, SecretChat{}, err
	}
	if !found {
		tx.Rollback()
		return nil, SecretChat{}, ErrSecretChatNotFound
	}
	if chat.AccessHash != accessHash || (chat.AdminID != userID && chat.ParticipantID != userID) {
		tx.Rollback()
		return nil, SecretChat{}, ErrSecretChatForbidden
	}
	return tx, chat, nil
}

type secretScanner interface {
	Scan(...any) error
}

func scanSecretChat(row secretScanner) (SecretChat, bool, error) {
	var chat SecretChat
	var historyDeleted int
	err := row.Scan(&chat.ID, &chat.AccessHash, &chat.AdminID, &chat.ParticipantID, &chat.State,
		&chat.GA, &chat.GB, &chat.KeyFingerprint, &chat.CreatedAt, &chat.AcceptedAt, &chat.DiscardedAt,
		&chat.DiscardedBy, &historyDeleted)
	if err == sql.ErrNoRows {
		return SecretChat{}, false, nil
	}
	if err != nil {
		return SecretChat{}, false, err
	}
	chat.HistoryDeleted = historyDeleted != 0
	return chat, true, nil
}

const secretChatColumns = `id, access_hash, admin_user_id, participant_user_id, state, g_a, g_b,
	key_fingerprint, created_at, accepted_at, discarded_at, discarded_by_user_id, history_deleted`

func loadSecretChatDB(q *sql.DB, chatID int32) (SecretChat, bool, error) {
	return scanSecretChat(q.QueryRow(`SELECT `+secretChatColumns+` FROM apifull_secret_chat WHERE id=?`, chatID))
}

func loadSecretChatTx(tx *sql.Tx, chatID int32, forUpdate bool) (SecretChat, bool, error) {
	query := `SELECT ` + secretChatColumns + ` FROM apifull_secret_chat WHERE id=?`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanSecretChat(tx.QueryRow(query, chatID))
}

func loadSecretMessageTx(tx *sql.Tx, senderID, randomID int64) (SecretMessage, bool, error) {
	var message SecretMessage
	var service int
	var fileID, fileAccessHash, fileSize sql.NullInt64
	var fileDCID, fileFingerprint sql.NullInt32
	err := tx.QueryRow(`SELECT chat_id, sender_user_id, recipient_user_id, random_id, qts, date,
		encrypted_data, service, file_id, file_access_hash, file_size, file_dc_id, file_key_fingerprint
		FROM apifull_secret_message WHERE sender_user_id=? AND random_id=? FOR UPDATE`, senderID, randomID).
		Scan(&message.ChatID, &message.SenderID, &message.RecipientID, &message.RandomID, &message.QTS,
			&message.Date, &message.Data, &service, &fileID, &fileAccessHash, &fileSize, &fileDCID, &fileFingerprint)
	if err == sql.ErrNoRows {
		return SecretMessage{}, false, nil
	}
	if err != nil {
		return SecretMessage{}, false, err
	}
	message.Service = service != 0
	if fileID.Valid {
		message.File = &SecretFile{ID: fileID.Int64, AccessHash: fileAccessHash.Int64, Size: fileSize.Int64,
			DCID: fileDCID.Int32, KeyFingerprint: fileFingerprint.Int32}
	}
	return message, true, nil
}

func secretPeer(chat SecretChat, userID int64) int64 {
	if chat.AdminID == userID {
		return chat.ParticipantID
	}
	return chat.AdminID
}

func secretFileArgs(file *SecretFile) (any, any, any, any, any) {
	if file == nil {
		return nil, nil, nil, nil, nil
	}
	return file.ID, file.AccessHash, file.Size, file.DCID, file.KeyFingerprint
}

func equalSecretFile(a, b *SecretFile) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func duplicateKey(err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == "23505"
}
