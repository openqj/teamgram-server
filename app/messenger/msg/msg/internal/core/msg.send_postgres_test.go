package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	idgenclient "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type sendUsersStub struct {
	userclient.UserClient
	users []*mtproto.ImmutableUser
	err   error
}

func (s *sendUsersStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	if s.err != nil {
		return nil, s.err
	}
	return proto.Clone(&userpb.Vector_ImmutableUser{Datas: s.users}).(*userpb.Vector_ImmutableUser), nil
}

func (s *sendUsersStub) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	if s.err != nil {
		return nil, s.err
	}
	return proto.Clone(&mtproto.MutableUsers{Users: s.users}).(*mtproto.MutableUsers), nil
}

type sendChatStub struct {
	chatclient.ChatClient
	chat *mtproto.MutableChat
}

func (s *sendChatStub) ChatGetMutableChat(context.Context, *chatpb.TLChatGetMutableChat) (*mtproto.MutableChat, error) {
	return proto.Clone(s.chat).(*mtproto.MutableChat), nil
}

type sendIDsStub struct {
	idgen.UnimplementedRPCIdgenServer
	mu   sync.Mutex
	next int64
}

func (s *sendIDsStub) IdgenGetNextIdValList(_ context.Context, in *idgen.TLIdgenGetNextIdValList) (*idgen.Vector_IdVal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := &idgen.Vector_IdVal{}
	for range in.GetId() {
		s.next++
		result.Datas = append(result.Datas, idgen.MakeTLIdVal(&idgen.IdVal{Id_INT64: s.next}).To_IdVal())
	}
	return result, nil
}

type sendRPCClient struct{ conn *grpc.ClientConn }

func (c sendRPCClient) Conn() *grpc.ClientConn { return c.conn }

func newPostgresSendCore(t *testing.T) (*MsgCore, *sendUsersStub) {
	t.Helper()
	dsn := os.Getenv("MESSENGER_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MESSENGER_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var version int
	if err := admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil || version/10000 != 18 {
		t.Fatalf("PostgreSQL 18 required: version=%d err=%v", version, err)
	}
	schema := pgx.Identifier{fmt.Sprintf("msg_core_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("cleanup schema: %v", err)
		}
	})
	for _, table := range []string{"messages", "dialogs", "hash_tags", "user_pts_updates", "msg_inbox_delivery_outbox", "msg_state_delivery_outbox", "msg_inbox_consumer_receipts", "idgen_counters", "users", "user_peer_blocks", "chats", "chat_participants", "message_read_outbox", "saved_dialogs"} {
		name := pgx.Identifier{table}.Sanitize()
		if _, err := admin.Exec(ctx, `CREATE TABLE `+schema+`.`+name+` (LIKE public.`+name+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,phone) VALUES (101,'fixture-101'),(202,'fixture-202');
 INSERT INTO chats(id,creator_user_id,access_hash,random_id) VALUES (303,101,1,1);
 INSERT INTO chat_participants(chat_id,user_id,state) VALUES (303,101,0),(303,202,0);`); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	idgen.RegisterRPCIdgenServer(server, &sendIDsStub{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users := &sendUsersStub{users: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 101, AccessHash: 1010, FirstName: "sender"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: 202, AccessHash: 2020, FirstName: "recipient"}}).To_ImmutableUser(),
	}}
	chat := &sendChatStub{chat: &mtproto.MutableChat{Chat: &mtproto.ImmutableChat{Id: 303, Creator: 101, Title: "PG group", ParticipantsCount: 2}, ChatParticipants: []*mtproto.ImmutableChatParticipant{
		{ChatId: 303, UserId: 101, State: mtproto.ChatMemberStateNormal, ParticipantType: mtproto.ChatMemberCreator},
		{ChatId: 303, UserId: 202, State: mtproto.ChatMemberStateNormal},
	}}}
	d := &dao.Dao{Postgres: &dao.Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)}, IDGenClient2: idgenclient.NewIDGenClient2(sendRPCClient{conn}), UserClient: users, ChatClient: chat}
	return New(ctx, &svc.ServiceContext{Dao: d}), users
}

func sendCoreRequest(peerType int32, peerID int64, count int) *msg.TLMsgSendMessageV2 {
	request := &msg.TLMsgSendMessageV2{UserId: 101, AuthKeyId: 404, PeerType: peerType, PeerId: peerID}
	for i := 0; i < count; i++ {
		request.Message = append(request.Message, &msg.OutboxMessage{RandomId: int64(500 + i), Message: mtproto.MakeTLMessage(&mtproto.Message{
			PeerId: mtproto.MakePeer(peerType, peerID), Message: "PG actual send", Date: 1,
		}).To_Message()})
	}
	return request
}

func TestPostgresMsgSendEntryBatchAndRetry(t *testing.T) {
	for _, peerType := range []int32{mtproto.PEER_USER, mtproto.PEER_CHAT} {
		for _, count := range []int{1, 2} {
			t.Run(fmt.Sprintf("peer_%d_batch_%d", peerType, count), func(t *testing.T) {
				core, _ := newPostgresSendCore(t)
				peerID := int64(202)
				if peerType == mtproto.PEER_CHAT {
					peerID = 303
				}
				request := sendCoreRequest(peerType, peerID, count)
				for retry := 0; retry < 2; retry++ {
					result, err := core.MsgSendMessageV2(proto.Clone(request).(*msg.TLMsgSendMessageV2))
					if err != nil || result == nil {
						t.Fatalf("send result=%#v err=%v", result, err)
					}
					buffer := mtproto.NewEncodeBuf(512)
					if err := result.Encode(buffer, 229); err != nil {
						t.Fatalf("Layer 229 encode: %v", err)
					}
					mapped := 0
					for _, update := range result.Updates {
						if update.GetPredicateName() == mtproto.Predicate_updateMessageID {
							if update.RandomId != request.Message[mapped].RandomId || update.Id_INT32 != int32(mapped+1) {
								t.Fatalf("random-id vector order changed: %#v", update)
							}
							mapped++
						}
					}
					if mapped != count {
						t.Fatalf("mapped messages=%d want=%d", mapped, count)
					}
				}
				var messages, updates, deliveries int
				if err := core.svcCtx.Pool.QueryRow(core.ctx, `SELECT (SELECT count(*) FROM messages), (SELECT count(*) FROM user_pts_updates), (SELECT count(*) FROM msg_inbox_delivery_outbox)`).Scan(&messages, &updates, &deliveries); err != nil {
					t.Fatal(err)
				}
				if messages != count || updates != count || deliveries != count*2 {
					t.Fatalf("send rows=%d/%d/%d want=%d/%d/%d", messages, updates, deliveries, count, count, count*2)
				}
				if pts, err := counter.NewCounterStore(core.svcCtx.Pool).Current(core.ctx, counter.PtsKey(101)); err != nil || pts != int64(count) {
					t.Fatalf("send pts=%d err=%v", pts, err)
				}
			})
		}
	}
}

func TestPostgresMsgSendEntryRejectsStalePermissions(t *testing.T) {
	for _, scenario := range []string{"blocked", "recipient_deleted", "sender_kicked", "chat_deactivated", "user_rpc_failure"} {
		t.Run(scenario, func(t *testing.T) {
			core, users := newPostgresSendCore(t)
			query := ""
			peerType, peerID := int32(mtproto.PEER_USER), int64(202)
			switch scenario {
			case "blocked":
				query = `INSERT INTO user_peer_blocks(user_id,peer_type,peer_id) VALUES (202,2,101)`
			case "recipient_deleted":
				query = `UPDATE users SET deleted=TRUE WHERE id=202`
			case "sender_kicked":
				peerType, peerID = mtproto.PEER_CHAT, 303
				query = `UPDATE chat_participants SET state=2 WHERE user_id=101`
			case "chat_deactivated":
				peerType, peerID = mtproto.PEER_CHAT, 303
				query = `UPDATE chats SET deactivated=TRUE WHERE id=303`
			case "user_rpc_failure":
				users.err = errors.New("user database unavailable")
			}
			if query != "" {
				if _, err := core.svcCtx.Pool.Exec(core.ctx, query); err != nil {
					t.Fatal(err)
				}
			}
			result, err := core.MsgSendMessageV2(sendCoreRequest(peerType, peerID, 2))
			if scenario == "blocked" {
				if err != nil || result == nil {
					t.Fatalf("blocked sender response=%#v err=%v", result, err)
				}
				var deliveries int
				if err := core.svcCtx.Pool.QueryRow(core.ctx, `SELECT count(*) FROM msg_inbox_delivery_outbox WHERE recipient_user_id=202`).Scan(&deliveries); err != nil || deliveries != 0 {
					t.Fatalf("blocked recipient received intents=%d err=%v", deliveries, err)
				}
				return
			}
			if err == nil || result != nil {
				t.Fatalf("stale permission succeeded: result=%#v err=%v", result, err)
			}
			for _, table := range []string{"messages", "dialogs", "user_pts_updates", "msg_inbox_delivery_outbox", "idgen_counters"} {
				var count int
				if err := core.svcCtx.Pool.QueryRow(core.ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
					t.Fatalf("rejected send retained %s=%d err=%v", table, count, err)
				}
			}
		})
	}
}
