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
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestAutoSaveSettingsRoundTrip(t *testing.T) {
	for _, id := range []int64{1, 2} {
		if err := persist.Default.Set(autoSaveKey(id), ""); err != nil {
			t.Fatal(err)
		}
	}
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	anon := &ApiFullCore{}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42}).To_InputPeer()
	users := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{Photos: true, Videos: true}).To_AutoSaveSettings()
	exc := mtproto.MakeTLAutoSaveSettings(&mtproto.AutoSaveSettings{Photos: true}).To_AutoSaveSettings()

	if _, err := anon.AccountGetAutoSaveSettings(&mtproto.TLAccountGetAutoSaveSettings{}); err == nil {
		t.Fatal("get: expected auth error")
	}
	if _, err := anon.AccountSaveAutoSaveSettings(&mtproto.TLAccountSaveAutoSaveSettings{Users: true, Settings: users}); err == nil {
		t.Fatal("save: expected auth error")
	}
	if _, err := anon.AccountDeleteAutoSaveExceptions(&mtproto.TLAccountDeleteAutoSaveExceptions{}); err == nil {
		t.Fatal("delete: expected auth error")
	}
	if _, err := c.AccountDeleteAutoSaveExceptions(&mtproto.TLAccountDeleteAutoSaveExceptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccountSaveAutoSaveSettings(&mtproto.TLAccountSaveAutoSaveSettings{
		Users:    true,
		Settings: users,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccountSaveAutoSaveSettings(&mtproto.TLAccountSaveAutoSaveSettings{
		Peer:     peer,
		Settings: exc,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := c.AccountGetAutoSaveSettings(&mtproto.TLAccountGetAutoSaveSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetUsersSettings() == nil || !got.GetUsersSettings().GetPhotos() || !got.GetUsersSettings().GetVideos() {
		t.Fatalf("users settings = %#v", got.GetUsersSettings())
	}
	if len(got.GetExceptions()) != 1 || got.GetExceptions()[0].GetPeer().GetUserId() != 42 || !got.GetExceptions()[0].GetSettings().GetPhotos() {
		t.Fatalf("exceptions = %#v", got.GetExceptions())
	}

	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 2}}
	otherGot, err := other.AccountGetAutoSaveSettings(&mtproto.TLAccountGetAutoSaveSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if otherGot.GetUsersSettings().GetPhotos() || otherGot.GetUsersSettings().GetVideos() ||
		otherGot.GetChatsSettings().GetPhotos() || otherGot.GetChatsSettings().GetVideos() ||
		otherGot.GetBroadcastsSettings().GetPhotos() || otherGot.GetBroadcastsSettings().GetVideos() ||
		len(otherGot.GetExceptions()) != 0 {
		t.Fatal("other user saw autosave settings")
	}

	if _, err := c.AccountDeleteAutoSaveExceptions(&mtproto.TLAccountDeleteAutoSaveExceptions{}); err != nil {
		t.Fatal(err)
	}
	left, err := c.AccountGetAutoSaveSettings(&mtproto.TLAccountGetAutoSaveSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if len(left.GetExceptions()) != 0 {
		t.Fatalf("exceptions after delete = %d", len(left.GetExceptions()))
	}
	if left.GetUsersSettings() == nil || !left.GetUsersSettings().GetPhotos() {
		t.Fatal("users settings cleared with exceptions")
	}
}
