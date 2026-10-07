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

type birthdayUserClient struct {
	userclient.UserClient
	result  *mtproto.Bool
	err     error
	request *userpb.TLUserUpdateBirthday
	calls   int
}

func (c *birthdayUserClient) UserUpdateBirthday(_ context.Context, in *userpb.TLUserUpdateBirthday) (*mtproto.Bool, error) {
	c.calls++
	c.request = in
	return c.result, c.err
}

func TestAccountUpdateBirthdayForwardsAuthenticatedRequest(t *testing.T) {
	birthday := mtproto.MakeTLBirthday(&mtproto.Birthday{
		Year:  mtproto.MakeFlagsInt32(1990),
		Month: 1,
		Day:   2,
	}).To_Birthday()
	users := &birthdayUserClient{result: mtproto.BoolTrue}

	got, err := newBirthdayCore(users, 42).AccountUpdateBirthday(&mtproto.TLAccountUpdateBirthday{Birthday: birthday})
	if err != nil {
		t.Fatalf("AccountUpdateBirthday() error = %v", err)
	}
	if got != mtproto.BoolTrue {
		t.Fatalf("AccountUpdateBirthday() = %v, want BoolTrue", got)
	}
	if users.calls != 1 || users.request == nil {
		t.Fatalf("UserUpdateBirthday calls/request = (%d, %v), want one request", users.calls, users.request)
	}
	if users.request.GetUserId() != 42 || users.request.GetBirthday().GetYear().GetValue() != 1990 || users.request.GetBirthday().GetMonth() != 1 || users.request.GetBirthday().GetDay() != 2 {
		t.Fatalf("UserUpdateBirthday request = %v, want user 42 and 1990-01-02", users.request)
	}
}

func TestAccountUpdateBirthdayPreservesFalseResultAndClearsBirthday(t *testing.T) {
	users := &birthdayUserClient{result: mtproto.BoolFalse}

	got, err := newBirthdayCore(users, 42).AccountUpdateBirthday(&mtproto.TLAccountUpdateBirthday{})
	if err != nil {
		t.Fatalf("AccountUpdateBirthday() error = %v", err)
	}
	if got != mtproto.BoolFalse {
		t.Fatalf("AccountUpdateBirthday() = %v, want BoolFalse", got)
	}
	if users.request == nil || users.request.GetBirthday() != nil {
		t.Fatalf("UserUpdateBirthday birthday = %v, want nil clear request", users.request.GetBirthday())
	}
}

func TestAccountUpdateBirthdayPropagatesDependencyError(t *testing.T) {
	want := errors.New("birthday update failed")
	users := &birthdayUserClient{err: want}

	got, err := newBirthdayCore(users, 42).AccountUpdateBirthday(&mtproto.TLAccountUpdateBirthday{})
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("AccountUpdateBirthday() = (%v, %v), want propagated dependency error", got, err)
	}
}

func TestAccountUpdateBirthdayFailsClosedOnNilResponse(t *testing.T) {
	users := &birthdayUserClient{}

	got, err := newBirthdayCore(users, 42).AccountUpdateBirthday(&mtproto.TLAccountUpdateBirthday{})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("AccountUpdateBirthday() = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestAccountUpdateBirthdayRejectsInvalidBoundaryBeforeWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		user int64
		in   *mtproto.TLAccountUpdateBirthday
		want error
	}{
		{name: "unauthenticated", user: 0, in: &mtproto.TLAccountUpdateBirthday{}, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", user: 42, want: mtproto.ErrInputRequestInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := &birthdayUserClient{result: mtproto.BoolTrue}
			got, err := newBirthdayCore(users, tc.user).AccountUpdateBirthday(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("AccountUpdateBirthday() = (%v, %v), want %v", got, err, tc.want)
			}
			if users.calls != 0 {
				t.Fatalf("UserUpdateBirthday calls = %d, want none", users.calls)
			}
		})
	}
}

func newBirthdayCore(users *birthdayUserClient, userID int64) *UserChannelProfilesCore {
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
