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

package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type saveMusicUserClient struct {
	userclient.UserClient
	result  *mtproto.Bool
	err     error
	request *userpb.TLUserSaveMusic
	calls   int
}

func (c *saveMusicUserClient) UserSaveMusic(_ context.Context, in *userpb.TLUserSaveMusic) (*mtproto.Bool, error) {
	c.calls++
	c.request = in
	return c.result, c.err
}

func TestAccountSaveMusicForwardsValidRequest(t *testing.T) {
	users := &saveMusicUserClient{result: mtproto.BoolTrue}
	core := newSaveMusicCore(users, 42)

	got, err := core.AccountSaveMusic(&mtproto.TLAccountSaveMusic{
		Unsave:  true,
		Id:      accountSavedMusicInputDocument(10, 100),
		AfterId: accountSavedMusicInputDocument(20, 200),
	})
	if err != nil {
		t.Fatalf("AccountSaveMusic() error = %v", err)
	}
	if got != mtproto.BoolTrue {
		t.Fatalf("AccountSaveMusic() = %v, want BoolTrue", got)
	}
	if users.calls != 1 || users.request == nil {
		t.Fatalf("UserSaveMusic calls/request = (%d, %v), want one request", users.calls, users.request)
	}
	if users.request.GetUserId() != 42 || !users.request.GetUnsave() || users.request.GetId() != 10 || users.request.GetAfterId().GetValue() != 20 {
		t.Fatalf("UserSaveMusic request = %v, want owner 42, unsave, id 10, after 20", users.request)
	}
}

func TestAccountSaveMusicPreservesFalseResult(t *testing.T) {
	users := &saveMusicUserClient{result: mtproto.BoolFalse}
	core := newSaveMusicCore(users, 42)

	got, err := core.AccountSaveMusic(&mtproto.TLAccountSaveMusic{Id: accountSavedMusicInputDocument(10, 100)})
	if err != nil {
		t.Fatalf("AccountSaveMusic() error = %v", err)
	}
	if got != mtproto.BoolFalse {
		t.Fatalf("AccountSaveMusic() = %v, want BoolFalse", got)
	}
	if users.request.GetAfterId() != nil {
		t.Fatalf("absent after_id was forwarded as %v, want nil", users.request.GetAfterId())
	}
}

func TestAccountSaveMusicPropagatesDependencyError(t *testing.T) {
	want := errors.New("save music failed")
	users := &saveMusicUserClient{err: want}
	core := newSaveMusicCore(users, 42)

	got, err := core.AccountSaveMusic(&mtproto.TLAccountSaveMusic{Id: accountSavedMusicInputDocument(10, 100)})
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("AccountSaveMusic() = (%v, %v), want propagated dependency error", got, err)
	}
}

func TestAccountSaveMusicFailsClosedOnNilResponse(t *testing.T) {
	users := &saveMusicUserClient{}
	core := newSaveMusicCore(users, 42)

	got, err := core.AccountSaveMusic(&mtproto.TLAccountSaveMusic{Id: accountSavedMusicInputDocument(10, 100)})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("AccountSaveMusic() = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestAccountSaveMusicRejectsInvalidInputBeforeWrite(t *testing.T) {
	cases := []struct {
		name  string
		user  int64
		input *mtproto.TLAccountSaveMusic
		want  error
	}{
		{name: "unauthenticated", user: 0, input: &mtproto.TLAccountSaveMusic{Id: accountSavedMusicInputDocument(10, 100)}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", user: 42, want: mtproto.ErrInputRequestInvalid},
		{name: "missing document", user: 42, input: &mtproto.TLAccountSaveMusic{}, want: mtproto.ErrDocumentInvalid},
		{name: "empty document", user: 42, input: &mtproto.TLAccountSaveMusic{Id: mtproto.MakeTLInputDocumentEmpty(nil).To_InputDocument()}, want: mtproto.ErrDocumentInvalid},
		{name: "zero access hash", user: 42, input: &mtproto.TLAccountSaveMusic{Id: accountSavedMusicInputDocument(10, 0)}, want: mtproto.ErrDocumentInvalid},
		{name: "invalid after document", user: 42, input: &mtproto.TLAccountSaveMusic{Id: accountSavedMusicInputDocument(10, 100), AfterId: accountSavedMusicInputDocument(0, 200)}, want: mtproto.ErrDocumentInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			users := &saveMusicUserClient{result: mtproto.BoolTrue}
			got, err := newSaveMusicCore(users, tc.user).AccountSaveMusic(tc.input)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("AccountSaveMusic() = (%v, %v), want %v", got, err, tc.want)
			}
			if users.calls != 0 {
				t.Fatalf("UserSaveMusic calls = %d, want none", users.calls)
			}
		})
	}
}

func newSaveMusicCore(users *saveMusicUserClient, userID int64) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: users,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func accountSavedMusicInputDocument(id, accessHash int64) *mtproto.InputDocument {
	return mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: id, AccessHash: accessHash}).To_InputDocument()
}
