/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const channelDifferenceMaxLimit int32 = 100

type persistedChannelState struct {
	CreatorUserID int64 `db:"creator_user_id"`
	Pts           int32 `db:"pts"`
}

type persistedChannelMember struct {
	BannedRights string `db:"banned_rights"`
}

type persistedChannelMessage struct {
	ChannelID   int64  `db:"channel_id"`
	MessageID   int32  `db:"message_id"`
	Sender      int64  `db:"sender_user_id"`
	Date        int64  `db:"date"`
	Text        string `db:"message"`
	ContentJSON string `db:"content_json"`
	Edited      bool   `db:"edited"`
	EditedAt    int64  `db:"edited_at"`
	Pinned      bool   `db:"pinned"`
}

type persistedChannelEvent struct {
	ChannelID   int64  `db:"channel_id"`
	Pts         int32  `db:"pts"`
	PtsCount    int32  `db:"pts_count"`
	EventType   string `db:"event_type"`
	MessageIDs  string `db:"message_ids"`
	Sender      int64  `db:"sender_user_id"`
	Date        int64  `db:"date"`
	Text        string `db:"message"`
	ContentJSON string `db:"content_json"`
	EditedAt    int64  `db:"edited_at"`
	Pinned      bool   `db:"pinned"`
}

type channelDifferenceItem struct {
	pts     int32
	message *mtproto.Message
	update  *mtproto.Update
}

type channelBannedRights struct {
	ViewMessages bool `json:"view_messages"`
}

// UpdatesGetChannelDifferenceV2
// updates.getChannelDifferenceV2 auth_key_id:long user_id:long channel_id:long pts:int limit:int = ChannelDifference;
func (c *UpdatesCore) UpdatesGetChannelDifferenceV2(in *updates.TLUpdatesGetChannelDifferenceV2) (*updates.ChannelDifference, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetChannelId() <= 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	if in.GetUserId() <= 0 {
		return nil, mtproto.ErrActiveUserRequired
	}
	if in.GetPts() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetLimit() <= 0 || in.GetLimit() > channelDifferenceMaxLimit {
		return nil, mtproto.ErrLimitInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		state     persistedChannelState
		rows      []persistedChannelMessage
		events    []persistedChannelEvent
		hiddenIDs []int32
		err       error
	)
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Pool == nil {
		return nil, mtproto.ErrInternalServerError
	}
	state, rows, events, hiddenIDs, err = loadChannelDataPostgres(c.ctx, c.svcCtx.Dao.Postgres.Pool,
		in.GetChannelId(), in.GetUserId(), in.GetPts(), in.GetLimit())
	if err != nil {
		return nil, err
	}

	hidden := make(map[int32]struct{})
	for _, id := range hiddenIDs {
		hidden[id] = struct{}{}
	}

	items := make([]channelDifferenceItem, 0, len(rows)+len(events))
	seenMessages := make(map[int32]struct{}, len(events))
	for _, event := range events {
		ids := make([]int32, 0, 1)
		if event.MessageIDs != "" {
			if err = json.Unmarshal([]byte(event.MessageIDs), &ids); err != nil {
				return nil, err
			}
		}
		visibleIDs := make([]int32, 0, len(ids))
		for _, id := range ids {
			if _, ok := hidden[id]; !ok {
				visibleIDs = append(visibleIDs, id)
			}
		}
		if len(visibleIDs) == 0 && event.EventType != "delete" {
			continue
		}
		switch event.EventType {
		case "new":
			for _, id := range visibleIDs {
				seenMessages[id] = struct{}{}
				row := persistedChannelMessage{ChannelID: event.ChannelID, MessageID: id, Sender: event.Sender, Date: event.Date, Text: event.Text, ContentJSON: event.ContentJSON, Pinned: event.Pinned, EditedAt: event.EditedAt, Edited: event.EditedAt != 0}
				message, messageErr := persistedChannelMessageToMTProto(row)
				if messageErr != nil {
					return nil, messageErr
				}
				items = append(items, channelDifferenceItem{pts: event.Pts, message: message})
			}
		case "edit":
			if len(visibleIDs) == 0 {
				continue
			}
			row := persistedChannelMessage{ChannelID: event.ChannelID, MessageID: visibleIDs[0], Sender: event.Sender, Date: event.Date, Text: event.Text, ContentJSON: event.ContentJSON, Pinned: event.Pinned, Edited: true, EditedAt: event.EditedAt}
			message, messageErr := persistedChannelMessageToMTProto(row)
			if messageErr != nil {
				return nil, messageErr
			}
			items = append(items, channelDifferenceItem{pts: event.Pts, update: mtproto.MakeTLUpdateEditChannelMessage(&mtproto.Update{
				Message_MESSAGE: message,
				Pts_INT32:       event.Pts,
				PtsCount:        maxPtsCount(event.PtsCount),
			}).To_Update()})
		case "delete":
			items = append(items, channelDifferenceItem{pts: event.Pts, update: mtproto.MakeTLUpdateDeleteChannelMessages(&mtproto.Update{
				ChannelId: event.ChannelID,
				Messages:  visibleIDs,
				Pts_INT32: event.Pts,
				PtsCount:  maxPtsCount(event.PtsCount),
			}).To_Update()})
		case "pin":
			items = append(items, channelDifferenceItem{pts: event.Pts, update: mtproto.MakeTLUpdatePinnedChannelMessages(&mtproto.Update{
				Pinned:    event.Pinned,
				ChannelId: event.ChannelID,
				Messages:  visibleIDs,
				Pts_INT32: event.Pts,
				PtsCount:  maxPtsCount(event.PtsCount),
			}).To_Update()})
		}
	}
	for _, row := range rows {
		if _, ok := seenMessages[row.MessageID]; ok {
			continue
		}
		message, messageErr := persistedChannelMessageToMTProto(row)
		if messageErr != nil {
			return nil, messageErr
		}
		items = append(items, channelDifferenceItem{pts: row.MessageID, message: message})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].pts < items[j].pts })

	hasMore := len(items) > int(in.GetLimit())
	if hasMore {
		items = items[:in.GetLimit()]
	}
	newMessages := make([]*mtproto.Message, 0, len(items))
	otherUpdates := make([]*mtproto.Update, 0, len(items))
	pts := in.GetPts()
	for _, item := range items {
		if item.message != nil {
			newMessages = append(newMessages, item.message)
		}
		if item.update != nil {
			otherUpdates = append(otherUpdates, item.update)
		}
		if item.pts > pts {
			pts = item.pts
		}
	}
	if !hasMore && pts < state.Pts {
		pts = state.Pts
	}

	return updates.MakeTLChannelDifference(&updates.ChannelDifference{
		Final:        !hasMore,
		Pts:          pts,
		NewMessages:  newMessages,
		OtherUpdates: otherUpdates,
	}).To_ChannelDifference(), nil
}

func maxPtsCount(count int32) int32 {
	if count <= 0 {
		return 1
	}
	return count
}

func missingChannelEventTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "apifull_channel_event") && (strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "unknown table"))
}

// loadChannelDataPostgres mirrors the legacy channel-difference reads with
// PostgreSQL placeholders and pgx scans. The APIFull PostgreSQL schema is
// initialized by the APIFull domain store before this path is served.
func loadChannelDataPostgres(ctx context.Context, pool *pgxpool.Pool, channelID, userID int64, pts, limit int32) (
	persistedChannelState, []persistedChannelMessage, []persistedChannelEvent, []int32, error) {
	var state persistedChannelState
	if err := pool.QueryRow(ctx, `SELECT c.creator_user_id,
		COALESCE(s.pts, (SELECT COALESCE(MAX(m.message_id), 0) FROM apifull_channel_message m WHERE m.channel_id=c.id)) AS pts
		FROM apifull_channel c
		LEFT JOIN apifull_channel_message_seq s ON s.channel_id=c.id
		WHERE c.id = $1`, channelID).Scan(&state.CreatorUserID, &state.Pts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistedChannelState{}, nil, nil, nil, mtproto.ErrChannelInvalid
		}
		return persistedChannelState{}, nil, nil, nil, err
	}
	if err := ensureChannelMemberPostgres(ctx, pool, channelID, userID, state.CreatorUserID); err != nil {
		return persistedChannelState{}, nil, nil, nil, err
	}

	rows := make([]persistedChannelMessage, 0, limit+1)
	messageRows, err := pool.Query(ctx, `SELECT channel_id, message_id, sender_user_id, date, message,
		COALESCE(content_json, '') AS content_json, edited, edited_at, pinned
		FROM apifull_channel_message
		WHERE channel_id = $1 AND message_id > $2 AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id = $3 AND h.channel_id = apifull_channel_message.channel_id
			AND h.message_id = apifull_channel_message.message_id)
		ORDER BY message_id ASC LIMIT $4`, channelID, pts, userID, limit+1)
	if err != nil {
		return persistedChannelState{}, nil, nil, nil, err
	}
	for messageRows.Next() {
		var row persistedChannelMessage
		var edited, pinned int16
		if err := messageRows.Scan(&row.ChannelID, &row.MessageID, &row.Sender, &row.Date, &row.Text,
			&row.ContentJSON, &edited, &row.EditedAt, &pinned); err != nil {
			messageRows.Close()
			return persistedChannelState{}, nil, nil, nil, err
		}
		row.Edited, row.Pinned = edited != 0, pinned != 0
		rows = append(rows, row)
	}
	if err := messageRows.Err(); err != nil {
		messageRows.Close()
		return persistedChannelState{}, nil, nil, nil, err
	}
	messageRows.Close()

	events := make([]persistedChannelEvent, 0, limit+1)
	eventRows, err := pool.Query(ctx, `SELECT channel_id, pts, pts_count, event_type, message_ids,
		sender_user_id, date, message, COALESCE(content_json, '') AS content_json, edited_at, pinned
		FROM apifull_channel_event WHERE channel_id = $1 AND pts > $2
		ORDER BY pts ASC LIMIT $3`, channelID, pts, limit+1)
	if err == nil {
		for eventRows.Next() {
			var row persistedChannelEvent
			var pinned int16
			if scanErr := eventRows.Scan(&row.ChannelID, &row.Pts, &row.PtsCount, &row.EventType,
				&row.MessageIDs, &row.Sender, &row.Date, &row.Text, &row.ContentJSON, &row.EditedAt, &pinned); scanErr != nil {
				eventRows.Close()
				return persistedChannelState{}, nil, nil, nil, scanErr
			}
			row.Pinned = pinned != 0
			events = append(events, row)
		}
		if rowsErr := eventRows.Err(); rowsErr != nil {
			eventRows.Close()
			return persistedChannelState{}, nil, nil, nil, rowsErr
		}
		eventRows.Close()
	} else if !missingChannelEventTable(err) {
		return persistedChannelState{}, nil, nil, nil, err
	}

	hiddenIDs := make([]int32, 0)
	hiddenRows, err := pool.Query(ctx, `SELECT message_id FROM apifull_channel_message_hidden WHERE user_id = $1 AND channel_id = $2`, userID, channelID)
	if err == nil {
		for hiddenRows.Next() {
			var id int32
			if scanErr := hiddenRows.Scan(&id); scanErr != nil {
				hiddenRows.Close()
				return persistedChannelState{}, nil, nil, nil, scanErr
			}
			hiddenIDs = append(hiddenIDs, id)
		}
		if rowsErr := hiddenRows.Err(); rowsErr != nil {
			hiddenRows.Close()
			return persistedChannelState{}, nil, nil, nil, rowsErr
		}
		hiddenRows.Close()
	} else if !missingChannelEventTable(err) {
		return persistedChannelState{}, nil, nil, nil, err
	}

	return state, rows, events, hiddenIDs, nil
}

func ensureChannelMemberPostgres(ctx context.Context, pool *pgxpool.Pool, channelID, userID, creatorID int64) error {
	if userID == creatorID {
		return nil
	}
	var bannedRights string
	err := pool.QueryRow(ctx, `SELECT banned_rights FROM apifull_channel_member WHERE channel_id = $1 AND user_id = $2`, channelID, userID).Scan(&bannedRights)
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrUserNotParticipant
	}
	if err != nil {
		return err
	}
	if bannedRights == "" {
		return nil
	}
	var rights channelBannedRights
	if err := json.Unmarshal([]byte(bannedRights), &rights); err != nil {
		return err
	}
	if rights.ViewMessages {
		return mtproto.ErrUserNotParticipant
	}
	return nil
}

func persistedChannelMessageToMTProto(row persistedChannelMessage) (*mtproto.Message, error) {
	var content struct {
		Media       *mtproto.MessageMedia    `json:"media,omitempty"`
		Entities    []*mtproto.MessageEntity `json:"entities,omitempty"`
		ReplyMarkup *mtproto.ReplyMarkup     `json:"reply_markup,omitempty"`
		GroupedID   int64                    `json:"grouped_id,omitempty"`
	}
	if row.ContentJSON != "" {
		if err := json.Unmarshal([]byte(row.ContentJSON), &content); err != nil {
			return nil, err
		}
	}
	message := &mtproto.Message{
		Out:         true,
		Id:          row.MessageID,
		FromId:      mtproto.MakePeerUser(row.Sender),
		PeerId:      mtproto.MakePeerChannel(row.ChannelID),
		Date:        int32(row.Date),
		Message:     row.Text,
		Media:       content.Media,
		Entities:    content.Entities,
		ReplyMarkup: content.ReplyMarkup,
		Pinned:      row.Pinned,
		Views:       wrapperspb.Int32(0),
		// The current TL schema shares this flag with forwards, so encode both fields.
		Forwards: wrapperspb.Int32(0),
	}
	if content.GroupedID > 0 {
		message.GroupedId = wrapperspb.Int64(content.GroupedID)
	}
	if row.Edited && row.EditedAt != 0 {
		message.EditDate = wrapperspb.Int32(int32(row.EditedAt))
	}
	return mtproto.MakeTLMessage(message).To_Message(), nil
}
