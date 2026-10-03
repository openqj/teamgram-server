package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type sendAsUserClientStub struct {
	userclient.UserClient
	userID int64
}

func (s *sendAsUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: s.userID}}).To_ImmutableUser(),
	}}, nil
}

func TestChannelsGetSendAsFiltersUnownedPreference(t *testing.T) {
	const userID int64 = 972341
	key := "default_send_as:972341:1:972342"
	if err := persist.Default.Set(key, "2:972343"); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	core := &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: &sendAsUserClientStub{userID: userID}}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
	got, err := core.ChannelsGetSendAs(&mtproto.TLChannelsGetSendAs{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 972342}).To_InputPeer(),
	})
	if err != nil {
		t.Fatalf("ChannelsGetSendAs() error = %v", err)
	}
	if got == nil || len(got.GetPeers_VECTORSENDASPEER()) != 1 || len(got.GetChats()) != 0 {
		t.Fatalf("ChannelsGetSendAs() = %+v, want only self", got)
	}
	if peer := got.GetPeers_VECTORSENDASPEER()[0].GetPeer(); peer == nil || peer.GetUserId() != userID {
		t.Fatalf("self send-as peer = %+v", peer)
	}
}

func TestChannelsGetSendAsRejectsMissingRequest(t *testing.T) {
	ctx := context.Background()
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 1}, Logger: logx.WithContext(ctx)}
	if _, err := core.ChannelsGetSendAs(nil); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("missing request error = %v, want PEER_ID_INVALID", err)
	}
}

func TestChannelsGetSendAsRejectsUnsupportedFlags(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 1}}
	request := &mtproto.TLChannelsGetSendAs{
		ForPaidReactions: true,
		Peer:             mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(),
	}
	if _, err := core.ChannelsGetSendAs(request); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("paid-reactions flag error = %v, want METHOD_NOT_IMPL", err)
	}
}
