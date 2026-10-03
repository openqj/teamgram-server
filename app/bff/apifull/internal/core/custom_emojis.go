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
	"github.com/teamgram/proto/mtproto"
)

// RPCCustomEmojisServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) AccountGetDefaultProfilePhotoEmojis(in *mtproto.TLAccountGetDefaultProfilePhotoEmojis) (*mtproto.EmojiList, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{
		Hash:       0,
		DocumentId: []int64{},
	}).To_EmojiList(), nil
}

func (c *ApiFullCore) AccountGetDefaultGroupPhotoEmojis(in *mtproto.TLAccountGetDefaultGroupPhotoEmojis) (*mtproto.EmojiList, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{
		Hash:       0,
		DocumentId: []int64{},
	}).To_EmojiList(), nil
}

func (c *ApiFullCore) MessagesGetCustomEmojiDocuments(in *mtproto.TLMessagesGetCustomEmojiDocuments) (*mtproto.Vector_Document, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return &mtproto.Vector_Document{Datas: []*mtproto.Document{}}, nil
}

func (c *ApiFullCore) MessagesGetEmojiStickers(in *mtproto.TLMessagesGetEmojiStickers) (*mtproto.Messages_AllStickers, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAllStickers(&mtproto.Messages_AllStickers{
		Hash: 0,
		Sets: []*mtproto.StickerSet{},
	}).To_Messages_AllStickers(), nil
}

func (c *ApiFullCore) MessagesGetFeaturedEmojiStickers(in *mtproto.TLMessagesGetFeaturedEmojiStickers) (*mtproto.Messages_FeaturedStickers, error) {
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return c.loadFeaturedStickers(hash)
}

func (c *ApiFullCore) MessagesSearchCustomEmoji(in *mtproto.TLMessagesSearchCustomEmoji) (*mtproto.EmojiList, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{
		Hash:       0,
		DocumentId: []int64{},
	}).To_EmojiList(), nil
}
