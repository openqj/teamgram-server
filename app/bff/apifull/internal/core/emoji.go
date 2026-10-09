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
	"errors"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// emojiKeywordLanguages lists the language codes that APIFull can serve from
// the keyword data exposed by MessagesGetEmojiKeywords. The catalogue is
// intentionally limited to data backed by this service.
var emojiKeywordLanguages = map[string]struct{}{
	"en": {},
}

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
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	lang := ""
	if in != nil {
		lang = in.GetLangCode()
	}
	rows, version, err := persist.LoadEmojiKeywords(stickerRequestContext(c), lang)
	if err != nil {
		if errors.Is(err, persist.ErrStickerProviderUnavailable) {
			return nil, stickersProviderUnavailable(c)
		}
		return nil, err
	}
	kws := emojiKeywordRecords(rows)
	return emojiKeywordDifference(lang, 0, version, kws), nil
}

func (c *ApiFullCore) MessagesGetEmojiKeywordsDifference(in *mtproto.TLMessagesGetEmojiKeywordsDifference) (*mtproto.EmojiKeywordsDifference, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	lang := ""
	var from int32
	if in != nil {
		lang = in.GetLangCode()
		from = in.GetFromVersion()
	}
	rows, version, err := persist.LoadEmojiKeywords(stickerRequestContext(c), lang)
	if err != nil {
		if errors.Is(err, persist.ErrStickerProviderUnavailable) {
			return nil, stickersProviderUnavailable(c)
		}
		return nil, err
	}
	kws := emojiKeywordRecords(rows)
	return emojiKeywordDifference(lang, from, version, kws), nil
}

func (c *ApiFullCore) MessagesGetEmojiKeywordsLanguages(in *mtproto.TLMessagesGetEmojiKeywordsLanguages) (*mtproto.Vector_EmojiLanguage, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}

	var requested []string
	if in != nil {
		requested = in.GetLangCodes()
	}
	rows, err := persist.LoadEmojiLanguages(stickerRequestContext(c), requested)
	if err != nil {
		if errors.Is(err, persist.ErrStickerProviderUnavailable) {
			return nil, stickersProviderUnavailable(c)
		}
		return nil, err
	}
	languages := make([]*mtproto.EmojiLanguage, 0, len(rows))
	for _, row := range rows {
		languages = append(languages, mtproto.MakeTLEmojiLanguage(&mtproto.EmojiLanguage{
			LangCode: row.LangCode,
		}).To_EmojiLanguage())
	}
	return &mtproto.Vector_EmojiLanguage{Datas: languages}, nil
}

func (c *ApiFullCore) MessagesGetEmojiURL(in *mtproto.TLMessagesGetEmojiURL) (*mtproto.EmojiURL, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	lang := ""
	if in != nil {
		lang = in.GetLangCode()
	}
	url, err := persist.LoadEmojiURL(stickerRequestContext(c), lang)
	if err != nil {
		if errors.Is(err, persist.ErrStickerProviderUnavailable) {
			return nil, stickersProviderUnavailable(c)
		}
		return nil, err
	}
	if url == "" {
		return nil, mtproto.ErrLangCodeNotSupported
	}
	return mtproto.MakeTLEmojiURL(&mtproto.EmojiURL{Url: url}).To_EmojiURL(), nil
}

func emojiKeywordRecords(rows []persist.EmojiKeywordRecord) []*mtproto.EmojiKeyword {
	out := make([]*mtproto.EmojiKeyword, 0, len(rows))
	for _, row := range rows {
		out = append(out, mtproto.MakeTLEmojiKeyword(&mtproto.EmojiKeyword{
			Keyword: row.Keyword, Emoticons: append([]string(nil), row.Emoticons...),
		}).To_EmojiKeyword())
	}
	return out
}
