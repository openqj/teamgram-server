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
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

// RPCImportedChatsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func historyImportTitleKey(uid int64) string {
	return fmt.Sprintf("b12:%d:", uid)
}

func historyImportMetaKey(uid int64) string {
	return fmt.Sprintf("b12:%d:meta", uid)
}

type historyImportMeta struct {
	ID      int64  `json:"id"`
	Confirm string `json:"confirm,omitempty"`
	Pm      bool   `json:"pm,omitempty"`
	Group   bool   `json:"group,omitempty"`
}

func loadHistoryImportMeta(uid int64) (historyImportMeta, error) {
	var meta historyImportMeta
	raw, err := persist.Default.Get(historyImportMetaKey(uid))
	if err != nil || raw == "" {
		return meta, err
	}
	err = json.Unmarshal([]byte(raw), &meta)
	return meta, err
}

func historyImportPeerFlags(p *mtproto.InputPeer) (confirm string, pm, group bool) {
	if p == nil {
		return "", false, false
	}
	switch {
	case p.GetUserId() != 0:
		return "user:" + strconv.FormatInt(p.GetUserId(), 10), true, false
	case p.GetChatId() != 0:
		return "chat:" + strconv.FormatInt(p.GetChatId(), 10), false, true
	case p.GetChannelId() != 0:
		return "channel:" + strconv.FormatInt(p.GetChannelId(), 10), false, true
	default:
		return "", false, false
	}
}

func (c *ApiFullCore) MessagesCheckHistoryImport(in *mtproto.TLMessagesCheckHistoryImport) (*mtproto.Messages_HistoryImportParsed, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// No importer/parser provider is wired to inspect the archive header.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesInitHistoryImport(in *mtproto.TLMessagesInitHistoryImport) (*mtproto.Messages_HistoryImport, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetFile() == nil || in.GetMediaCount() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if confirm, _, _ := historyImportPeerFlags(in.GetPeer()); confirm == "" {
		return nil, mtproto.ErrPeerIdInvalid
	}
	// No message-service import job provider is wired to own this session.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesUploadImportedMedia(in *mtproto.TLMessagesUploadImportedMedia) (*mtproto.MessageMedia, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetMedia() == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	if err = miscSave(uid, "MessagesUploadImportedMedia", in); err != nil {
		return nil, err
	}
	if in.GetFileName() != "" {
		if err = persist.Default.Set(historyImportTitleKey(uid), in.GetFileName()); err != nil {
			return nil, err
		}
	}

	if in.GetImportId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	meta, err := loadHistoryImportMeta(uid)
	if err != nil {
		return nil, err
	}
	if meta.ID == 0 || meta.ID != in.GetImportId() {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	confirm, _, _ := historyImportPeerFlags(in.GetPeer())
	if meta.Confirm == "" || confirm != meta.Confirm {
		return nil, mtproto.ErrPeerIdInvalid
	}

	d := c.apifullDao()
	if d == nil || d.DfsClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	media := *in.GetMedia()
	if media.GetFile() == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	if in.GetFileName() != "" {
		file := *media.GetFile()
		file.Name = in.GetFileName()
		media.File = &file
	}

	switch media.GetPredicateName() {
	case mtproto.Predicate_inputMediaUploadedPhoto:
		photo, uploadErr := d.DfsUploadPhotoFileV2(callContext(c), &dfs.TLDfsUploadPhotoFileV2{
			Creator: uid,
			File:    media.GetFile(),
		})
		if uploadErr != nil {
			return nil, uploadErr
		}
		if photo == nil || photo.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Photo_FLAGPHOTO: photo,
			TtlSeconds:      media.GetTtlSeconds(),
		}).To_MessageMedia(), nil
	case mtproto.Predicate_inputMediaUploadedDocument:
		document, uploadErr := d.DfsUploadDocumentFileV2(callContext(c), &dfs.TLDfsUploadDocumentFileV2{
			Creator: uid,
			Media:   &media,
		})
		if uploadErr != nil {
			return nil, uploadErr
		}
		if document == nil || document.GetId() == 0 {
			return nil, mtproto.ErrMediaInvalid
		}
		return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document:   document,
			TtlSeconds: media.GetTtlSeconds(),
		}).To_MessageMedia(), nil
	default:
		return nil, mtproto.ErrMediaInvalid
	}
}

func (c *ApiFullCore) MessagesStartHistoryImport(in *mtproto.TLMessagesStartHistoryImport) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetImportId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if confirm, _, _ := historyImportPeerFlags(in.GetPeer()); confirm == "" {
		return nil, mtproto.ErrPeerIdInvalid
	}
	// No importer worker is wired to consume this request.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesCheckHistoryImportPeer(in *mtproto.TLMessagesCheckHistoryImportPeer) (*mtproto.Messages_CheckedHistoryImportPeer, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if confirm, _, _ := historyImportPeerFlags(in.GetPeer()); confirm == "" {
		return nil, mtproto.ErrPeerIdInvalid
	}
	// No message-service import state provider is wired to check this peer.
	return nil, mtproto.ErrMethodNotImpl
}
