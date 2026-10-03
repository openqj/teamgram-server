package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type readMentionsMsgClientStub struct {
	msgclient.MsgClient
	response *mtproto.Messages_AffectedHistory
	err      error
	calls    int
	request  *msgpb.TLMsgReadMentions
}

func (s *readMentionsMsgClientStub) MsgReadMentions(_ context.Context, in *msgpb.TLMsgReadMentions) (*mtproto.Messages_AffectedHistory, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

func newReadMentionsTestCore(t *testing.T, stub msgclient.MsgClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MsgClient: stub,
			ChatClient: newMessagesViewsChatClient(t, &messagesViewsChatRPCStub{chat: &mtproto.MutableChat{
				Chat: &mtproto.ImmutableChat{Id: 77},
				ChatParticipants: []*mtproto.ImmutableChatParticipant{{
					UserId:          42,
					ParticipantType: mtproto.ChatMemberNormal,
					State:           mtproto.ChatMemberStateNormal,
				}},
			}}),
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
	}
}

func TestMessagesReadMentionsDispatchesMentionOnlyRPC(t *testing.T) {
	stub := &readMentionsMsgClientStub{response: mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      7,
		PtsCount: 2,
	}).To_Messages_AffectedHistory()}
	got, err := newReadMentionsTestCore(t, stub).MessagesReadMentions(&mtproto.TLMessagesReadMentions{
		Peer:     mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
		TopMsgId: wrapperspb.Int32(123),
	})
	if err != nil || got != stub.response {
		t.Fatalf("MessagesReadMentions() = (%v, %v), want stub response", got, err)
	}
	if stub.calls != 1 || stub.request == nil {
		t.Fatalf("MsgReadMentions calls = %d request = %v, want one request", stub.calls, stub.request)
	}
	if stub.request.GetUserId() != 42 || stub.request.GetAuthKeyId() != 99 || stub.request.GetPeerType() != mtproto.PEER_CHAT || stub.request.GetPeerId() != 77 || stub.request.GetTopMsgId().GetValue() != 123 {
		t.Fatalf("MsgReadMentions request = %v, want caller/chat/top_msg_id fields", stub.request)
	}
}

func TestMessagesReadMentionsRejectsUnsupportedOrInvalidInputBeforeRPC(t *testing.T) {
	cases := []struct {
		name string
		in   *mtproto.TLMessagesReadMentions
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputConstructorInvalid},
		{name: "nil peer", in: &mtproto.TLMessagesReadMentions{}, want: mtproto.ErrPeerIdInvalid},
		{name: "zero top message", in: &mtproto.TLMessagesReadMentions{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(), TopMsgId: wrapperspb.Int32(0)}, want: mtproto.ErrMessageIdInvalid},
		{name: "channel storage", in: &mtproto.TLMessagesReadMentions{Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 77, AccessHash: 770}).To_InputPeer()}, want: mtproto.ErrMethodNotImpl},
		{name: "user storage", in: &mtproto.TLMessagesReadMentions{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 77, AccessHash: 770}).To_InputPeer()}, want: mtproto.ErrMethodNotImpl},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &readMentionsMsgClientStub{}
			got, err := newReadMentionsTestCore(t, stub).MessagesReadMentions(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("MessagesReadMentions() = (%v, %v), want nil/%v", got, err, tc.want)
			}
			if stub.calls != 0 {
				t.Fatalf("MsgReadMentions calls = %d, want 0", stub.calls)
			}
		})
	}
}

func TestMessagesReadMentionsFailsClosedOnBackendResult(t *testing.T) {
	stub := &readMentionsMsgClientStub{err: errors.New("mention store unavailable")}
	got, err := newReadMentionsTestCore(t, stub).MessagesReadMentions(&mtproto.TLMessagesReadMentions{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
	})
	if got != nil || err == nil || err.Error() != "mention store unavailable" {
		t.Fatalf("MessagesReadMentions() = (%v, %v), want backend error", got, err)
	}
}
