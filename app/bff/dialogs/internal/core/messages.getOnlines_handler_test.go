package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type getOnlinesChatClientStub struct {
	chatclient.ChatClient
	chat *mtproto.MutableChat
	err  error
}

func (s *getOnlinesChatClientStub) ChatGetMutableChat(context.Context, *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	return s.chat, s.err
}

type getOnlinesUserClientStub struct {
	userclient.UserClient
	seens     *userpb.Vector_LastSeenData
	err       error
	requested []int64
}

func (s *getOnlinesUserClientStub) UserGetLastSeens(_ context.Context, in *userpb.TLUserGetLastSeens) (*userpb.Vector_LastSeenData, error) {
	s.requested = append([]int64(nil), in.GetId()...)
	return s.seens, s.err
}

func newGetOnlinesTestCore(chat *getOnlinesChatClientStub, users *getOnlinesUserClientStub) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{ChatClient: chat, UserClient: users}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func getOnlinesChat(chatID int64, memberIDs ...int64) *mtproto.MutableChat {
	chat := &mtproto.MutableChat{
		Chat: &mtproto.ImmutableChat{PredicateName: mtproto.Predicate_chat, Id: chatID},
	}
	for _, id := range memberIDs {
		chat.ChatParticipants = append(chat.ChatParticipants, &mtproto.ImmutableChatParticipant{
			UserId: id, ParticipantType: mtproto.ChatMemberNormal, State: mtproto.ChatMemberStateNormal,
		})
	}
	return chat
}

func TestMessagesGetOnlinesCountsRecentMembers(t *testing.T) {
	now := time.Now().Unix()
	users := &getOnlinesUserClientStub{seens: &userpb.Vector_LastSeenData{Datas: []*userpb.LastSeenData{
		{UserId: 42, LastSeenAt: now},
		{UserId: 43, LastSeenAt: now - 61},
	}}}
	core := newGetOnlinesTestCore(&getOnlinesChatClientStub{chat: getOnlinesChat(77, 42, 43)}, users)
	result, err := core.MessagesGetOnlines(&mtproto.TLMessagesGetOnlines{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer()})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.GetOnlines() != 1 {
		t.Fatalf("online count = %v, want 1", result)
	}
	if len(users.requested) != 2 || users.requested[0] != 42 || users.requested[1] != 43 {
		t.Fatalf("last-seen request = %v, want [42 43]", users.requested)
	}
}

func TestMessagesGetOnlinesRejectsInvalidPeer(t *testing.T) {
	core := newGetOnlinesTestCore(&getOnlinesChatClientStub{}, &getOnlinesUserClientStub{})
	result, err := core.MessagesGetOnlines(&mtproto.TLMessagesGetOnlines{})
	if result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("invalid peer = (%v, %v), want PEER_ID_INVALID", result, err)
	}
}

func TestMessagesGetOnlinesFailsWhenPresenceUnavailable(t *testing.T) {
	core := newGetOnlinesTestCore(&getOnlinesChatClientStub{err: errors.New("chat service unavailable")}, &getOnlinesUserClientStub{})
	result, err := core.MessagesGetOnlines(&mtproto.TLMessagesGetOnlines{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer()})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("unavailable presence = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}
