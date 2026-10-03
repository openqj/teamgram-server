package core

import (
	"context"
	"database/sql"
	"net"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const messagesViewsCallerID int64 = 41

type messagesViewsMessageClientStub struct {
	messageclient.MessageClient
	response *messagepb.Vector_MessageBox
	err      error
	calls    int
	request  *messagepb.TLMessageGetUserMessageList
}

func (s *messagesViewsMessageClientStub) MessageGetUserMessageList(_ context.Context, in *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

type messagesViewsUserClientStub struct {
	userclient.UserClient
	response *userpb.Vector_ImmutableUser
	err      error
	calls    int
}

func (s *messagesViewsUserClientStub) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.calls++
	return s.response, s.err
}

type messagesViewsChatRPCStub struct {
	chatpb.UnimplementedRPCChatServer
	chat  *mtproto.MutableChat
	err   error
	calls int
}

func (s *messagesViewsChatRPCStub) ChatGetMutableChat(_ context.Context, _ *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	s.calls++
	return s.chat, s.err
}

type messagesViewsZRPCClient struct {
	conn *grpc.ClientConn
}

func (c messagesViewsZRPCClient) Conn() *grpc.ClientConn {
	return c.conn
}

func newMessagesViewsChatClient(t *testing.T, rpcStub *messagesViewsChatRPCStub) *chatclient.ChatClientHelper {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	chatpb.RegisterRPCChatServer(server, rpcStub)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial in-memory chat RPC: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return chatclient.NewChatClientHelper(messagesViewsZRPCClient{conn: conn})
}

func newMessagesViewsTestCore(messages messageclient.MessageClient, users userclient.UserClient, chats *chatclient.ChatClientHelper) *MessagesCore {
	ctx := context.Background()
	return &MessagesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			MessageClient: messages,
			UserClient:    users,
			ChatClient:    chats,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: messagesViewsCallerID},
	}
}

func messagesViewsUser(id, accessHash int64) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
			Id:         id,
			AccessHash: accessHash,
		}}).To_ImmutableUser(),
	}}
}

func messagesViewsBox(id int32, peer *mtproto.Peer, views, forwards *wrapperspb.Int32Value, replies *mtproto.MessageReplies) *mtproto.MessageBox {
	peerUtil := mtproto.FromPeer(peer)
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:       id,
		FromId:   mtproto.MakePeerUser(messagesViewsCallerID),
		PeerId:   peer,
		Message:  "views fixture",
		Views:    views,
		Forwards: forwards,
		Replies:  replies,
	}).To_Message()
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: id,
		PeerType:  peerUtil.PeerType,
		PeerId:    peerUtil.PeerId,
		Message:   message,
	}).To_MessageBox()
}

func TestMessagesGetMessagesViewsReturnsStoredValuesInRequestOrder(t *testing.T) {
	const (
		peerID     int64 = 84
		accessHash int64 = 840
	)
	peer := mtproto.MakePeerUser(peerID)
	replies := mtproto.MakeTLMessageReplies(&mtproto.MessageReplies{Comments: true, Replies: 3}).To_MessageReplies()
	first := messagesViewsBox(1, peer, nil, wrapperspb.Int32(7), replies)
	second := messagesViewsBox(2, peer, wrapperspb.Int32(20), wrapperspb.Int32(4), nil)
	messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{first, second},
	}}
	users := &messagesViewsUserClientStub{response: messagesViewsUser(peerID, accessHash)}
	core := newMessagesViewsTestCore(messages, users, nil)

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
			UserId:     peerID,
			AccessHash: accessHash,
		}).To_InputPeer(),
		Id:        []int32{2, 1, 2},
		Increment: mtproto.BoolFalse,
	})
	if err != nil {
		t.Fatalf("MessagesGetMessagesViews() error = %v", err)
	}
	if messages.calls != 1 || len(messages.request.GetIdList()) != 3 {
		t.Fatalf("message lookup calls=%d ids=%v, want one lookup for requested IDs", messages.calls, messages.request.GetIdList())
	}
	if len(got.GetViews()) != 3 {
		t.Fatalf("views length = %d, want 3", len(got.GetViews()))
	}
	if got.GetViews()[0].GetViews().GetValue() != 20 || got.GetViews()[2].GetViews().GetValue() != 20 {
		t.Fatalf("repeated message views = (%v, %v), want 20", got.GetViews()[0].GetViews(), got.GetViews()[2].GetViews())
	}
	if got.GetViews()[1].GetViews() != nil {
		t.Fatalf("nil stored views became %v", got.GetViews()[1].GetViews())
	}
	if got.GetViews()[1].GetForwards().GetValue() != 7 || got.GetViews()[1].GetReplies().GetReplies() != 3 {
		t.Fatalf("stored forwards/replies = (%v, %v), want (7, 3)", got.GetViews()[1].GetForwards(), got.GetViews()[1].GetReplies())
	}
}

func TestMessagesGetMessagesViewsRejectsMissingOrMismatchedStoredMessages(t *testing.T) {
	const (
		peerID     int64 = 84
		accessHash int64 = 840
	)
	validPeer := mtproto.MakePeerUser(peerID)
	tests := []struct {
		name  string
		ids   []int32
		boxes []*mtproto.MessageBox
	}{
		{
			name:  "missing",
			ids:   []int32{2},
			boxes: []*mtproto.MessageBox{messagesViewsBox(1, validPeer, wrapperspb.Int32(1), nil, nil)},
		},
		{
			name:  "peer mismatch",
			ids:   []int32{2},
			boxes: []*mtproto.MessageBox{messagesViewsBox(2, mtproto.MakePeerChat(99), wrapperspb.Int32(1), nil, nil)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{Datas: test.boxes}}
			users := &messagesViewsUserClientStub{response: messagesViewsUser(peerID, accessHash)}
			core := newMessagesViewsTestCore(messages, users, nil)

			got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
				Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
					UserId:     peerID,
					AccessHash: accessHash,
				}).To_InputPeer(),
				Id:        test.ids,
				Increment: mtproto.BoolFalse,
			})
			if got != nil || err != mtproto.ErrMessageIdInvalid {
				t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, MESSAGE_ID_INVALID)", got, err)
			}
		})
	}
}

func TestMessagesGetMessagesViewsRejectsBadUserAccessHashBeforeLookup(t *testing.T) {
	const (
		peerID     int64 = 84
		accessHash int64 = 840
	)
	messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{messagesViewsBox(1, mtproto.MakePeerUser(peerID), wrapperspb.Int32(1), nil, nil)},
	}}
	users := &messagesViewsUserClientStub{response: messagesViewsUser(peerID, accessHash)}
	core := newMessagesViewsTestCore(messages, users, nil)

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
			UserId:     peerID,
			AccessHash: accessHash + 1,
		}).To_InputPeer(),
		Id: []int32{1},
	})
	if got != nil || err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, PEER_ID_INVALID)", got, err)
	}
	if messages.calls != 0 {
		t.Fatalf("message lookup calls = %d, want 0 after invalid access hash", messages.calls)
	}
}

func TestMessagesGetMessagesViewsRequiresBasicGroupMembership(t *testing.T) {
	messages := &messagesViewsMessageClientStub{}
	chatStub := &messagesViewsChatRPCStub{chat: &mtproto.MutableChat{}}
	core := newMessagesViewsTestCore(messages, nil, newMessagesViewsChatClient(t, chatStub))

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
		Id:   []int32{1},
	})
	if got != nil || err != mtproto.ErrUserNotParticipant {
		t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, USER_NOT_PARTICIPANT)", got, err)
	}
	if messages.calls != 0 {
		t.Fatalf("message lookup calls = %d, want 0 for non-member", messages.calls)
	}
}

func TestMessagesGetMessagesViewsDoesNotFabricateIncrementedCounts(t *testing.T) {
	stored := messagesViewsBox(1, mtproto.MakePeerUser(messagesViewsCallerID), wrapperspb.Int32(7), nil, nil)
	messages := &messagesViewsMessageClientStub{response: &messagepb.Vector_MessageBox{
		Datas: []*mtproto.MessageBox{stored},
	}}
	core := newMessagesViewsTestCore(messages, nil, nil)

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Id:        []int32{1},
		Increment: mtproto.BoolTrue,
	})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesGetMessagesViews() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
	if messages.calls != 1 || stored.GetMessage().GetViews().GetValue() != 7 {
		t.Fatalf("increment changed persisted fixture or skipped validation: calls=%d views=%v", messages.calls, stored.GetMessage().GetViews())
	}
}

func TestMessagesGetMessagesViewsReturnsPersistedChannelCounts(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	if err := channelview.Open(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	channelID := time.Now().UnixNano()
	if _, err = db.Exec(`INSERT INTO apifull_channel
		(id, access_hash, creator_user_id, title, broadcast, megagroup, created_at)
		VALUES (?,?,?,?,?,?,?)`, channelID, channelID, messagesViewsCallerID, "message-views-test", 1, 0, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})

	core := newMessagesViewsTestCore(nil, nil, nil)
	input := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  channelID,
		AccessHash: channelID,
	}).To_InputPeer()
	for _, messageID := range []int32{1, 2, 3} {
		if _, err = db.Exec(`INSERT INTO apifull_channel_message
			(channel_id, message_id, sender_user_id, date, message)
			VALUES (?,?,?,?,?)`, channelID, messageID, messagesViewsCallerID, time.Now().Unix(), "channel views fixture"); err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []struct {
		userID    int64
		readMaxID int32
	}{
		{userID: 41, readMaxID: 2},
		{userID: 42, readMaxID: 1},
		{userID: 43, readMaxID: 2},
	} {
		if _, err = db.Exec(`INSERT INTO apifull_channel_read_state (user_id, channel_id, read_max_id) VALUES (?,?,?)`,
			state.userID, channelID, state.readMaxID); err != nil {
			t.Fatal(err)
		}
	}

	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer: input,
		Id:   []int32{2, 3, 1, 2},
	})
	if err != nil {
		t.Fatalf("valid channel error = %v", err)
	}
	if len(got.GetViews()) != 4 || got.GetViews()[0].GetViews().GetValue() != 2 ||
		got.GetViews()[1].GetViews() == nil || got.GetViews()[1].GetViews().GetValue() != 0 ||
		got.GetViews()[2].GetViews().GetValue() != 3 || got.GetViews()[3].GetViews().GetValue() != 2 {
		t.Fatalf("channel views = %v, want [2 0 3 2]", got.GetViews())
	}
	history, err := channelview.HistoryForInputPeer(messagesViewsCallerID, input, 0, 20)
	if err != nil {
		t.Fatalf("channel history error = %v", err)
	}
	historyViews := make(map[int32]*wrapperspb.Int32Value, len(history.GetMessages()))
	for _, message := range history.GetMessages() {
		historyViews[message.GetId()] = message.GetViews()
	}
	if historyViews[1] == nil || historyViews[1].GetValue() != 3 ||
		historyViews[2] == nil || historyViews[2].GetValue() != 2 ||
		historyViews[3] == nil || historyViews[3].GetValue() != 0 {
		t.Fatalf("serialized history views = %v, want message counts [3 2 0]", historyViews)
	}

	if _, err = db.Exec(`UPDATE apifull_channel_read_state SET read_max_id=2 WHERE user_id=? AND channel_id=?`, 42, channelID); err != nil {
		t.Fatal(err)
	}
	got, err = core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{Peer: input, Id: []int32{2}})
	if err != nil || len(got.GetViews()) != 1 || got.GetViews()[0].GetViews().GetValue() != 3 {
		t.Fatalf("updated persisted count = (%v, %v), want one view count of 3", got, err)
	}

	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  channelID,
		AccessHash: channelID + 1,
	}).To_InputPeer()
	got, err = core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{Peer: badHash, Id: []int32{1}})
	if got != nil || err != mtproto.ErrChannelInvalid {
		t.Fatalf("bad channel access hash = (%v, %v), want (nil, CHANNEL_INVALID)", got, err)
	}

	var readMaxBeforeInvalid int32
	if err = db.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		messagesViewsCallerID, channelID).Scan(&readMaxBeforeInvalid); err != nil {
		t.Fatal(err)
	}
	got, err = core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      input,
		Id:        []int32{4},
		Increment: mtproto.BoolTrue,
	})
	if got != nil || err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("missing channel message = (%v, %v), want (nil, MESSAGE_ID_INVALID)", got, err)
	}
	var readMaxAfterInvalid int32
	if err = db.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		messagesViewsCallerID, channelID).Scan(&readMaxAfterInvalid); err != nil {
		t.Fatal(err)
	}
	if readMaxAfterInvalid != readMaxBeforeInvalid {
		t.Fatalf("invalid increment changed channel read state from %d to %d", readMaxBeforeInvalid, readMaxAfterInvalid)
	}
	got, err = core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      input,
		Id:        []int32{3},
		Increment: mtproto.BoolTrue,
	})
	if err != nil || len(got.GetViews()) != 1 || got.GetViews()[0].GetViews() == nil || got.GetViews()[0].GetViews().GetValue() != 1 {
		t.Fatalf("incremented channel request = (%v, %v), want one view count of 1", got, err)
	}
	var readMaxAfterIncrement int32
	if err = db.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		messagesViewsCallerID, channelID).Scan(&readMaxAfterIncrement); err != nil {
		t.Fatal(err)
	}
	if readMaxAfterIncrement != 3 {
		t.Fatalf("incremented channel read state = %d, want 3", readMaxAfterIncrement)
	}
	got, err = core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{
		Peer:      input,
		Id:        []int32{3},
		Increment: mtproto.BoolTrue,
	})
	if err != nil || len(got.GetViews()) != 1 || got.GetViews()[0].GetViews() == nil || got.GetViews()[0].GetViews().GetValue() != 1 {
		t.Fatalf("repeated channel increment = (%v, %v), want one view count of 1", got, err)
	}
	var readMaxAfterRepeat int32
	if err = db.QueryRow(`SELECT read_max_id FROM apifull_channel_read_state WHERE user_id=? AND channel_id=?`,
		messagesViewsCallerID, channelID).Scan(&readMaxAfterRepeat); err != nil {
		t.Fatal(err)
	}
	if readMaxAfterRepeat != readMaxAfterIncrement {
		t.Fatalf("repeated increment changed channel read state from %d to %d", readMaxAfterIncrement, readMaxAfterRepeat)
	}
}

func TestMessagesGetMessagesViewsRejectsEmptyIDs(t *testing.T) {
	core := newMessagesViewsTestCore(nil, nil, nil)
	got, err := core.MessagesGetMessagesViews(&mtproto.TLMessagesGetMessagesViews{})
	if got != nil || err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("MessagesGetMessagesViews(empty IDs) = (%+v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
	}
}
