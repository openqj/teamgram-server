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
)

// RPCEmojiServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func loadCreatedEmojiKeywords(userID int64) ([]*mtproto.EmojiKeyword, int32, error) {
	raw, err := persist.Default.Get(fmt.Sprintf("%s%d:create", stickerKeyPrefix, userID))
	if err != nil || raw == "" {
		return []*mtproto.EmojiKeyword{}, 0, err
	}
	var saved mtproto.TLStickersCreateStickerSet
	if err = json.Unmarshal([]byte(raw), &saved); err != nil {
		return nil, 0, err
	}
	var kws []*mtproto.EmojiKeyword
	for _, item := range saved.GetStickers() {
		if item == nil {
			continue
		}
		keyword := ""
		if item.GetKeywords() != nil {
			keyword = item.GetKeywords().GetValue()
		}
		emoji := item.GetEmoji()
		if keyword == "" {
			keyword = emoji
		}
		if keyword == "" {
			continue
		}
		var emoticons []string
		if emoji != "" {
			emoticons = []string{emoji}
		}
		kws = append(kws, mtproto.MakeTLEmojiKeyword(&mtproto.EmojiKeyword{
			Keyword:   keyword,
			Emoticons: emoticons,
		}).To_EmojiKeyword())
	}
	if len(kws) == 0 {
		return []*mtproto.EmojiKeyword{}, 0, nil
	}
	return kws, 1, nil
}

func emojiKeywordDifference(lang string, from, version int32, kws []*mtproto.EmojiKeyword) *mtproto.EmojiKeywordsDifference {
	if kws == nil {
		kws = []*mtproto.EmojiKeyword{}
	}
	if version == 0 || (version != 0 && from == version) {
		kws = []*mtproto.EmojiKeyword{}
	}
	return mtproto.MakeTLEmojiKeywordsDifference(&mtproto.EmojiKeywordsDifference{
		LangCode:    lang,
		FromVersion: from,
		Version:     version,
		Keywords:    kws,
	}).To_EmojiKeywordsDifference()
}

func (c *ApiFullCore) MessagesGetEmojiKeywords(in *mtproto.TLMessagesGetEmojiKeywords) (*mtproto.EmojiKeywordsDifference, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	lang := ""
	if in != nil {
		lang = in.GetLangCode()
	}
	kws, version, err := loadCreatedEmojiKeywords(uid)
	if err != nil {
		return nil, err
	}
	return emojiKeywordDifference(lang, 0, version, kws), nil
}

func (c *ApiFullCore) MessagesGetEmojiKeywordsDifference(in *mtproto.TLMessagesGetEmojiKeywordsDifference) (*mtproto.EmojiKeywordsDifference, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	lang := ""
	var from int32
	if in != nil {
		lang = in.GetLangCode()
		from = in.GetFromVersion()
	}
	kws, version, err := loadCreatedEmojiKeywords(uid)
	if err != nil {
		return nil, err
	}
	return emojiKeywordDifference(lang, from, version, kws), nil
}

func (c *ApiFullCore) MessagesGetEmojiKeywordsLanguages(in *mtproto.TLMessagesGetEmojiKeywordsLanguages) (*mtproto.Vector_EmojiLanguage, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	// The APIFull service has no authoritative emoji language catalog. Returning
	// an empty vector would make clients cache a successful, incomplete catalog.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesGetEmojiURL(in *mtproto.TLMessagesGetEmojiURL) (*mtproto.EmojiURL, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	// URL resolution requires the emoji catalog/CDN service, which is not wired
	// into APIFull.
	return nil, mtproto.ErrMethodNotImpl
}
