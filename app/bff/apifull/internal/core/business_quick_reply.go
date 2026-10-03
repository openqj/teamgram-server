// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// RPCBusinessQuickReplyServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const quickReplyKeyPrefix = "qreply:"

var quickReplyMu sync.Mutex

type quickReplyStored struct {
	ShortcutId int32  `json:"shortcut_id"`
	Shortcut   string `json:"shortcut"`
}

func quickReplyKey(userId int64) string {
	return fmt.Sprintf("%s%d", quickReplyKeyPrefix, userId)
}

func loadQuickReplies(userId int64) ([]quickReplyStored, error) {
	raw, err := persist.Default.Get(quickReplyKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []quickReplyStored
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveQuickReplies(userId int64, list []quickReplyStored) error {
	if list == nil {
		list = []quickReplyStored{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(quickReplyKey(userId), string(b))
}

type qrMsgItem struct {
	ShortcutId int32   `json:"shortcut_id"`
	Ids        []int32 `json:"ids"`
}

func quickReplyMsgKey(userId int64) string {
	return fmt.Sprintf("%smsg:%d", quickReplyKeyPrefix, userId)
}

func loadQuickReplyMsgs(userId int64) ([]qrMsgItem, error) {
	raw, err := persist.Default.Get(quickReplyMsgKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []qrMsgItem
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveQuickReplyMsgs(userId int64, list []qrMsgItem) error {
	if list == nil {
		list = []qrMsgItem{}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(quickReplyMsgKey(userId), string(b))
}

func (c *ApiFullCore) hydrateQuickReplyMessages(userID int64, ids []int32) ([]*mtproto.Message, error) {
	if len(ids) == 0 {
		return []*mtproto.Message{}, nil
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	messages := make([]*mtproto.Message, 0, len(ids))
	seen := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		box, err := d.PollMessageReader.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{
			UserId: userID,
			Id:     id,
		})
		if err != nil {
			return nil, err
		}
		if box == nil || box.GetMessageId() != id || box.GetMessage() == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
		switch box.GetPeerType() {
		case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		default:
			return nil, mtproto.ErrPeerIdInvalid
		}
		messages = append(messages, box.ToMessage(userID))
	}
	return messages, nil
}

func (c *ApiFullCore) MessagesGetQuickReplies(in *mtproto.TLMessagesGetQuickReplies) (*mtproto.Messages_QuickReplies, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	_ = in
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	list, err := loadQuickReplies(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	items, err := loadQuickReplyMsgs(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	var messageIDs []int32
	for _, item := range items {
		messageIDs = append(messageIDs, item.Ids...)
	}
	messages, err := c.hydrateQuickReplyMessages(c.MD.UserId, messageIDs)
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.QuickReply, 0, len(list))
	for _, q := range list {
		out = append(out, mtproto.MakeTLQuickReply(&mtproto.QuickReply{
			ShortcutId: q.ShortcutId,
			Shortcut:   q.Shortcut,
		}).To_QuickReply())
	}
	return mtproto.MakeTLMessagesQuickReplies(&mtproto.Messages_QuickReplies{
		QuickReplies: out,
		Messages:     messages,
		Chats:        []*mtproto.Chat{},
		Users:        []*mtproto.User{},
	}).To_Messages_QuickReplies(), nil
}

func (c *ApiFullCore) MessagesReorderQuickReplies(in *mtproto.TLMessagesReorderQuickReplies) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	list, err := loadQuickReplies(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	if in == nil || len(in.GetOrder()) == 0 {
		return mtproto.BoolTrue, nil
	}
	byID := make(map[int32]quickReplyStored, len(list))
	for _, q := range list {
		byID[q.ShortcutId] = q
	}
	next := make([]quickReplyStored, 0, len(list))
	seen := map[int32]struct{}{}
	for _, id := range in.GetOrder() {
		q, ok := byID[id]
		if !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		next = append(next, q)
	}
	for _, q := range list {
		if _, ok := seen[q.ShortcutId]; !ok {
			next = append(next, q)
		}
	}
	if err := saveQuickReplies(c.MD.UserId, next); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesCheckQuickReplyShortcut(in *mtproto.TLMessagesCheckQuickReplyShortcut) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	name := ""
	if in != nil {
		name = in.GetShortcut()
	}
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	list, err := loadQuickReplies(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	for _, q := range list {
		if q.Shortcut == name {
			return mtproto.BoolFalse, nil
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesEditQuickReplyShortcut(in *mtproto.TLMessagesEditQuickReplyShortcut) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetShortcut() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	list, err := loadQuickReplies(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	for _, q := range list {
		if q.Shortcut == in.GetShortcut() && q.ShortcutId != in.GetShortcutId() {
			return mtproto.BoolFalse, nil
		}
	}
	for i := range list {
		if list[i].ShortcutId == in.GetShortcutId() {
			list[i].Shortcut = in.GetShortcut()
			if err := saveQuickReplies(c.MD.UserId, list); err != nil {
				return nil, err
			}
			return mtproto.BoolTrue, nil
		}
	}
	list = append(list, quickReplyStored{ShortcutId: in.GetShortcutId(), Shortcut: in.GetShortcut()})
	if err := saveQuickReplies(c.MD.UserId, list); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesDeleteQuickReplyShortcut(in *mtproto.TLMessagesDeleteQuickReplyShortcut) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	list, err := loadQuickReplies(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	next := list[:0]
	for _, q := range list {
		if q.ShortcutId != in.GetShortcutId() {
			next = append(next, q)
		}
	}
	if err := saveQuickReplies(c.MD.UserId, next); err != nil {
		return nil, err
	}
	msgs, err := loadQuickReplyMsgs(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	kept := msgs[:0]
	for _, item := range msgs {
		if item.ShortcutId != in.GetShortcutId() {
			kept = append(kept, item)
		}
	}
	if err := saveQuickReplyMsgs(c.MD.UserId, kept); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesGetQuickReplyMessages(in *mtproto.TLMessagesGetQuickReplyMessages) (*mtproto.Messages_Messages, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	quickReplyMu.Lock()
	defer quickReplyMu.Unlock()
	items, err := loadQuickReplyMsgs(c.MD.UserId)
	if err != nil {
		return nil, err
	}
	var ids []int32
	for _, item := range items {
		if item.ShortcutId == in.GetShortcutId() {
			ids = item.Ids
			break
		}
	}
	if len(in.GetId()) > 0 {
		allow := make(map[int32]struct{}, len(in.GetId()))
		for _, id := range in.GetId() {
			allow[id] = struct{}{}
		}
		kept := make([]int32, 0, len(ids))
		for _, id := range ids {
			if _, ok := allow[id]; ok {
				kept = append(kept, id)
			}
		}
		ids = kept
	}
	messages, err := c.hydrateQuickReplyMessages(c.MD.UserId, ids)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: messages,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}
