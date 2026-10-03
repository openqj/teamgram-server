package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type unblockUserClientStub struct {
	userclient.UserClient
	users        *userpb.Vector_ImmutableUser
	result       *mtproto.Bool
	unblockErr   error
	unblock      *userpb.TLUserUnBlockPeer
	unblockCalls int
}

func (s *unblockUserClientStub) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, nil
}

func (s *unblockUserClientStub) UserUnBlockPeer(_ context.Context, in *userpb.TLUserUnBlockPeer) (*mtproto.Bool, error) {
	s.unblockCalls++
	s.unblock = in
	return s.result, s.unblockErr
}

type unblockSyncClientStub struct {
	syncclient.SyncClient
	request *syncpb.TLSyncUpdatesNotMe
	err     error
}

func (s *unblockSyncClientStub) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.request = in
	return mtproto.EmptyVoid, s.err
}

func TestContactsUnblockFailsWhenUserServiceDoesNotConfirmWrite(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	userClient := &unblockUserClientStub{
		users:  unblockTestUsers(selfID, peerID),
		result: mtproto.BoolFalse,
	}
	syncClient := &unblockSyncClientStub{}
	core := newUnblockTestCore(selfID, userClient, syncClient)

	got, err := core.ContactsUnblock(unblockTestRequest(peerID))
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("ContactsUnblock() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
	if userClient.unblockCalls != 1 || syncClient.request != nil {
		t.Fatalf("unblock calls = %d, sync request = %v; want one unblock attempt and no update", userClient.unblockCalls, syncClient.request)
	}
}

func TestContactsUnblockPropagatesSyncFailureAfterWrite(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	syncErr := errors.New("sync unavailable")
	userClient := &unblockUserClientStub{
		users:  unblockTestUsers(selfID, peerID),
		result: mtproto.BoolTrue,
	}
	syncClient := &unblockSyncClientStub{err: syncErr}
	core := newUnblockTestCore(selfID, userClient, syncClient)

	got, err := core.ContactsUnblock(unblockTestRequest(peerID))
	if got != nil || err != syncErr {
		t.Fatalf("ContactsUnblock() = (%v, %v), want nil and sync error", got, err)
	}
	if userClient.unblockCalls != 1 || syncClient.request == nil {
		t.Fatalf("unblock calls = %d, sync request = %v; want persisted unblock and attempted update", userClient.unblockCalls, syncClient.request)
	}
}

func TestContactsUnblockReturnsSuccessAfterWriteAndSync(t *testing.T) {
	const (
		selfID     = int64(42)
		peerID     = int64(84)
		permAuthID = int64(126)
	)
	userClient := &unblockUserClientStub{
		users:  unblockTestUsers(selfID, peerID),
		result: mtproto.BoolTrue,
	}
	syncClient := &unblockSyncClientStub{}
	core := newUnblockTestCore(selfID, userClient, syncClient)
	core.MD.PermAuthKeyId = permAuthID

	got, err := core.ContactsUnblock(unblockTestRequest(peerID))
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("ContactsUnblock() = (%v, %v), want BoolTrue", got, err)
	}
	if userClient.unblockCalls != 1 || userClient.unblock == nil || userClient.unblock.UserId != selfID || userClient.unblock.PeerId != peerID {
		t.Fatalf("unblock calls = %d, request = %+v; want one unblock from %d for %d", userClient.unblockCalls, userClient.unblock, selfID, peerID)
	}
	if syncClient.request == nil || syncClient.request.UserId != selfID || syncClient.request.PermAuthKeyId != permAuthID {
		t.Fatalf("sync request = %+v; want delivery to other sessions for user %d", syncClient.request, selfID)
	}
}

func unblockTestRequest(peerID int64) *mtproto.TLContactsUnblock {
	return &mtproto.TLContactsUnblock{Id: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID}).To_InputPeer()}
}

func unblockTestUsers(selfID, peerID int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Owner"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: peerID, FirstName: "Peer"}}).To_ImmutableUser(),
	}}
}

func newUnblockTestCore(selfID int64, userClient *unblockUserClientStub, syncClient *unblockSyncClientStub) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient, SyncClient: syncClient}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: selfID},
	}
}
