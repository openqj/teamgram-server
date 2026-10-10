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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type inlinePeerValidationUserClient struct {
	user_client.UserClient
	users map[int64]*mtproto.ImmutableUser
}

func (c *inlinePeerValidationUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if profile := c.users[in.GetId()]; profile != nil {
		return profile, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

func TestValidateInlineQueryPeer(t *testing.T) {
	const (
		callerID = int64(1001)
		peerID   = int64(1002)
		peerHash = int64(2002)
	)
	client := &inlinePeerValidationUserClient{users: map[int64]*mtproto.ImmutableUser{
		peerID: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
			Id: peerID, AccessHash: peerHash,
		}}).To_ImmutableUser(),
	}}
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient: client,
		}},
	}

	valid := []struct {
		name string
		peer *mtproto.InputPeer
	}{
		{name: "self", peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()},
		{name: "user", peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID, AccessHash: peerHash}).To_InputPeer()},
		{name: "chat", peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 3001}).To_InputPeer()},
		{name: "channel", peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 4001, AccessHash: 5001}).To_InputPeer()},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.validateInlineQueryPeer(callerID, tc.peer); err != nil {
				t.Fatalf("validate %s peer: %v", tc.name, err)
			}
		})
	}

	invalid := []struct {
		name string
		peer *mtproto.InputPeer
		want error
	}{
		{name: "nil", want: mtproto.ErrInputRequestInvalid},
		{name: "empty", peer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(), want: mtproto.ErrInputConstructorInvalid},
		{name: "user missing id", peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{AccessHash: peerHash}).To_InputPeer(), want: mtproto.ErrUserIdInvalid},
		{name: "user missing hash", peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID}).To_InputPeer(), want: mtproto.ErrUserIdInvalid},
		{name: "user wrong hash", peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID, AccessHash: peerHash + 1}).To_InputPeer(), want: mtproto.ErrUserIdInvalid},
		{name: "chat missing id", peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{}).To_InputPeer(), want: mtproto.ErrPeerIdInvalid},
		{name: "channel missing hash", peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 4001}).To_InputPeer(), want: mtproto.ErrChannelInvalid},
		{name: "user from message", peer: mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{}).To_InputPeer(), want: mtproto.ErrMethodNotImpl},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.validateInlineQueryPeer(callerID, tc.peer); !errors.Is(err, tc.want) {
				t.Fatalf("validate %s peer: got %v, want %v", tc.name, err, tc.want)
			}
		})
	}
}

func TestInlineQueryGeoKey(t *testing.T) {
	key, err := inlineQueryGeoKey(&mtproto.InputGeoPoint{Lat: 31.2304, Long: 121.4737})
	if err != nil || key != "31.2304000,121.4737000,0" {
		t.Fatalf("geo key = %q, err=%v", key, err)
	}
	if key, err = inlineQueryGeoKey(nil); err != nil || key != "" {
		t.Fatalf("nil geo key = %q, err=%v", key, err)
	}
	for _, point := range []*mtproto.InputGeoPoint{
		{Lat: 91},
		{Long: -181},
		{Lat: 1, Long: 2, AccuracyRadius: wrapperspb.Int32(-1)},
	} {
		if _, err = inlineQueryGeoKey(point); !errors.Is(err, mtproto.ErrGeoPointInvalid) {
			t.Fatalf("invalid geo point %+v: err=%v", point, err)
		}
	}
}
