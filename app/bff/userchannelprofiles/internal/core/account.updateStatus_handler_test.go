package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type statusUserClient struct {
	userclient.UserClient
	sequence *[]string
	err      error
	request  *userpb.TLUserUpdateLastSeen
}

func (c *statusUserClient) UserUpdateLastSeen(_ context.Context, in *userpb.TLUserUpdateLastSeen) (*mtproto.Bool, error) {
	*c.sequence = append(*c.sequence, "persist")
	c.request = in
	return mtproto.BoolTrue, c.err
}

type statusSyncClient struct {
	syncclient.SyncClient
	sequence *[]string
	err      error
	request  *sync.TLSyncUpdatesNotMe
}

func (c *statusSyncClient) SyncUpdatesNotMe(_ context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	*c.sequence = append(*c.sequence, "push")
	c.request = in
	return mtproto.EmptyVoid, c.err
}

func TestAccountUpdateStatusPushesToOtherSessions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		offline    *mtproto.Bool
		wantOnline bool
	}{
		{name: "online", offline: mtproto.BoolFalse, wantOnline: true},
		{name: "offline", offline: mtproto.BoolTrue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sequence := []string{}
			users := &statusUserClient{sequence: &sequence}
			syncer := &statusSyncClient{sequence: &sequence}
			core := newStatusCore(users, syncer)

			got, err := core.AccountUpdateStatus(&mtproto.TLAccountUpdateStatus{Offline: tc.offline})
			if err != nil {
				t.Fatalf("AccountUpdateStatus() error = %v", err)
			}
			if got != mtproto.BoolTrue {
				t.Fatalf("AccountUpdateStatus() = %v, want BoolTrue", got)
			}
			if len(sequence) != 2 || sequence[0] != "persist" || sequence[1] != "push" {
				t.Fatalf("call order = %v, want [persist push]", sequence)
			}
			if users.request.GetId() != 42 || syncer.request.GetUserId() != 42 {
				t.Fatalf("status owner mismatch: persist=%d push=%d", users.request.GetId(), syncer.request.GetUserId())
			}
			if syncer.request.GetPermAuthKeyId() != 9001 {
				t.Fatalf("excluded auth key = %d, want 9001", syncer.request.GetPermAuthKeyId())
			}

			short := syncer.request.GetUpdates().To_UpdateShort()
			if short == nil || short.GetData2() == nil || short.GetData2().GetUpdate() == nil {
				t.Fatalf("status push has no updateShort payload: %v", syncer.request.GetUpdates())
			}
			update := short.GetData2().GetUpdate()
			if update.GetUserId() != 42 {
				t.Fatalf("update user_id = %d, want 42", update.GetUserId())
			}
			if tc.wantOnline {
				if update.GetStatus_USERSTATUS().GetExpires() == 0 || users.request.GetExpires() == 0 {
					t.Fatalf("online status expiry missing: update=%v persisted=%v", update.GetStatus_USERSTATUS(), users.request)
				}
			} else {
				if update.GetStatus_USERSTATUS().GetWasOnline() == 0 || users.request.GetExpires() != 0 {
					t.Fatalf("offline status mismatch: update=%v persisted=%v", update.GetStatus_USERSTATUS(), users.request)
				}
			}
		})
	}
}

func TestAccountUpdateStatusStopsWhenPersistenceFails(t *testing.T) {
	storeErr := errors.New("persist failed")
	sequence := []string{}
	users := &statusUserClient{sequence: &sequence, err: storeErr}
	syncer := &statusSyncClient{sequence: &sequence}
	core := newStatusCore(users, syncer)

	_, err := core.AccountUpdateStatus(&mtproto.TLAccountUpdateStatus{Offline: mtproto.BoolFalse})
	if !errors.Is(err, storeErr) {
		t.Fatalf("AccountUpdateStatus() error = %v, want %v", err, storeErr)
	}
	if len(sequence) != 1 || sequence[0] != "persist" {
		t.Fatalf("calls after persistence failure = %v, want [persist]", sequence)
	}
}

func TestAccountUpdateStatusReturnsPushFailure(t *testing.T) {
	pushErr := errors.New("push failed")
	sequence := []string{}
	users := &statusUserClient{sequence: &sequence}
	syncer := &statusSyncClient{sequence: &sequence, err: pushErr}
	core := newStatusCore(users, syncer)

	_, err := core.AccountUpdateStatus(&mtproto.TLAccountUpdateStatus{Offline: mtproto.BoolFalse})
	if !errors.Is(err, pushErr) {
		t.Fatalf("AccountUpdateStatus() error = %v, want %v", err, pushErr)
	}
	if len(sequence) != 2 || sequence[0] != "persist" || sequence[1] != "push" {
		t.Fatalf("calls after push failure = %v, want [persist push]", sequence)
	}
}

func newStatusCore(users *statusUserClient, syncer *statusSyncClient) *UserChannelProfilesCore {
	return &UserChannelProfilesCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{
				UserClient: users,
				SyncClient: syncer,
			},
		},
		Logger: logx.WithContext(context.Background()),
		MD: &metadata.RpcMetadata{
			UserId:        42,
			PermAuthKeyId: 9001,
		},
	}
}
