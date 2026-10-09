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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCEmojiCategoriesServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func emojiGroupListHash(groups []*mtproto.EmojiGroup) int32 {
	if len(groups) == 0 {
		return 0
	}
	h := int64(1)
	for _, g := range groups {
		if g == nil {
			continue
		}
		h = h*31 + g.GetIconEmojiId()
		for _, ch := range g.GetTitle() {
			h = h*31 + int64(ch)
		}
		for _, emo := range g.GetEmoticons() {
			for _, ch := range emo {
				h = h*31 + int64(ch)
			}
		}
	}
	if h < 0 {
		h = -h
	}
	return int32(h & 0x7fffffff)
}

func replyEmojiGroups(hash int32, groups []*mtproto.EmojiGroup) *mtproto.Messages_EmojiGroups {
	if groups == nil {
		groups = []*mtproto.EmojiGroup{}
	}
	h := emojiGroupListHash(groups)
	if len(groups) > 0 && hash != 0 && hash == h {
		return mtproto.MakeTLMessagesEmojiGroupsNotModified(&mtproto.Messages_EmojiGroups{
			Hash: h,
		}).To_Messages_EmojiGroups()
	}
	return mtproto.MakeTLMessagesEmojiGroups(&mtproto.Messages_EmojiGroups{
		Hash:   h,
		Groups: groups,
	}).To_Messages_EmojiGroups()
}

func createEmoticons(userID int64) (string, []string, error) {
	raw, err := persist.Default.Get(fmt.Sprintf("%s%d:create", stickerKeyPrefix, userID))
	if err != nil || raw == "" {
		return "", nil, err
	}
	var saved mtproto.TLStickersCreateStickerSet
	if json.Unmarshal([]byte(raw), &saved) != nil {
		return "", nil, nil
	}
	var emos []string
	seen := map[string]struct{}{}
	for _, item := range saved.GetStickers() {
		if item == nil || item.GetEmoji() == "" {
			continue
		}
		if _, ok := seen[item.GetEmoji()]; ok {
			continue
		}
		seen[item.GetEmoji()] = struct{}{}
		emos = append(emos, item.GetEmoji())
	}
	return saved.GetShortName(), emos, nil
}

func stickerEmojiGroups(userID int64) ([]*mtproto.EmojiGroup, error) {
	names, err := loadInstalledStickerNames(userID)
	if err != nil {
		return nil, err
	}
	created, err := persist.Default.Get(stickerNameKey(userID))
	if err != nil {
		return nil, err
	}
	createShort, emos, err := createEmoticons(userID)
	if err != nil {
		return nil, err
	}
	if created == "" {
		created = createShort
	}
	seen := map[string]struct{}{}
	var groups []*mtproto.EmojiGroup
	add := func(title string, emoticons []string) {
		if title == "" {
			return
		}
		if _, ok := seen[title]; ok {
			return
		}
		seen[title] = struct{}{}
		groups = append(groups, mtproto.MakeTLEmojiGroup(&mtproto.EmojiGroup{
			Title:     title,
			Emoticons: emoticons,
		}).To_EmojiGroup())
	}
	if created != "" {
		add(created, emos)
	}
	for _, token := range names {
		if token == created {
			continue
		}
		add(token, nil)
	}
	return groups, nil
}

func emojiStatusGroups(userID int64) ([]*mtproto.EmojiGroup, error) {
	raw, err := persist.Default.Get(emojiStatusKey(userID))
	if err != nil || raw == "" || raw == "null" {
		return nil, err
	}
	st := &mtproto.EmojiStatus{}
	if err = json.Unmarshal([]byte(raw), st); err != nil {
		return nil, err
	}
	id := st.GetDocumentId()
	if id == 0 {
		id = st.GetCollectibleId()
	}
	if id == 0 {
		return nil, nil
	}
	return []*mtproto.EmojiGroup{
		mtproto.MakeTLEmojiGroup(&mtproto.EmojiGroup{
			Title:       strconv.FormatInt(id, 10),
			IconEmojiId: id,
		}).To_EmojiGroup(),
	}, nil
}

func (c *ApiFullCore) MessagesGetEmojiGroups(in *mtproto.TLMessagesGetEmojiGroups) (*mtproto.Messages_EmojiGroups, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	var hash int32
	if in != nil {
		hash = in.GetHash()
	}
	rows, err := persist.LoadEmojiGroups(stickerRequestContext(c), "generic")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	groups := emojiGroupRecords(rows)
	return replyEmojiGroups(hash, groups), nil
}

func (c *ApiFullCore) MessagesGetEmojiStatusGroups(in *mtproto.TLMessagesGetEmojiStatusGroups) (*mtproto.Messages_EmojiGroups, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	var hash int32
	if in != nil {
		hash = in.GetHash()
	}
	rows, err := persist.LoadEmojiGroups(stickerRequestContext(c), "status")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	return replyEmojiGroups(hash, emojiGroupRecords(rows)), nil
}

func (c *ApiFullCore) MessagesGetEmojiProfilePhotoGroups(in *mtproto.TLMessagesGetEmojiProfilePhotoGroups) (*mtproto.Messages_EmojiGroups, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	var hash int32
	if in != nil {
		hash = in.GetHash()
	}
	rows, err := persist.LoadEmojiGroups(stickerRequestContext(c), "profile")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	return replyEmojiGroups(hash, emojiGroupRecords(rows)), nil
}

func (c *ApiFullCore) MessagesGetEmojiStickerGroups(in *mtproto.TLMessagesGetEmojiStickerGroups) (*mtproto.Messages_EmojiGroups, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	var hash int32
	if in != nil {
		hash = in.GetHash()
	}
	rows, err := persist.LoadEmojiGroups(stickerRequestContext(c), "sticker")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	return replyEmojiGroups(hash, emojiGroupRecords(rows)), nil
}

func emojiGroupRecords(rows []persist.EmojiGroupRecord) []*mtproto.EmojiGroup {
	out := make([]*mtproto.EmojiGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, mtproto.MakeTLEmojiGroup(&mtproto.EmojiGroup{
			Title: row.Title, IconEmojiId: row.IconEmojiID, Emoticons: append([]string(nil), row.Emoticons...),
		}).To_EmojiGroup())
	}
	return out
}
