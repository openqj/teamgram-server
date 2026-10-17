package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type setBlockedUserClientStub struct {
	userclient.UserClient
	blockedList *userpb.Vector_PeerBlocked
	blockedErr  error
	listCalls   int
}

func (s *setBlockedUserClientStub) UserGetBlockedList(_ context.Context, _ *userpb.TLUserGetBlockedList) (*userpb.Vector_PeerBlocked, error) {
	s.listCalls++
	return s.blockedList, s.blockedErr
}

func TestContactsSetBlockedFailsClosedOnNilBlockedList(t *testing.T) {
	ctx := context.Background()
	client := &setBlockedUserClientStub{}
	core := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	got, err := core.ContactsSetBlocked(&mtproto.TLContactsSetBlocked{
		Id: []*mtproto.InputPeer{
			mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84}).To_InputPeer(),
		},
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("ContactsSetBlocked() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
	}
	if client.listCalls != 1 {
		t.Fatalf("user.getBlockedList calls = %d, want 1", client.listCalls)
	}
}

func TestContactsSetBlockedPropagatesBlockedListProviderError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("blocked list unavailable")
	client := &setBlockedUserClientStub{blockedErr: wantErr}
	core := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	got, err := core.ContactsSetBlocked(&mtproto.TLContactsSetBlocked{
		Id: []*mtproto.InputPeer{
			mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84}).To_InputPeer(),
		},
	})
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("ContactsSetBlocked() = (%v, %v), want nil and provider error", got, err)
	}
}
