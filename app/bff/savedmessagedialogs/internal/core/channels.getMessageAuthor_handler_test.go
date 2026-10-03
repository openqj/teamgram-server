package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type messageAuthorMessageClient struct {
	messageclient.MessageClient
	box   *mtproto.MessageBox
	err   error
	calls int
}

func (c *messageAuthorMessageClient) MessageGetUserMessage(_ context.Context, _ *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	c.calls++
	return c.box, c.err
}

type messageAuthorUserClient struct {
	userclient.UserClient
	users *userpb.Vector_ImmutableUser
	err   error
	calls int
}

func (c *messageAuthorUserClient) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	c.calls++
	return c.users, c.err
}

func newMessageAuthorCore(messages *messageAuthorMessageClient, users *messageAuthorUserClient, userID int64) *SavedMessageDialogsCore {
	ctx := context.Background()
	return &SavedMessageDialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &bffdao.Dao{
			MessageClient: messages,
			UserClient:    users,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func messageAuthorUsers(selfID, authorID int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Self"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: authorID, FirstName: "Author"}}).To_ImmutableUser(),
	}}
}

func channelAuthorMessage(channelID, authorID int64) *mtproto.MessageBox {
	return &mtproto.MessageBox{
		PeerType: mtproto.PEER_CHANNEL,
		PeerId:   channelID,
		Message: &mtproto.Message{
			FromId: mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: authorID}).To_Peer(),
		},
	}
}

func messageAuthorRequest(channelID int64, msgID int32) *mtproto.TLChannelsGetMessageAuthor {
	return &mtproto.TLChannelsGetMessageAuthor{
		Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: 77}).To_InputChannel(),
		Id:      msgID,
	}
}

func TestChannelsGetMessageAuthorResolvesStoredChannelAuthor(t *testing.T) {
	const selfID, channelID, authorID int64 = 42, 9001, 77
	messages := &messageAuthorMessageClient{box: channelAuthorMessage(channelID, authorID)}
	users := &messageAuthorUserClient{users: messageAuthorUsers(selfID, authorID)}

	got, err := newMessageAuthorCore(messages, users, selfID).ChannelsGetMessageAuthor(messageAuthorRequest(channelID, 12))
	if err != nil {
		t.Fatalf("ChannelsGetMessageAuthor() error = %v", err)
	}
	if got == nil || got.GetId() != authorID || got.GetFirstName().GetValue() != "Author" {
		t.Fatalf("ChannelsGetMessageAuthor() = %v, want author %d", got, authorID)
	}
	if messages.calls != 1 || users.calls != 1 {
		t.Fatalf("provider calls = message %d user %d, want one each", messages.calls, users.calls)
	}
}

func TestChannelsGetMessageAuthorReturnsEmptyForNonChannelOrAnonymousMessage(t *testing.T) {
	const selfID, channelID int64 = 42, 9001
	for name, box := range map[string]*mtproto.MessageBox{
		"other peer": {PeerType: mtproto.PEER_USER, PeerId: 88},
		"channel author": {
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   channelID,
			Message:  &mtproto.Message{FromId: mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: 33}).To_Peer()},
		},
	} {
		t.Run(name, func(t *testing.T) {
			messages := &messageAuthorMessageClient{box: box}
			users := &messageAuthorUserClient{users: messageAuthorUsers(selfID, 77)}
			got, err := newMessageAuthorCore(messages, users, selfID).ChannelsGetMessageAuthor(messageAuthorRequest(channelID, 12))
			if err != nil {
				t.Fatalf("ChannelsGetMessageAuthor() error = %v", err)
			}
			if got == nil || got.GetPredicateName() != mtproto.Predicate_userEmpty {
				t.Fatalf("ChannelsGetMessageAuthor() = %v, want userEmpty", got)
			}
			if users.calls != 0 {
				t.Fatalf("UserGetMutableUsers calls = %d, want 0", users.calls)
			}
		})
	}
}

func TestChannelsGetMessageAuthorFailsClosedOnAuthAndProviderErrors(t *testing.T) {
	request := messageAuthorRequest(9001, 12)
	unauthenticated := newMessageAuthorCore(&messageAuthorMessageClient{}, &messageAuthorUserClient{}, 0)
	if got, err := unauthenticated.ChannelsGetMessageAuthor(request); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}

	missingProvider := &SavedMessageDialogsCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &bffdao.Dao{}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	if got, err := missingProvider.ChannelsGetMessageAuthor(request); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing message provider = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}

	core := newMessageAuthorCore(&messageAuthorMessageClient{err: errors.New("message unavailable")}, &messageAuthorUserClient{}, 42)
	if got, err := core.ChannelsGetMessageAuthor(request); got == nil || got.GetPredicateName() != mtproto.Predicate_userEmpty || err != nil {
		t.Fatalf("message provider error = (%v, %v), want empty user and nil error", got, err)
	}

	wantUserErr := errors.New("user unavailable")
	core = newMessageAuthorCore(&messageAuthorMessageClient{box: channelAuthorMessage(channelIDForAuthorTest, 77)}, &messageAuthorUserClient{err: wantUserErr}, 42)
	if got, err := core.ChannelsGetMessageAuthor(messageAuthorRequest(channelIDForAuthorTest, 12)); got != nil || !errors.Is(err, wantUserErr) {
		t.Fatalf("user provider error = (%v, %v), want propagated error", got, err)
	}
}

const channelIDForAuthorTest int64 = 9001

func TestChannelsGetMessageAuthorRejectsNilRequest(t *testing.T) {
	core := newMessageAuthorCore(&messageAuthorMessageClient{}, &messageAuthorUserClient{}, 42)
	if got, err := core.ChannelsGetMessageAuthor(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}
