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
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type savedMusicIDsUserClient struct {
	userclient.UserClient
	ids     []int64
	err     error
	nilList bool
	request *userpb.TLUserGetSavedMusicIdList
}

func (c *savedMusicIDsUserClient) UserGetSavedMusicIdList(_ context.Context, in *userpb.TLUserGetSavedMusicIdList) (*userpb.Vector_Long, error) {
	c.request = in
	if c.err != nil {
		return nil, c.err
	}
	if c.nilList {
		return nil, nil
	}
	return &userpb.Vector_Long{Datas: append([]int64(nil), c.ids...)}, nil
}

func TestAccountGetSavedMusicIdsReturnsPersistedIDs(t *testing.T) {
	users := &savedMusicIDsUserClient{ids: []int64{30, 10, 20}}
	core := newSavedMusicIDsCore(users, 42)

	got, err := core.AccountGetSavedMusicIds(&mtproto.TLAccountGetSavedMusicIds{})
	if err != nil {
		t.Fatalf("AccountGetSavedMusicIds() error = %v", err)
	}
	if got.GetPredicateName() != mtproto.Predicate_account_savedMusicIds || !reflect.DeepEqual(got.GetIds(), []int64{30, 10, 20}) {
		t.Fatalf("AccountGetSavedMusicIds() = %v, want saved-music IDs [30 10 20]", got)
	}
	if users.request == nil || users.request.GetUserId() != 42 {
		t.Fatalf("saved-music request = %v, want user 42", users.request)
	}
}

func TestAccountGetSavedMusicIdsReturnsNotModifiedForMatchingHash(t *testing.T) {
	users := &savedMusicIDsUserClient{ids: []int64{30, 10, 20}}
	core := newSavedMusicIDsCore(users, 42)

	got, err := core.AccountGetSavedMusicIds(&mtproto.TLAccountGetSavedMusicIds{
		Hash: 5652242728387261,
	})
	if err != nil {
		t.Fatalf("AccountGetSavedMusicIds() error = %v", err)
	}
	if got.GetPredicateName() != mtproto.Predicate_account_savedMusicIdsNotModified || len(got.GetIds()) != 0 {
		t.Fatalf("AccountGetSavedMusicIds() = %v, want savedMusicIdsNotModified", got)
	}
}

func TestSavedMusicIDsHashIgnoresOrder(t *testing.T) {
	if got := savedMusicIDsHash([]int64{30, 10, 20}); got != 5652242728387261 {
		t.Fatalf("savedMusicIDsHash([30 10 20]) = %d, want 5652242728387261", got)
	}
	if got := savedMusicIDsHash([]int64{10, 20, 30}); got != 5652242728387261 {
		t.Fatalf("savedMusicIDsHash([10 20 30]) = %d, want 5652242728387261", got)
	}
}

func TestAccountGetSavedMusicIdsFailsClosedForMissingResponse(t *testing.T) {
	users := &savedMusicIDsUserClient{nilList: true}
	core := newSavedMusicIDsCore(users, 42)

	got, err := core.AccountGetSavedMusicIds(&mtproto.TLAccountGetSavedMusicIds{})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("AccountGetSavedMusicIds() = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestAccountGetSavedMusicIdsRejectsUnauthenticatedAndNilRequests(t *testing.T) {
	users := &savedMusicIDsUserClient{}

	got, err := newSavedMusicIDsCore(users, 0).AccountGetSavedMusicIds(&mtproto.TLAccountGetSavedMusicIds{})
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated AccountGetSavedMusicIds() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}

	got, err = newSavedMusicIDsCore(users, 42).AccountGetSavedMusicIds(nil)
	if got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil AccountGetSavedMusicIds() = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}

func newSavedMusicIDsCore(users *savedMusicIDsUserClient, userID int64) *UserChannelProfilesCore {
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
