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

type blockUserClientStub struct {
	userclient.UserClient
	users      *userpb.Vector_ImmutableUser
	blockReply *mtproto.Bool
	blockErr   error
	blockCalls int
}

func (s *blockUserClientStub) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, nil
}

func (s *blockUserClientStub) UserBlockPeer(_ context.Context, _ *userpb.TLUserBlockPeer) (*mtproto.Bool, error) {
	s.blockCalls++
	return s.blockReply, s.blockErr
}

type blockSyncClientStub struct {
	syncclient.SyncClient
	request *syncpb.TLSyncUpdatesNotMe
	err     error
}

func (s *blockSyncClientStub) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.request = in
	return mtproto.EmptyVoid, s.err
}

func TestContactsBlockFailsWhenUserServiceDoesNotPersist(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	userClient := &blockUserClientStub{
		users:      blockTestUsers(selfID, peerID),
		blockReply: mtproto.BoolFalse,
	}
	syncClient := &blockSyncClientStub{}
	core := newBlockTestCore(selfID, userClient, syncClient)

	got, err := core.ContactsBlock(blockTestRequest(peerID))
	if got != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("ContactsBlock() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
	if userClient.blockCalls != 1 || syncClient.request != nil {
		t.Fatalf("block calls = %d, sync request = %v; want one block attempt and no update", userClient.blockCalls, syncClient.request)
	}
}

func TestContactsBlockPropagatesSyncFailureAfterPersistence(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	syncErr := errors.New("sync unavailable")
	userClient := &blockUserClientStub{
		users:      blockTestUsers(selfID, peerID),
		blockReply: mtproto.BoolTrue,
	}
	syncClient := &blockSyncClientStub{err: syncErr}
	core := newBlockTestCore(selfID, userClient, syncClient)

	got, err := core.ContactsBlock(blockTestRequest(peerID))
	if got != nil || err != syncErr {
		t.Fatalf("ContactsBlock() = (%v, %v), want nil and sync error", got, err)
	}
	if userClient.blockCalls != 1 || syncClient.request == nil {
		t.Fatalf("block calls = %d, sync request = %v; want persisted block and attempted update", userClient.blockCalls, syncClient.request)
	}
}

func TestContactsBlockReturnsSuccessAfterPersistenceAndSync(t *testing.T) {
	const (
		selfID     = int64(42)
		peerID     = int64(84)
		permAuthID = int64(126)
	)
	userClient := &blockUserClientStub{
		users:      blockTestUsers(selfID, peerID),
		blockReply: mtproto.BoolTrue,
	}
	syncClient := &blockSyncClientStub{}
	core := newBlockTestCore(selfID, userClient, syncClient)
	core.MD.PermAuthKeyId = permAuthID

	got, err := core.ContactsBlock(blockTestRequest(peerID))
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("ContactsBlock() = (%v, %v), want BoolTrue", got, err)
	}
	if userClient.blockCalls != 1 || syncClient.request == nil || syncClient.request.UserId != selfID || syncClient.request.PermAuthKeyId != permAuthID {
		t.Fatalf("block calls = %d, sync request = %+v; want one persisted block and sync to other sessions", userClient.blockCalls, syncClient.request)
	}
}

func blockTestRequest(peerID int64) *mtproto.TLContactsBlock {
	return &mtproto.TLContactsBlock{Id: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID}).To_InputPeer()}
}

func blockTestUsers(selfID, peerID int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Owner"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: peerID, FirstName: "Peer"}}).To_ImmutableUser(),
	}}
}

func newBlockTestCore(selfID int64, userClient *blockUserClientStub, syncClient *blockSyncClientStub) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient, SyncClient: syncClient}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: selfID},
	}
}
