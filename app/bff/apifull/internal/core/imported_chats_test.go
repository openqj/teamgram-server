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
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	dfs "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

type importedMediaDfsClient struct {
	dfs_client.DfsClient
	photo           *mtproto.Photo
	document        *mtproto.Document
	err             error
	photoRequest    *dfs.TLDfsUploadPhotoFileV2
	documentRequest *dfs.TLDfsUploadDocumentFileV2
}

func (c *importedMediaDfsClient) DfsUploadPhotoFileV2(_ context.Context, in *dfs.TLDfsUploadPhotoFileV2) (*mtproto.Photo, error) {
	c.photoRequest = in
	return c.photo, c.err
}

func (c *importedMediaDfsClient) DfsUploadDocumentFileV2(_ context.Context, in *dfs.TLDfsUploadDocumentFileV2) (*mtproto.Document, error) {
	c.documentRequest = in
	return c.document, c.err
}

func importedMediaCore(uid int64, client dfs_client.DfsClient) *ApiFullCore {
	return &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: uid},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			DfsClient: client,
		}},
	}
}

func seedHistoryImportMeta(t *testing.T, uid, importID int64, peer *mtproto.InputPeer) {
	t.Helper()
	confirm, pm, group := historyImportPeerFlags(peer)
	raw, err := json.Marshal(historyImportMeta{ID: importID, Confirm: confirm, Pm: pm, Group: group})
	if err != nil {
		t.Fatal(err)
	}
	if err = persist.Default.Set(historyImportMetaKey(uid), string(raw)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = persist.CompareAndDelete(historyImportMetaKey(uid), string(raw))
	})
}

func TestHistoryImportOrchestrationFailsClosed(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81012}}
	peer := &mtproto.InputPeer{UserId: 81013}
	cases := []struct {
		name string
		call func() (bool, error)
	}{
		{
			name: "check",
			call: func() (bool, error) {
				result, err := c.MessagesCheckHistoryImport(&mtproto.TLMessagesCheckHistoryImport{ImportHead: "export.json"})
				return result != nil, err
			},
		},
		{
			name: "init",
			call: func() (bool, error) {
				result, err := c.MessagesInitHistoryImport(&mtproto.TLMessagesInitHistoryImport{
					Peer:       peer,
					File:       &mtproto.InputFile{Id_INT64: 1, Parts: 1, Name: "export.json"},
					MediaCount: 1,
				})
				return result != nil, err
			},
		},
		{
			name: "start",
			call: func() (bool, error) {
				result, err := c.MessagesStartHistoryImport(&mtproto.TLMessagesStartHistoryImport{Peer: peer, ImportId: 1})
				return result != nil, err
			},
		},
		{
			name: "peer",
			call: func() (bool, error) {
				result, err := c.MessagesCheckHistoryImportPeer(&mtproto.TLMessagesCheckHistoryImportPeer{Peer: peer})
				return result != nil, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if got {
				t.Fatal("result is non-nil, want nil")
			}
			if !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("error = %v, want METHOD_NOT_IMPL", err)
			}
		})
	}
}

func TestHistoryImportOrchestrationValidatesRequests(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81014}}
	cases := []struct {
		name string
		call func() (bool, error)
	}{
		{
			name: "check nil",
			call: func() (bool, error) { result, err := c.MessagesCheckHistoryImport(nil); return result != nil, err },
		},
		{
			name: "init nil",
			call: func() (bool, error) { result, err := c.MessagesInitHistoryImport(nil); return result != nil, err },
		},
		{
			name: "start nil",
			call: func() (bool, error) { result, err := c.MessagesStartHistoryImport(nil); return result != nil, err },
		},
		{
			name: "peer nil",
			call: func() (bool, error) { result, err := c.MessagesCheckHistoryImportPeer(nil); return result != nil, err },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if got {
				t.Fatal("result is non-nil, want nil")
			}
			if !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("error = %v, want INPUT_REQUEST_INVALID", err)
			}
		})
	}
}

func TestImportedChatsRejectsMissingMedia(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81015}}
	got, err := c.MessagesUploadImportedMedia(&mtproto.TLMessagesUploadImportedMedia{FileName: "chat-export-81015"})
	if got != nil || !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("metadata-only upload = (%v, %v), want nil and MEDIA_INVALID", got, err)
	}
}

func TestMessagesUploadImportedMediaUploadsPhoto(t *testing.T) {
	const uid, importID, peerID int64 = 81016, 501, 81017
	peer := &mtproto.InputPeer{UserId: peerID}
	seedHistoryImportMeta(t, uid, importID, peer)
	dfsClient := &importedMediaDfsClient{
		photo: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 7001, AccessHash: 9001}).To_Photo(),
	}
	in := &mtproto.TLMessagesUploadImportedMedia{
		Peer:     peer,
		ImportId: importID,
		FileName: "renamed.jpg",
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: &mtproto.InputFile{Id_INT64: 41, Parts: 1, Name: "source.jpg"},
		}).To_InputMedia(),
	}

	got, err := importedMediaCore(uid, dfsClient).MessagesUploadImportedMedia(in)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetPhoto_FLAGPHOTO() == nil || got.GetPhoto_FLAGPHOTO().GetId() != 7001 {
		t.Fatalf("media = %#v, want photo 7001", got)
	}
	if dfsClient.photoRequest == nil || dfsClient.photoRequest.GetCreator() != uid || dfsClient.photoRequest.GetFile().GetName() != "renamed.jpg" {
		t.Fatalf("DFS request = %#v", dfsClient.photoRequest)
	}
}

func TestMessagesUploadImportedMediaUploadsDocument(t *testing.T) {
	const uid, importID, peerID int64 = 81018, 502, 81019
	peer := &mtproto.InputPeer{UserId: peerID}
	seedHistoryImportMeta(t, uid, importID, peer)
	dfsClient := &importedMediaDfsClient{
		document: mtproto.MakeTLDocument(&mtproto.Document{Id: 7002, AccessHash: 9002}).To_Document(),
	}
	in := &mtproto.TLMessagesUploadImportedMedia{
		Peer:     peer,
		ImportId: importID,
		FileName: "report.pdf",
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:     &mtproto.InputFile{Id_INT64: 42, Parts: 1, Name: "source.bin"},
			MimeType: "application/pdf",
		}).To_InputMedia(),
	}

	got, err := importedMediaCore(uid, dfsClient).MessagesUploadImportedMedia(in)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetDocument() == nil || got.GetDocument().GetId() != 7002 {
		t.Fatalf("media = %#v, want document 7002", got)
	}
	if dfsClient.documentRequest == nil || dfsClient.documentRequest.GetCreator() != uid || dfsClient.documentRequest.GetMedia().GetFile().GetName() != "report.pdf" {
		t.Fatalf("DFS request = %#v", dfsClient.documentRequest)
	}
}

func TestMessagesUploadImportedMediaPropagatesDFSError(t *testing.T) {
	const uid, importID, peerID int64 = 81020, 503, 81021
	peer := &mtproto.InputPeer{UserId: peerID}
	seedHistoryImportMeta(t, uid, importID, peer)
	want := errors.New("dfs unavailable")
	dfsClient := &importedMediaDfsClient{err: want}
	in := &mtproto.TLMessagesUploadImportedMedia{
		Peer:     peer,
		ImportId: importID,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: &mtproto.InputFile{Id_INT64: 43, Parts: 1, Name: "source.jpg"},
		}).To_InputMedia(),
	}

	got, err := importedMediaCore(uid, dfsClient).MessagesUploadImportedMedia(in)
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("result=(%#v, %v), want DFS error", got, err)
	}
}
