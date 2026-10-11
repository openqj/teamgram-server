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
	if err != nil {
		return nil, err
	}
	stored, err := decodeRings(raw)
	if err != nil {
		return nil, err
	}
	return ringDocuments(stored), nil
}

func decodeRings(raw string) ([]ringItem, error) {
	if raw == "" {
		return nil, nil
	}
	var stored []ringItem
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	return stored, nil
}

func ringDocuments(stored []ringItem) []*mtproto.Document {
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
	return out
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

// updateRings performs one owner-scoped ringtone mutation under the shared
// store transaction boundary. The PostgreSQL path locks the KV row/advisory
// key so concurrent saves from separate BFF instances cannot lose entries.
func updateRings(userID int64, fn func([]ringItem) ([]ringItem, error)) error {
	if userID <= 0 || fn == nil {
		return mtproto.ErrAuthKeyUnregistered
	}
	return persist.Update(ringKey(userID), func(raw string) (string, error) {
		stored, err := decodeRings(raw)
		if err != nil {
			return "", err
		}
		next, err := fn(stored)
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(next)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
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
	err = updateRings(userID, func(prev []ringItem) ([]ringItem, error) {
		list := make([]ringItem, 0, len(prev)+1)
		if unsave {
			for _, item := range prev {
				if item.ID != id.GetId() {
					list = append(list, item)
				}
			}
			return list, nil
		}
		item := ringItem{ID: id.GetId(), AccessHash: id.GetAccessHash()}
		if fr := id.GetFileReference(); len(fr) > 0 {
			item.Ref = append([]byte(nil), fr...)
		}
		list = append(list, item)
		for _, existing := range prev {
			if existing.ID != id.GetId() {
				list = append(list, existing)
			}
		}
		return list, nil
	})
	if err != nil {
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
