package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type chatlistAccessHashUserClient struct {
	user_client.UserClient
	users []*mtproto.ImmutableUser
	calls int
}

func (c *chatlistAccessHashUserClient) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	c.calls++
	return &mtproto.MutableUsers{Users: c.users}, nil
}

func TestChatlistEntitiesRejectsStaleUserAccessHash(t *testing.T) {
	const (
		callerID   = int64(81025001)
		peerID     = int64(81025002)
		accessHash = int64(91025002)
		staleHash  = accessHash + 1
	)
	client := &chatlistAccessHashUserClient{users: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: callerID, AccessHash: callerID}},
		{User: &mtproto.UserData{Id: peerID, AccessHash: accessHash}},
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID, AccessHash: staleHash}).To_InputPeer()

	users, chats, err := c.chatlistEntities(callerID, []*mtproto.InputPeer{peer})
	if users != nil || chats != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("stale user peer = (%v, %v, %v), want USER_ID_INVALID", users, chats, err)
	}
	if client.calls != 1 {
		t.Fatalf("mutable user provider calls = %d, want 1", client.calls)
	}
}
