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
)

type readHistoryMsgClientStub struct {
	msgclient.MsgClient
	response *mtproto.Messages_AffectedMessages
	err      error
	calls    int
	request  *msgpb.TLMsgReadHistoryV2
}

func (s *readHistoryMsgClientStub) MsgReadHistoryV2(_ context.Context, in *msgpb.TLMsgReadHistoryV2) (*mtproto.Messages_AffectedMessages, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

func newReadHistoryTestCore(stub msgclient.MsgClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MsgClient: stub}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
	}
}

func TestMessagesReadHistoryMapsPeerAndPropagatesResult(t *testing.T) {
	stub := &readHistoryMsgClientStub{response: mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      7,
		PtsCount: 1,
	}).To_Messages_AffectedMessages()}
	c := newReadHistoryTestCore(stub)

	got, err := c.MessagesReadHistory(&mtproto.TLMessagesReadHistory{
		Peer:  mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
		MaxId: 123,
	})
	if err != nil || got != stub.response {
		t.Fatalf("MessagesReadHistory() = (%v, %v), want stub response", got, err)
	}
	if stub.calls != 1 || stub.request == nil {
		t.Fatalf("MsgReadHistoryV2 calls = %d request = %v, want one request", stub.calls, stub.request)
	}
	if stub.request.GetUserId() != 42 || stub.request.GetAuthKeyId() != 99 || stub.request.GetPeerType() != mtproto.PEER_CHAT || stub.request.GetPeerId() != 77 || stub.request.GetMaxId() != 123 {
		t.Fatalf("MsgReadHistoryV2 request = %v, want caller/chat/max_id fields", stub.request)
	}
}

func TestMessagesReadHistoryRejectsInvalidInputBeforeRPC(t *testing.T) {
	cases := []struct {
		name string
		in   *mtproto.TLMessagesReadHistory
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputConstructorInvalid},
		{name: "nil peer", in: &mtproto.TLMessagesReadHistory{}, want: mtproto.ErrPeerIdInvalid},
		{name: "empty peer", in: &mtproto.TLMessagesReadHistory{Peer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer()}, want: mtproto.ErrPeerIdInvalid},
		{name: "zero user", in: &mtproto.TLMessagesReadHistory{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 0}).To_InputPeer()}, want: mtproto.ErrPeerIdInvalid},
		{name: "channel peer", in: &mtproto.TLMessagesReadHistory{Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 77, AccessHash: 770}).To_InputPeer()}, want: mtproto.ErrPeerIdInvalid},
		{name: "negative max id", in: &mtproto.TLMessagesReadHistory{Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(), MaxId: -1}, want: mtproto.ErrMessageIdInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &readHistoryMsgClientStub{}
			got, err := newReadHistoryTestCore(stub).MessagesReadHistory(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("MessagesReadHistory() = (%v, %v), want nil/%v", got, err, tc.want)
			}
			if stub.calls != 0 {
				t.Fatalf("MsgReadHistoryV2 calls = %d, want 0", stub.calls)
			}
		})
	}

	unauthenticated := &MessagesCore{}
	if got, err := unauthenticated.MessagesReadHistory(&mtproto.TLMessagesReadHistory{}); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated call = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}

	missingClient := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	if got, err := missingClient.MessagesReadHistory(&mtproto.TLMessagesReadHistory{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing client call = (%v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestMessagesReadHistoryFailsClosedOnBackendResult(t *testing.T) {
	backendErr := errors.New("read history unavailable")
	for _, tc := range []struct {
		name     string
		response *mtproto.Messages_AffectedMessages
		err      error
		wantErr  error
	}{
		{name: "backend error", response: nil, err: backendErr, wantErr: backendErr},
		{name: "nil response", response: nil, err: nil, wantErr: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &readHistoryMsgClientStub{response: tc.response, err: tc.err}
			c := newReadHistoryTestCore(stub)
			got, err := c.MessagesReadHistory(&mtproto.TLMessagesReadHistory{
				Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
			})
			if got != nil || !errors.Is(err, tc.wantErr) {
				t.Fatalf("MessagesReadHistory() = (%v, %v), want nil/%v", got, err, tc.wantErr)
			}
			if stub.calls != 1 || stub.request.GetPeerType() != mtproto.PEER_SELF || stub.request.GetPeerId() != 42 {
				t.Fatalf("MsgReadHistoryV2 request = %v, calls = %d, want self mapping", stub.request, stub.calls)
			}
		})
	}
}
