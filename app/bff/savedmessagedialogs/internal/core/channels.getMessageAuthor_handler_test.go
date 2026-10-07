package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	bffdao "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type messageAuthorResolver struct {
	authorID  int64
	err       error
	calls     int
	userID    int64
	channelID int64
	messageID int32
}

func (r *messageAuthorResolver) resolve(userID int64, channel *mtproto.InputChannel, id int32) (int64, error) {
	r.calls++
	r.userID = userID
	if channel != nil {
		r.channelID = channel.GetChannelId()
	}
	r.messageID = id
	return r.authorID, r.err
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

func newMessageAuthorCore(resolver *messageAuthorResolver, users *messageAuthorUserClient, userID int64) *SavedMessageDialogsCore {
	ctx := context.Background()
	return &SavedMessageDialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &bffdao.Dao{
			UserClient: users,
		}},
		Logger:               logx.WithContext(ctx),
		MD:                   &metadata.RpcMetadata{UserId: userID},
		channelMessageAuthor: resolver.resolve,
	}
}

func messageAuthorUsers(selfID, authorID int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Self"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: authorID, FirstName: "Author"}}).To_ImmutableUser(),
	}}
}

func messageAuthorRequest(channelID int64, msgID int32) *mtproto.TLChannelsGetMessageAuthor {
	return &mtproto.TLChannelsGetMessageAuthor{
		Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: 77}).To_InputChannel(),
		Id:      msgID,
	}
}

func TestChannelsGetMessageAuthorResolvesNativeChannelMessageID(t *testing.T) {
	const selfID, channelID, messageID, authorID int64 = 42, 9001, 12, 77
	resolver := &messageAuthorResolver{authorID: authorID}
	users := &messageAuthorUserClient{users: messageAuthorUsers(selfID, authorID)}

	got, err := newMessageAuthorCore(resolver, users, selfID).ChannelsGetMessageAuthor(messageAuthorRequest(channelID, int32(messageID)))
	if err != nil {
		t.Fatalf("ChannelsGetMessageAuthor() error = %v", err)
	}
	if got == nil || got.GetId() != authorID || got.GetFirstName().GetValue() != "Author" {
		t.Fatalf("ChannelsGetMessageAuthor() = %v, want author %d", got, authorID)
	}
	if resolver.calls != 1 || resolver.userID != selfID || resolver.channelID != channelID || resolver.messageID != int32(messageID) {
		t.Fatalf("provider call = %+v, want user=%d channel=%d id=%d", resolver, selfID, channelID, messageID)
	}
	if users.calls != 1 {
		t.Fatalf("UserGetMutableUsers calls = %d, want one", users.calls)
	}
}

func TestChannelsGetMessageAuthorReturnsEmptyForAnonymousOrMissingMessage(t *testing.T) {
	for name, authorID := range map[string]int64{"anonymous": 0, "missing": 0} {
		t.Run(name, func(t *testing.T) {
			resolver := &messageAuthorResolver{authorID: authorID}
			users := &messageAuthorUserClient{users: messageAuthorUsers(42, 77)}
			got, err := newMessageAuthorCore(resolver, users, 42).ChannelsGetMessageAuthor(messageAuthorRequest(9001, 12))
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
	unauthenticated := newMessageAuthorCore(&messageAuthorResolver{}, &messageAuthorUserClient{}, 0)
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
		t.Fatalf("missing user provider = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}

	providerErr := errors.New("channel store unavailable")
	core := newMessageAuthorCore(&messageAuthorResolver{err: providerErr}, &messageAuthorUserClient{}, 42)
	if got, err := core.ChannelsGetMessageAuthor(request); got != nil || !errors.Is(err, providerErr) {
		t.Fatalf("channel provider error = (%v, %v), want propagated error", got, err)
	}

	wantUserErr := errors.New("user unavailable")
	core = newMessageAuthorCore(&messageAuthorResolver{authorID: 77}, &messageAuthorUserClient{err: wantUserErr}, 42)
	if got, err := core.ChannelsGetMessageAuthor(request); got != nil || !errors.Is(err, wantUserErr) {
		t.Fatalf("user provider error = (%v, %v), want propagated error", got, err)
	}
}

func TestChannelsGetMessageAuthorRejectsNilRequest(t *testing.T) {
	core := newMessageAuthorCore(&messageAuthorResolver{}, &messageAuthorUserClient{}, 42)
	if got, err := core.ChannelsGetMessageAuthor(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}
