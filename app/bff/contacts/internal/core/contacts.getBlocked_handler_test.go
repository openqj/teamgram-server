package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
)

type blockedListUserClientStub struct {
	userclient.UserClient
	blockedList *userpb.Vector_PeerBlocked
	request     *userpb.TLUserGetBlockedList
}

func (s *blockedListUserClientStub) UserGetBlockedList(_ context.Context, in *userpb.TLUserGetBlockedList) (*userpb.Vector_PeerBlocked, error) {
	s.request = in
	return s.blockedList, nil
}

func (s *blockedListUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{}}, nil
}

func TestContactsGetBlockedPassesOffsetAndCapsLimit(t *testing.T) {
	ctx := context.Background()
	blockedList := &userpb.Vector_PeerBlocked{TotalCount: 73}
	wire, err := proto.Marshal(blockedList)
	if err != nil {
		t.Fatal(err)
	}
	decodedList := &userpb.Vector_PeerBlocked{}
	if err := proto.Unmarshal(wire, decodedList); err != nil {
		t.Fatal(err)
	}
	client := &blockedListUserClientStub{blockedList: decodedList}
	core := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	got, err := core.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Offset: 7, Limit: 60})
	if err != nil || got == nil {
		t.Fatalf("ContactsGetBlocked() = (%+v, %v), want empty result", got, err)
	}
	if client.request == nil || client.request.UserId != 42 || client.request.Offset != 7 || client.request.Limit != 50 {
		t.Fatalf("user.getBlockedList request = %+v, want user=42 offset=7 limit=50", client.request)
	}
	if got.PredicateName != mtproto.Predicate_contacts_blockedSlice || got.Count != 73 {
		t.Fatalf("blocked response = predicate %q count %d, want blockedSlice count 73", got.PredicateName, got.Count)
	}
}

func TestContactsGetBlockedFailsClosedWithoutChannelHydration(t *testing.T) {
	ctx := context.Background()
	client := &blockedListUserClientStub{blockedList: &userpb.Vector_PeerBlocked{
		Datas: []*mtproto.PeerBlocked{mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
			PeerId: mtproto.MakePeerChannel(123),
			Date:   456,
		}).To_PeerBlocked()},
	}}
	core := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	got, err := core.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Limit: 10})
	if err == nil || got != nil {
		t.Fatalf("ContactsGetBlocked() = (%+v, %v), want an error without a partial response", got, err)
	}
}
