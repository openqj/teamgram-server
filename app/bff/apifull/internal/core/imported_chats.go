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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"google.golang.org/protobuf/types/known/wrapperspb"
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
	FileID  int64  `json:"file_id,omitempty"`
	Parts   int32  `json:"parts,omitempty"`
	Name    string `json:"name,omitempty"`
	Media   int32  `json:"media_count,omitempty"`
	Started bool   `json:"started,omitempty"`
}

type historyImportParsedProviderResult struct {
	Pm    bool   `json:"pm"`
	Group bool   `json:"group"`
	Title string `json:"title,omitempty"`
}

type historyImportIDProviderResult struct {
	ImportID int64 `json:"import_id"`
}

type historyImportStartProviderResult struct {
	Accepted bool `json:"accepted"`
}

type historyImportPeerProviderResult struct {
	ConfirmText string `json:"confirm_text"`
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
	if strings.TrimSpace(in.GetImportHead()) == "" || len(in.GetImportHead()) > 1<<20 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var provider historyImportParsedProviderResult
	if err := c.callBusinessProvider("messages.checkHistoryImport", map[string]any{"import_head": in.GetImportHead()}, &provider); err != nil {
		return nil, err
	}
	if !provider.Pm && !provider.Group || provider.Pm && provider.Group {
		return nil, mtproto.ErrInternalServerError
	}
	parsed := mtproto.MakeTLMessagesHistoryImportParsed(&mtproto.Messages_HistoryImportParsed{
		Pm: provider.Pm, Group: provider.Group,
	})
	if strings.TrimSpace(provider.Title) != "" {
		parsed.Data2.Title = wrapperspb.String(provider.Title)
	}
	return parsed.To_Messages_HistoryImportParsed(), nil
}

func (c *ApiFullCore) MessagesInitHistoryImport(in *mtproto.TLMessagesInitHistoryImport) (*mtproto.Messages_HistoryImport, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetFile() == nil || in.GetMediaCount() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if confirm, _, _ := historyImportPeerFlags(in.GetPeer()); confirm == "" {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var provider historyImportIDProviderResult
	if err = c.callBusinessProvider("messages.initHistoryImport", map[string]any{
		"peer": in.GetPeer(), "file": in.GetFile(), "media_count": in.GetMediaCount(),
	}, &provider); err != nil {
		return nil, err
	}
	if provider.ImportID <= 0 {
		return nil, mtproto.ErrInternalServerError
	}
	meta := historyImportMeta{ID: provider.ImportID, Confirm: func() string { v, _, _ := historyImportPeerFlags(in.GetPeer()); return v }(),
		Pm: in.GetPeer().GetUserId() != 0, Group: in.GetPeer().GetUserId() == 0,
		FileID: in.GetFile().GetId_INT64(), Parts: in.GetFile().GetParts(), Name: in.GetFile().GetName(), Media: in.GetMediaCount()}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = persist.Default.Set(historyImportMetaKey(uid), string(b)); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesHistoryImport(&mtproto.Messages_HistoryImport{Id: provider.ImportID}).To_Messages_HistoryImport(), nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetImportId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if confirm, _, _ := historyImportPeerFlags(in.GetPeer()); confirm == "" {
		return nil, mtproto.ErrPeerIdInvalid
	}
	meta, err := loadHistoryImportMeta(uid)
	if err != nil {
		return nil, err
	}
	confirm, _, _ := historyImportPeerFlags(in.GetPeer())
	if meta.ID == 0 || meta.ID != in.GetImportId() || meta.Confirm != confirm {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if meta.Started {
		return mtproto.BoolTrue, nil
	}
	var provider historyImportStartProviderResult
	if err = c.callBusinessProvider("messages.startHistoryImport", map[string]any{"peer": in.GetPeer(), "import_id": in.GetImportId()}, &provider); err != nil {
		return nil, err
	}
	if !provider.Accepted {
		return nil, mtproto.ErrMethodNotImpl
	}
	if err = persist.Update(historyImportMetaKey(uid), func(raw string) (string, error) {
		var current historyImportMeta
		if raw != "" && json.Unmarshal([]byte(raw), &current) != nil {
			return "", mtproto.ErrInternalServerError
		}
		if current.ID != in.GetImportId() || current.Confirm != confirm {
			return "", mtproto.ErrInputRequestInvalid
		}
		current.Started = true
		b, marshalErr := json.Marshal(current)
		return string(b), marshalErr
	}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
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
	var provider historyImportPeerProviderResult
	if err := c.callBusinessProvider("messages.checkHistoryImportPeer", map[string]any{"peer": in.GetPeer()}, &provider); err != nil {
		return nil, err
	}
	if strings.TrimSpace(provider.ConfirmText) == "" || len(provider.ConfirmText) > 4096 {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLMessagesCheckedHistoryImportPeer(&mtproto.Messages_CheckedHistoryImportPeer{ConfirmText: provider.ConfirmText}).To_Messages_CheckedHistoryImportPeer(), nil
}
