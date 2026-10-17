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

func TestContactsGetBlockedRejectsMissingChatProvider(t *testing.T) {
	ctx := context.Background()
	client := &blockedListUserClientStub{blockedList: &userpb.Vector_PeerBlocked{Datas: []*mtproto.PeerBlocked{mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
		PeerId: mtproto.MakePeerChat(12),
		Date:   456,
	}).To_PeerBlocked()}}}
	core := &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	result, err := core.ContactsGetBlocked(&mtproto.TLContactsGetBlocked{Limit: 10})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing chat provider = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}
