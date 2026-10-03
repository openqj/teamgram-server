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
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
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
	ChannelID int64  `db:"channel_id"`
	MessageID int32  `db:"message_id"`
	Sender    int64  `db:"sender_user_id"`
	Date      int64  `db:"date"`
	Text      string `db:"message"`
	Edited    bool   `db:"edited"`
	EditedAt  int64  `db:"edited_at"`
	Pinned    bool   `db:"pinned"`
}

type persistedChannelEvent struct {
	ChannelID  int64  `db:"channel_id"`
	Pts        int32  `db:"pts"`
	PtsCount   int32  `db:"pts_count"`
	EventType  string `db:"event_type"`
	MessageIDs string `db:"message_ids"`
	Sender     int64  `db:"sender_user_id"`
	Date       int64  `db:"date"`
	Text       string `db:"message"`
	EditedAt   int64  `db:"edited_at"`
	Pinned     bool   `db:"pinned"`
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
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Mysql == nil || c.svcCtx.Dao.DB == nil {
		return nil, mtproto.ErrInternalServerError
	}

	state, err := loadChannelState(c.ctx, c.svcCtx.Dao.DB, in.GetChannelId())
	if err != nil {
		return nil, err
	}
	if err = ensureChannelMember(c.ctx, c.svcCtx.Dao.DB, in.GetChannelId(), in.GetUserId(), state.CreatorUserID); err != nil {
		return nil, err
	}

	rows := make([]persistedChannelMessage, 0, in.GetLimit()+1)
	err = c.svcCtx.Dao.DB.QueryRowsPartial(c.ctx, &rows, `SELECT channel_id, message_id, sender_user_id, date, message, edited, edited_at, pinned
		FROM apifull_channel_message WHERE channel_id=? AND message_id>? AND NOT EXISTS (
			SELECT 1 FROM apifull_channel_message_hidden h
			WHERE h.user_id=? AND h.channel_id=apifull_channel_message.channel_id
			AND h.message_id=apifull_channel_message.message_id)
		ORDER BY message_id ASC LIMIT ?`, in.GetChannelId(), in.GetPts(), in.GetUserId(), in.GetLimit()+1)
	if err != nil {
		return nil, err
	}

	events := make([]persistedChannelEvent, 0, in.GetLimit()+1)
	if err = c.svcCtx.Dao.DB.QueryRowsPartial(c.ctx, &events, `SELECT channel_id, pts, pts_count, event_type, message_ids,
		sender_user_id, date, message, edited_at, pinned
		FROM apifull_channel_event WHERE channel_id=? AND pts>? ORDER BY pts ASC LIMIT ?`,
		in.GetChannelId(), in.GetPts(), in.GetLimit()+1); err != nil && !missingChannelEventTable(err) {
		return nil, err
	}

	hidden := make(map[int32]struct{})
	var hiddenIDs []int32
	if err = c.svcCtx.Dao.DB.QueryRowsPartial(c.ctx, &hiddenIDs, `SELECT message_id FROM apifull_channel_message_hidden WHERE user_id=? AND channel_id=?`, in.GetUserId(), in.GetChannelId()); err == nil {
		for _, id := range hiddenIDs {
			hidden[id] = struct{}{}
		}
	} else if !missingChannelEventTable(err) {
		return nil, err
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
				row := persistedChannelMessage{ChannelID: event.ChannelID, MessageID: id, Sender: event.Sender, Date: event.Date, Text: event.Text, Pinned: event.Pinned, EditedAt: event.EditedAt, Edited: event.EditedAt != 0}
				items = append(items, channelDifferenceItem{pts: event.Pts, message: persistedChannelMessageToMTProto(row)})
			}
		case "edit":
			if len(visibleIDs) == 0 {
				continue
			}
			row := persistedChannelMessage{ChannelID: event.ChannelID, MessageID: visibleIDs[0], Sender: event.Sender, Date: event.Date, Text: event.Text, Pinned: event.Pinned, Edited: true, EditedAt: event.EditedAt}
			items = append(items, channelDifferenceItem{pts: event.Pts, update: mtproto.MakeTLUpdateEditChannelMessage(&mtproto.Update{
				Message_MESSAGE: persistedChannelMessageToMTProto(row),
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
		items = append(items, channelDifferenceItem{pts: row.MessageID, message: persistedChannelMessageToMTProto(row)})
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

func loadChannelState(ctx context.Context, db *sqlx.DB, channelID int64) (persistedChannelState, error) {
	var state persistedChannelState
	err := db.QueryRowPartial(ctx, &state, `SELECT c.creator_user_id,
		COALESCE(s.pts, (SELECT COALESCE(MAX(m.message_id), 0) FROM apifull_channel_message m WHERE m.channel_id=c.id)) AS pts
		FROM apifull_channel c
		LEFT JOIN apifull_channel_message_seq s ON s.channel_id=c.id
		WHERE c.id=?`, channelID)
	if errors.Is(err, sql.ErrNoRows) {
		return persistedChannelState{}, mtproto.ErrChannelInvalid
	}
	if err != nil {
		return persistedChannelState{}, err
	}
	return state, nil
}

func ensureChannelMember(ctx context.Context, db *sqlx.DB, channelID, userID, creatorID int64) error {
	if userID == creatorID {
		return nil
	}
	var member persistedChannelMember
	err := db.QueryRowPartial(ctx, &member, `SELECT banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return mtproto.ErrUserNotParticipant
	}
	if err != nil {
		return err
	}
	if member.BannedRights != "" {
		var rights channelBannedRights
		if err = json.Unmarshal([]byte(member.BannedRights), &rights); err != nil {
			return err
		}
		if rights.ViewMessages {
			return mtproto.ErrUserNotParticipant
		}
	}
	return nil
}

func persistedChannelMessageToMTProto(row persistedChannelMessage) *mtproto.Message {
	message := &mtproto.Message{
		Out:     true,
		Id:      row.MessageID,
		FromId:  mtproto.MakePeerUser(row.Sender),
		PeerId:  mtproto.MakePeerChannel(row.ChannelID),
		Date:    int32(row.Date),
		Message: row.Text,
		Pinned:  row.Pinned,
		Views:   wrapperspb.Int32(0),
		// The current TL schema shares this flag with forwards, so encode both fields.
		Forwards: wrapperspb.Int32(0),
	}
	if row.Edited && row.EditedAt != 0 {
		message.EditDate = wrapperspb.Int32(int32(row.EditedAt))
	}
	return mtproto.MakeTLMessage(message).To_Message()
}
