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
	"errors"
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCCustomEmojisServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) AccountGetDefaultProfilePhotoEmojis(in *mtproto.TLAccountGetDefaultProfilePhotoEmojis) (*mtproto.EmojiList, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	ids, err := persist.LoadEmojiDocumentIDs(stickerRequestContext(c), "profile")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	hash := emojiListHash(ids)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{Hash: hash}).To_EmojiList(), nil
	}
	return emojiListReply(ids, hash), nil
}

func (c *ApiFullCore) AccountGetDefaultGroupPhotoEmojis(in *mtproto.TLAccountGetDefaultGroupPhotoEmojis) (*mtproto.EmojiList, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	ids, err := persist.LoadEmojiDocumentIDs(stickerRequestContext(c), "group")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	hash := emojiListHash(ids)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{Hash: hash}).To_EmojiList(), nil
	}
	return emojiListReply(ids, hash), nil
}

func (c *ApiFullCore) MessagesGetCustomEmojiDocuments(in *mtproto.TLMessagesGetCustomEmojiDocuments) (*mtproto.Vector_Document, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	var ids []int64
	if in != nil {
		ids = in.GetDocumentId()
	}
	rows, err := persist.LoadCustomEmojiDocuments(stickerRequestContext(c), ids)
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	docs := make([]*mtproto.Document, 0, len(rows))
	for _, row := range rows {
		docs = append(docs, customEmojiDocument(row))
	}
	return &mtproto.Vector_Document{Datas: docs}, nil
}

func (c *ApiFullCore) MessagesGetEmojiStickers(in *mtproto.TLMessagesGetEmojiStickers) (*mtproto.Messages_AllStickers, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.SearchStickerSets(stickerRequestContext(c), uid, "", true, 100)
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	items := make([]*mtproto.StickerSet, 0, len(sets))
	ids := make([]int64, 0, len(sets))
	for i := range sets {
		set := sets[i]
		ids = append(ids, set.ID)
		items = append(items, emojiStickerSet(&set))
	}
	hash := idListHash(ids)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesAllStickersNotModified(&mtproto.Messages_AllStickers{Hash: hash}).To_Messages_AllStickers(), nil
	}
	return mtproto.MakeTLMessagesAllStickers(&mtproto.Messages_AllStickers{Hash: hash, Sets: items}).To_Messages_AllStickers(), nil
}

func (c *ApiFullCore) MessagesGetFeaturedEmojiStickers(in *mtproto.TLMessagesGetFeaturedEmojiStickers) (*mtproto.Messages_FeaturedStickers, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	sets, err := persist.ListStickerSets(stickerRequestContext(c), uid, false, false, true, 100)
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	covers := make([]*mtproto.StickerSetCovered, 0, len(sets))
	ids := make([]int64, 0, len(sets))
	for i := range sets {
		if !sets[i].Emojis {
			continue
		}
		ids = append(ids, sets[i].ID)
		covers = append(covers, stickerCovered(sets[i]))
	}
	hash := idListHash(ids)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesFeaturedStickersNotModified(&mtproto.Messages_FeaturedStickers{Count: int32(len(covers)), Hash: hash}).To_Messages_FeaturedStickers(), nil
	}
	return mtproto.MakeTLMessagesFeaturedStickers(&mtproto.Messages_FeaturedStickers{Count: int32(len(covers)), Hash: hash, Sets: covers}).To_Messages_FeaturedStickers(), nil
}

func (c *ApiFullCore) MessagesSearchCustomEmoji(in *mtproto.TLMessagesSearchCustomEmoji) (*mtproto.EmojiList, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	emoticon := ""
	var requestedHash int64
	if in != nil {
		emoticon, requestedHash = in.GetEmoticon(), in.GetHash()
	}
	rows, err := persist.SearchCustomEmojiDocuments(stickerRequestContext(c), emoticon)
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	hash := emojiListHash(ids)
	if requestedHash != 0 && requestedHash == hash {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{Hash: hash}).To_EmojiList(), nil
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{Hash: hash, DocumentId: ids}).To_EmojiList(), nil
}

func customEmojiProviderError(c *ApiFullCore, err error) error {
	if errors.Is(err, persist.ErrStickerProviderUnavailable) {
		return stickersProviderUnavailable(c)
	}
	return err
}

func emojiListHash(ids []int64) int64 {
	h := fnv.New64a()
	for _, id := range ids {
		var b [8]byte
		for i := range b {
			b[i] = byte(id >> (8 * i))
		}
		_, _ = h.Write(b[:])
	}
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

func emojiListReply(ids []int64, hash int64) *mtproto.EmojiList {
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{Hash: hash, DocumentId: append([]int64(nil), ids...)}).To_EmojiList()
}

func customEmojiDocument(row persist.EmojiDocumentRecord) *mtproto.Document {
	mime := row.MimeType
	if mime == "" {
		mime = "application/x-tgsticker"
	}
	return mtproto.MakeTLDocument(&mtproto.Document{
		Id: row.ID, AccessHash: row.AccessHash, Date: row.Date,
		MimeType: mime, Size2_INT64: row.SizeBytes, DcId: row.DCID,
		FileReference: append([]byte(nil), row.FileRef...),
		Attributes:    []*mtproto.DocumentAttribute{mtproto.MakeTLDocumentAttributeCustomEmoji(&mtproto.DocumentAttribute{Alt: row.Alt}).To_DocumentAttribute()},
	}).To_Document()
}

func emojiStickerSet(s *persist.StickerSet) *mtproto.StickerSet {
	return mtproto.MakeTLStickerSet(&mtproto.StickerSet{
		Id: s.ID, AccessHash: s.AccessHash, Title: s.Title, ShortName: s.ShortName,
		Emojis: true, Creator: s.Creator, Archived: s.Archived, Count: int32(len(s.Documents)), Hash: int32(stickerSetHash(s)),
		Animated: s.Animated, Videos: s.Videos,
	}).To_StickerSet()
}
