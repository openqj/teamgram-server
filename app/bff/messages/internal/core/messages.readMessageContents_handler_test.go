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
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
)

type readMessageContentsMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
	calls    int
}

func (s *readMessageContentsMessageClientStub) MessageGetUserMessageList(context.Context, *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	s.calls++
	return s.response, s.err
}

type readMessageContentsMsgClientStub struct {
	msgclient.MsgClient
	response *mtproto.Messages_AffectedMessages
	err      error
	calls    int
}

func (s *readMessageContentsMsgClientStub) MsgReadMessageContents(context.Context, *msgpb.TLMsgReadMessageContents) (*mtproto.Messages_AffectedMessages, error) {
	s.calls++
	return s.response, s.err
}

func newReadMessageContentsTestCore(messages messageclient.MessageClient, msg msgclient.MsgClient) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			MsgClient:     msg,
			SyncClient:    &readMessageContentsSyncClientStub{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

type readMessageContentsSyncClientStub struct{ syncclient.SyncClient }

func TestMessagesReadMessageContentsRejectsUnauthenticatedAndMalformedInput(t *testing.T) {
	request := &mtproto.TLMessagesReadMessageContents{Id: []int32{1}}
	for _, tc := range []struct {
		name string
		core *MessagesCore
		in   *mtproto.TLMessagesReadMessageContents
		want error
	}{
		{name: "nil core", core: nil, in: request, want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &MessagesCore{}, in: request, want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}, want: mtproto.ErrInputRequestInvalid},
		{name: "nonpositive id", core: &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}, in: &mtproto.TLMessagesReadMessageContents{Id: []int32{0}}, want: mtproto.ErrMessageIdInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.core.MessagesReadMessageContents(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("MessagesReadMessageContents() = (%v, %v), want nil/%v", got, err, tc.want)
			}
		})
	}
}

func TestMessagesReadMessageContentsFailsClosedOnMissingDependenciesAndResponses(t *testing.T) {
	request := &mtproto.TLMessagesReadMessageContents{Id: []int32{7}}
	missingDeps := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	if got, err := missingDeps.MessagesReadMessageContents(request); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing dependencies = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
	}

	lookupErr := errors.New("message lookup unavailable")
	lookup := &readMessageContentsMessageClientStub{err: lookupErr}
	core := newReadMessageContentsTestCore(lookup, &readMessageContentsMsgClientStub{})
	if got, err := core.MessagesReadMessageContents(request); got != nil || !errors.Is(err, lookupErr) {
		t.Fatalf("lookup failure = (%v, %v), want propagated lookup error", got, err)
	}

	nilList := &readMessageContentsMessageClientStub{}
	core = newReadMessageContentsTestCore(nilList, &readMessageContentsMsgClientStub{})
	if got, err := core.MessagesReadMessageContents(request); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil message list = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
	}

	box := &mtproto.MessageBox{
		MessageId:    7,
		SenderUserId: 84,
		PeerType:     mtproto.PEER_USER,
		PeerId:       84,
		Message:      &mtproto.Message{},
	}
	messageList := &readMessageContentsMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: []*mtproto.MessageBox{box}}}
	nilAffected := &readMessageContentsMsgClientStub{}
	core = newReadMessageContentsTestCore(messageList, nilAffected)
	if got, err := core.MessagesReadMessageContents(request); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil affected response = (%v, %v), want nil/INTERNAL_SERVER_ERROR", got, err)
	}
}

func TestGroupReadMessageContentsByResolvedPeer(t *testing.T) {
	const userID int64 = 100
	messages := []*mtproto.MessageBox{
		{
			MessageId:    1,
			SenderUserId: 200,
			PeerType:     mtproto.PEER_USER,
			PeerId:       userID,
			Message:      &mtproto.Message{},
		},
		{
			MessageId:    2,
			SenderUserId: userID,
			PeerType:     mtproto.PEER_USER,
			PeerId:       200,
			Message:      &mtproto.Message{},
		},
		{
			MessageId:    3,
			SenderUserId: 300,
			PeerType:     mtproto.PEER_CHAT,
			Message: &mtproto.Message{
				PeerId:    mtproto.MakePeerChat(3000),
				Mentioned: true,
			},
		},
		{
			MessageId:    4,
			SenderUserId: 301,
			PeerType:     mtproto.PEER_CHAT,
			Message: &mtproto.Message{
				PeerId:      mtproto.MakePeerChat(3000),
				MediaUnread: true,
			},
		},
		{
			MessageId:    5,
			SenderUserId: 400,
			PeerType:     mtproto.PEER_USER,
			PeerId:       userID,
			Message:      &mtproto.Message{},
		},
	}

	groups, err := groupReadMessageContents(userID, messages)
	if err != nil {
		t.Fatalf("groupReadMessageContents() error = %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("groupReadMessageContents() returned %d groups, want 3", len(groups))
	}
	if got := groups[0].peer; got != (readMessageContentsPeer{peerType: mtproto.PEER_USER, peerID: 200}) {
		t.Fatalf("first peer = %+v, want user 200", got)
	}
	if len(groups[0].contents) != 0 {
		t.Fatalf("user group has %d content updates, want 0", len(groups[0].contents))
	}
	if got := groups[1].peer; got != (readMessageContentsPeer{peerType: mtproto.PEER_CHAT, peerID: 3000}) {
		t.Fatalf("second peer = %+v, want chat 3000", got)
	}
	if len(groups[1].contents) != 2 || !groups[1].contents[0].Mentioned || !groups[1].contents[1].MediaUnread {
		t.Fatalf("chat content updates = %+v, want mentioned and media-unread updates", groups[1].contents)
	}
	if got := groups[2].peer; got != (readMessageContentsPeer{peerType: mtproto.PEER_USER, peerID: 400}) {
		t.Fatalf("third peer = %+v, want user 400", got)
	}
}

func TestGroupReadMessageContentsRejectsUnsupportedPeerBeforeMutation(t *testing.T) {
	messages := []*mtproto.MessageBox{
		{
			MessageId: 1,
			PeerType:  mtproto.PEER_USER,
			PeerId:    200,
			Message:   &mtproto.Message{},
		},
		{
			MessageId: 2,
			PeerType:  mtproto.PEER_CHANNEL,
			PeerId:    300,
			Message:   &mtproto.Message{},
		},
	}

	groups, err := groupReadMessageContents(100, messages)
	if err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("groupReadMessageContents() error = %v, want PEER_ID_INVALID", err)
	}
	if groups != nil {
		t.Fatalf("groupReadMessageContents() returned partial groups before validation: %+v", groups)
	}
}
