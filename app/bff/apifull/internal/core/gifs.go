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

// RPCGifsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type savedGifJSON struct {
	Id            int64  `json:"id"`
	AccessHash    int64  `json:"access_hash"`
	FileReference []byte `json:"file_reference,omitempty"`
	MimeType      string `json:"mime_type"`
}

func savedGifKey(uid int64) string {
	return fmt.Sprintf("gif:%d", uid)
}

func loadSavedGifs(uid int64) ([]*mtproto.Document, error) {
	raw, err := persist.Default.Get(savedGifKey(uid))
	if err != nil || raw == "" {
		return nil, err
	}
	var rows []savedGifJSON
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, err
	}
	gifs := make([]*mtproto.Document, 0, len(rows))
	for _, row := range rows {
		var ref []byte
		if len(row.FileReference) > 0 {
			ref = append([]byte(nil), row.FileReference...)
		}
		gifs = append(gifs, mtproto.MakeTLDocument(&mtproto.Document{
			Id:            row.Id,
			AccessHash:    row.AccessHash,
			FileReference: ref,
			MimeType:      row.MimeType,
			Thumbs:        []*mtproto.PhotoSize{},
			VideoThumbs:   []*mtproto.VideoSize{},
			Attributes:    []*mtproto.DocumentAttribute{},
		}).To_Document())
	}
	return gifs, nil
}

func storeSavedGifs(uid int64, gifs []*mtproto.Document) error {
	rows := make([]savedGifJSON, 0, len(gifs))
	for _, g := range gifs {
		var ref []byte
		if fr := g.GetFileReference(); len(fr) > 0 {
			ref = append([]byte(nil), fr...)
		}
		rows = append(rows, savedGifJSON{
			Id:            g.GetId(),
			AccessHash:    g.GetAccessHash(),
			FileReference: ref,
			MimeType:      g.GetMimeType(),
		})
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return persist.Default.Set(savedGifKey(uid), string(b))
}

func savedGifHash(gifs []*mtproto.Document) int64 {
	var h int64 = 1
	for _, g := range gifs {
		h = h*31 + g.GetId()
	}
	if h < 0 {
		h = -h
	}
	return h
}

func (c *ApiFullCore) MessagesGetSavedGifs(in *mtproto.TLMessagesGetSavedGifs) (*mtproto.Messages_SavedGifs, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	gifs, err := loadSavedGifs(uid)
	if err != nil {
		return nil, err
	}
	if gifs == nil {
		gifs = []*mtproto.Document{}
	}
	hash := savedGifHash(gifs)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesSavedGifsNotModified(&mtproto.Messages_SavedGifs{}).To_Messages_SavedGifs(), nil
	}
	return mtproto.MakeTLMessagesSavedGifs(&mtproto.Messages_SavedGifs{
		Hash: hash,
		Gifs: gifs,
	}).To_Messages_SavedGifs(), nil
}

func (c *ApiFullCore) MessagesSaveGif(in *mtproto.TLMessagesSaveGif) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetId() == nil || in.GetId().GetId() == 0 {
		return nil, mtproto.ErrGifIdInvalid
	}
	id := in.GetId()
	unsave := in.GetUnsave() != nil && mtproto.FromBool(in.GetUnsave())

	prev, err := loadSavedGifs(uid)
	if err != nil {
		return nil, err
	}
	list := make([]*mtproto.Document, 0, len(prev))
	if unsave {
		for _, g := range prev {
			if g.GetId() != id.GetId() {
				list = append(list, g)
			}
		}
	} else {
		var ref []byte
		if fr := id.GetFileReference(); len(fr) > 0 {
			ref = append([]byte(nil), fr...)
		}
		doc := mtproto.MakeTLDocument(&mtproto.Document{
			Id:            id.GetId(),
			AccessHash:    id.GetAccessHash(),
			FileReference: ref,
			MimeType:      "video/mp4",
			Thumbs:        []*mtproto.PhotoSize{},
			VideoThumbs:   []*mtproto.VideoSize{},
			Attributes:    []*mtproto.DocumentAttribute{},
		}).To_Document()
		list = append(list, doc)
		for _, g := range prev {
			if g.GetId() != id.GetId() {
				list = append(list, g)
			}
		}
	}
	if err := storeSavedGifs(uid, list); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
