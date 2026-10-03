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
	"strconv"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

// RPCRingtoneServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type ringItem struct {
	ID         int64  `json:"id"`
	AccessHash int64  `json:"access_hash"`
	Ref        []byte `json:"ref,omitempty"`
}

var ringMu sync.Mutex

func ringKey(userID int64) string {
	return "ring:" + strconv.FormatInt(userID, 10)
}

func loadRings(userID int64) ([]*mtproto.Document, error) {
	raw, err := persist.Default.Get(ringKey(userID))
	if err != nil || raw == "" {
		return nil, err
	}
	var stored []ringItem
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	out := make([]*mtproto.Document, 0, len(stored))
	for _, s := range stored {
		out = append(out, mtproto.MakeTLDocument(&mtproto.Document{
			Id:            s.ID,
			AccessHash:    s.AccessHash,
			FileReference: s.Ref,
			MimeType:      "audio/mpeg",
			Thumbs:        []*mtproto.PhotoSize{},
			VideoThumbs:   []*mtproto.VideoSize{},
			Attributes:    []*mtproto.DocumentAttribute{},
		}).To_Document())
	}
	return out, nil
}

func storeRings(userID int64, docs []*mtproto.Document) error {
	stored := make([]ringItem, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		item := ringItem{ID: d.GetId(), AccessHash: d.GetAccessHash()}
		if fr := d.GetFileReference(); len(fr) > 0 {
			item.Ref = append([]byte(nil), fr...)
		}
		stored = append(stored, item)
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return persist.Default.Set(ringKey(userID), string(raw))
}

func ringHash(docs []*mtproto.Document) int64 {
	var h int64 = 1
	for _, d := range docs {
		h = h*31 + d.GetId()
	}
	if h < 0 {
		h = -h
	}
	return h
}

func (c *ApiFullCore) AccountGetSavedRingtones(in *mtproto.TLAccountGetSavedRingtones) (*mtproto.Account_SavedRingtones, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	ringMu.Lock()
	docs, err := loadRings(userID)
	ringMu.Unlock()
	if err != nil {
		return nil, err
	}
	if docs == nil {
		docs = []*mtproto.Document{}
	}
	hash := ringHash(docs)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLAccountSavedRingtonesNotModified(&mtproto.Account_SavedRingtones{}).To_Account_SavedRingtones(), nil
	}
	return mtproto.MakeTLAccountSavedRingtones(&mtproto.Account_SavedRingtones{
		Hash:      hash,
		Ringtones: docs,
	}).To_Account_SavedRingtones(), nil
}

func (c *ApiFullCore) AccountSaveRingtone(in *mtproto.TLAccountSaveRingtone) (*mtproto.Account_SavedRingtone, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetId() == nil || in.GetId().GetId() == 0 {
		return nil, mtproto.ErrDocumentInvalid
	}
	id := in.GetId()
	unsave := in.GetUnsave() != nil && mtproto.FromBool(in.GetUnsave())

	ringMu.Lock()
	defer ringMu.Unlock()
	prev, err := loadRings(userID)
	if err != nil {
		return nil, err
	}
	list := make([]*mtproto.Document, 0, len(prev)+1)
	if unsave {
		for _, d := range prev {
			if d.GetId() != id.GetId() {
				list = append(list, d)
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
			MimeType:      "audio/mpeg",
			Thumbs:        []*mtproto.PhotoSize{},
			VideoThumbs:   []*mtproto.VideoSize{},
			Attributes:    []*mtproto.DocumentAttribute{},
		}).To_Document()
		list = append(list, doc)
		for _, d := range prev {
			if d.GetId() != id.GetId() {
				list = append(list, d)
			}
		}
	}
	if err := storeRings(userID, list); err != nil {
		return nil, err
	}
	return mtproto.MakeTLAccountSavedRingtone(&mtproto.Account_SavedRingtone{}).To_Account_SavedRingtone(), nil
}

func (c *ApiFullCore) AccountUploadRingtone(in *mtproto.TLAccountUploadRingtone) (*mtproto.Document, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetFile() == nil || in.GetFileName() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	d := c.apifullDao()
	if d == nil || d.DfsClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	mimeType := in.GetMimeType()
	if mimeType == "" {
		mimeType = "audio/mpeg"
	}
	return d.DfsUploadRingtoneFile(callContext(c), &dfs.TLDfsUploadRingtoneFile{
		Creator:  uid,
		File:     in.GetFile(),
		MimeType: mimeType,
		FileName: in.GetFileName(),
	})
}
