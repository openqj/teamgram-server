package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type blockedListNilUsersStub struct {
	*blockedListUserClientStub
}

func (*blockedListNilUsersStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return nil, nil
}

func TestContactsGetBlockedRejectsNilUserHydrationResult(t *testing.T) {
	ctx := context.Background()
	client := &blockedListNilUsersStub{blockedListUserClientStub: &blockedListUserClientStub{
		blockedList: &userpb.Vector_PeerBlocked{Datas: []*mtproto.PeerBlocked{mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
			PeerId: mtproto.MakePeerUser(84),
			Date:   456,
		}).To_PeerBlocked()}},
	}}
	core := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	result, err := core.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Limit: 10})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil user hydration result = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}
