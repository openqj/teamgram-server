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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCSavedMessageTagsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type savedTagJSON struct {
	Emoticon   string  `json:"emoticon,omitempty"`
	DocumentId int64   `json:"documentId,omitempty"`
	Title      *string `json:"title,omitempty"`
	Count      int32   `json:"count,omitempty"`
}

func savedTagsKey(userId int64) string {
	return fmt.Sprintf("tags:%d", userId)
}

func loadSavedTags(userId int64) ([]savedTagJSON, error) {
	raw, err := persist.Default.Get(savedTagsKey(userId))
	if err != nil || raw == "" {
		return nil, err
	}
	var list []savedTagJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveSavedTags(userId int64, list []savedTagJSON) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return persist.Default.Set(savedTagsKey(userId), string(b))
}

func sameSavedReaction(rx *mtproto.Reaction, t savedTagJSON) bool {
	if rx == nil {
		return false
	}
	return rx.GetEmoticon() == t.Emoticon && rx.GetDocumentId() == t.DocumentId
}

func reactionFromSavedTag(t savedTagJSON) *mtproto.Reaction {
	if t.DocumentId != 0 {
		return mtproto.MakeTLReactionCustomEmoji(&mtproto.Reaction{DocumentId: t.DocumentId}).To_Reaction()
	}
	return mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: t.Emoticon}).To_Reaction()
}

func (c *ApiFullCore) MessagesGetSavedReactionTags(in *mtproto.TLMessagesGetSavedReactionTags) (*mtproto.Messages_SavedReactionTags, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	list, err := loadSavedTags(userId)
	if err != nil {
		return nil, err
	}
	h := savedTagsHash(list)
	if in != nil && in.GetHash() != 0 && in.GetHash() == h {
		return mtproto.MakeTLMessagesSavedReactionTagsNotModified(&mtproto.Messages_SavedReactionTags{}).To_Messages_SavedReactionTags(), nil
	}
	tags := make([]*mtproto.SavedReactionTag, 0, len(list))
	for _, t := range list {
		tag := &mtproto.SavedReactionTag{
			Reaction: reactionFromSavedTag(t),
			Count:    t.Count,
		}
		if t.Title != nil {
			tag.Title = wrapperspb.String(*t.Title)
		}
		tags = append(tags, mtproto.MakeTLSavedReactionTag(tag).To_SavedReactionTag())
	}
	return mtproto.MakeTLMessagesSavedReactionTags(&mtproto.Messages_SavedReactionTags{
		Hash: h,
		Tags: tags,
	}).To_Messages_SavedReactionTags(), nil
}

func (c *ApiFullCore) MessagesUpdateSavedReactionTag(in *mtproto.TLMessagesUpdateSavedReactionTag) (*mtproto.Bool, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetReaction() == nil {
		return nil, mtproto.ErrReactionInvalid
	}
	rx := in.GetReaction()
	list, err := loadSavedTags(userId)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, t := range list {
		if sameSavedReaction(rx, t) {
			idx = i
			break
		}
	}
	if in.GetTitle() == nil {
		if idx >= 0 {
			list = append(list[:idx], list[idx+1:]...)
			if err := saveSavedTags(userId, list); err != nil {
				return nil, err
			}
		}
		return mtproto.BoolTrue, nil
	}
	title := in.GetTitle().GetValue()
	if idx >= 0 {
		list[idx].Title = &title
	} else {
		list = append(list, savedTagJSON{
			Emoticon:   rx.GetEmoticon(),
			DocumentId: rx.GetDocumentId(),
			Title:      &title,
		})
	}
	if err := saveSavedTags(userId, list); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func savedTagsHash(list []savedTagJSON) int64 {
	var h int64 = 1
	for _, t := range list {
		for _, r := range t.Emoticon {
			h = h*131 + int64(r)
		}
		h = h*131 + t.DocumentId
		if t.Title != nil {
			for _, r := range *t.Title {
				h = h*131 + int64(r)
			}
		}
		h = h*131 + int64(t.Count)
	}
	return h
}

func (c *ApiFullCore) MessagesGetDefaultTagReactions(in *mtproto.TLMessagesGetDefaultTagReactions) (*mtproto.Messages_Reactions, error) {
	userId, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	tags, err := loadSavedTags(userId)
	if err != nil {
		return nil, err
	}
	list := make([]*mtproto.Reaction, 0)
	if len(tags) > 0 {
		for _, t := range tags {
			list = append(list, reactionFromSavedTag(t))
		}
	} else {
		for _, emoji := range staticReactionEmojis {
			list = append(list, reactionOf(emoji, 0))
		}
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return reactionsResult(list, hash)
}
