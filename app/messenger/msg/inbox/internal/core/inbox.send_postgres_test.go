package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	idgenclient "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type inboxSyncStub struct {
	syncclient.SyncClient
	err       error
	count     int
	pts       int32
	nilResult bool
	updates   *mtproto.Updates
}

func (s *inboxSyncStub) push(updates *mtproto.Updates) (*mtproto.Void, error) {
	s.count++
	s.pts = updates.Updates[0].Pts_INT32
	s.updates = proto.Clone(updates).(*mtproto.Updates)
	if s.err != nil {
		return nil, s.err
	}
	if s.nilResult {
		return nil, nil
	}
	return mtproto.EmptyVoid, nil
}

func TestPostgresInboxChatMigrationRetryKeepsOriginalReadUpdate(t *testing.T) {
	d, _, push := newInboxSendFixture(t)
	ctx := context.Background()
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		PeerId: mtproto.MakePeerChat(303), FromId: mtproto.MakePeerUser(101), Date: 1,
		Action: mtproto.MakeMessageActionChatMigrateTo(404),
	}).To_Message()
	request := &inbox.TLInboxSendUserMessageToInboxV2{UserId: 202, FromId: 101, PeerType: mtproto.PEER_CHAT, PeerId: 303,
		BoxList: []*mtproto.MessageBox{{UserId: 101, SenderUserId: 101, MessageId: 1, DialogMessageId: 77, RandomId: 0, PeerType: mtproto.PEER_CHAT, PeerId: 303, Message: message}},
	}
	if err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error { return d.EnqueueInboxDeliveryOn(ctx, tx, request) }); err != nil {
		t.Fatal(err)
	}
	core := New(ctx, &svc.ServiceContext{Dao: d})
	push.err = errors.New("sync unavailable")
	if _, err := core.InboxSendUserMessageToInboxV2(proto.Clone(request).(*inbox.TLInboxSendUserMessageToInboxV2)); !errors.Is(err, push.err) {
		t.Fatalf("migration sync failure: %v", err)
	}
	push.err = nil
	if _, err := core.InboxSendUserMessageToInboxV2(proto.Clone(request).(*inbox.TLInboxSendUserMessageToInboxV2)); err != nil {
		t.Fatal(err)
	}
	if len(push.updates.Updates) != 2 || push.updates.Updates[0].Pts_INT32 != 1 || push.updates.Updates[1].Pts_INT32 != 2 || push.updates.Updates[1].GetPredicateName() != mtproto.Predicate_updateReadHistoryInbox {
		t.Fatalf("migration updates changed order/pts: %#v", push.updates)
	}
	var readMax, unread, updateCount int32
	if err := d.Pool.QueryRow(ctx, `SELECT read_inbox_max_id, unread_count, (SELECT count(*) FROM user_pts_updates WHERE user_id=202) FROM dialogs WHERE user_id=202 AND peer_id=303`).Scan(&readMax, &unread, &updateCount); err != nil || readMax != 1 || unread != 0 || updateCount != 2 {
		t.Fatalf("migration read/unread/updates=%d/%d/%d err=%v", readMax, unread, updateCount, err)
	}
	if pts, err := counter.NewCounterStore(d.Pool).Current(ctx, counter.PtsKey(202)); err != nil || pts != 2 {
		t.Fatalf("migration allocated retry pts=%d err=%v", pts, err)
	}
	if _, err := core.InboxSendUserMessageToInboxV2(proto.Clone(request).(*inbox.TLInboxSendUserMessageToInboxV2)); err != nil || push.count != 2 {
		t.Fatalf("completed migration replay pushed again count=%d err=%v", push.count, err)
	}
}

func (s *inboxSyncStub) SyncPushUpdates(_ context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	return s.push(in.Updates)
}

func (s *inboxSyncStub) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	return s.push(in.Updates)
}

type inboxIDStub struct {
	idgen.UnimplementedRPCIdgenServer
	seq map[string]int64
}

func (s *inboxIDStub) IdgenGetNextSeqId(_ context.Context, in *idgen.TLIdgenGetNextSeqId) (*mtproto.Int64, error) {
	s.seq[in.Key]++
	return &mtproto.Int64{V: s.seq[in.Key]}, nil
}

func (s *inboxIDStub) IdgenGetNextIdValList(_ context.Context, in *idgen.TLIdgenGetNextIdValList) (*idgen.Vector_IdVal, error) {
	result := &idgen.Vector_IdVal{}
	for _, id := range in.Id {
		s.seq[id.Key]++
		result.Datas = append(result.Datas, idgen.MakeTLSeqIdVal(&idgen.IdVal{Id_INT64: s.seq[id.Key]}).To_IdVal())
	}
	return result, nil
}

type inboxTestRPCClient struct{ conn *grpc.ClientConn }

func (c inboxTestRPCClient) Conn() *grpc.ClientConn { return c.conn }

func newInboxSendFixture(t *testing.T) (*dao.Dao, *inboxIDStub, *inboxSyncStub) {
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
	schema := pgx.Identifier{fmt.Sprintf("inbox_core_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	for _, table := range []string{"messages", "dialogs", "hash_tags", "user_pts_updates", "msg_inbox_delivery_outbox", "idgen_counters", "users", "user_peer_blocks"} {
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
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,phone) VALUES (101,'fixture-101'),(202,'fixture-202')`); err != nil {
		t.Fatal(err)
	}
	ids := &inboxIDStub{seq: make(map[string]int64)}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	idgen.RegisterRPCIdgenServer(server, ids)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	push := &inboxSyncStub{}
	return &dao.Dao{
		Postgres:     &dao.Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)},
		IDGenClient2: idgenclient.NewIDGenClient2(inboxTestRPCClient{conn}),
		SyncClient:   push,
	}, ids, push
}

func TestPostgresInboxConsumerRetryAfterSyncFailure(t *testing.T) {
	for _, sender := range []bool{false, true} {
		for _, randomID := range []int64{0, 303} {
			t.Run(fmt.Sprintf("sender_%t_random_%d", sender, randomID), func(t *testing.T) {
				d, _, push := newInboxSendFixture(t)
				ctx := context.Background()
				request := &inbox.TLInboxSendUserMessageToInboxV2{
					UserId: 202, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202,
				}
				if sender {
					request.UserId = 101
					request.Out = true
				}
				boxes, err := d.SendMessagesWithDeliveries(ctx, 101, mtproto.MakePeerUtil(mtproto.PEER_USER, 202), []*msg.OutboxMessage{{
					RandomId: randomID, Message: mtproto.MakeTLMessage(&mtproto.Message{Message: "durable consumer", Date: 1, PeerId: mtproto.MakePeerUser(202)}).To_Message(),
				}}, []*inbox.TLInboxSendUserMessageToInboxV2{
					{UserId: 101, Out: true, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
					{UserId: 202, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
				})
				if err != nil {
					t.Fatal(err)
				}
				request.BoxList = boxes
				push.err = errors.New("sync queue unavailable")
				core := New(ctx, &svc.ServiceContext{Dao: d})
				invoke := func() error {
					_, err := core.InboxSendUserMessageToInboxV2(proto.Clone(request).(*inbox.TLInboxSendUserMessageToInboxV2))
					return err
				}
				if err := invoke(); !errors.Is(err, push.err) {
					t.Fatalf("sync failure result = %v", err)
				}
				pending, err := d.HasPendingInboxDelivery(ctx, 101, boxes[0].DialogMessageId, request.UserId)
				if err != nil || !pending {
					t.Fatalf("failed sync delivery pending = %t, %v", pending, err)
				}
				push.err = nil
				if err := invoke(); err != nil {
					t.Fatal(err)
				}
				pending, err = d.HasPendingInboxDelivery(ctx, 101, boxes[0].DialogMessageId, request.UserId)
				if err != nil || pending || push.count != 2 || push.pts != 1 {
					t.Fatalf("retry pending=%t count=%d pts=%d err=%v", pending, push.count, push.pts, err)
				}
				if err := invoke(); err != nil || push.count != 2 {
					t.Fatalf("completed replay sent another update: count=%d err=%v", push.count, err)
				}
				var count int
				if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM user_pts_updates WHERE user_id=$1`, request.UserId).Scan(&count); err != nil || count != 1 {
					t.Fatalf("recipient updates=%d err=%v", count, err)
				}
				if current, err := counter.NewCounterStore(d.Pool).Current(ctx, counter.PtsKey(request.UserId)); err != nil || current != 1 {
					t.Fatal("retry allocated another pts")
				}
			})
		}
	}
}

func TestPostgresInboxConsumerMissingSyncResultKeepsDelivery(t *testing.T) {
	for _, sender := range []bool{false, true} {
		for _, missingClient := range []bool{false, true} {
			t.Run(fmt.Sprintf("sender_%t_missing_client_%t", sender, missingClient), func(t *testing.T) {
				d, _, push := newInboxSendFixture(t)
				ctx := context.Background()
				request := &inbox.TLInboxSendUserMessageToInboxV2{UserId: 202, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202}
				if sender {
					request.UserId, request.Out = 101, true
				}
				boxes, err := d.SendMessagesWithDeliveries(ctx, 101, mtproto.MakePeerUtil(mtproto.PEER_USER, 202), []*msg.OutboxMessage{{
					RandomId: 0, Message: mtproto.MakeTLMessage(&mtproto.Message{Message: "missing ack", Date: 1, PeerId: mtproto.MakePeerUser(202)}).To_Message(),
				}}, []*inbox.TLInboxSendUserMessageToInboxV2{
					{UserId: 101, Out: true, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
					{UserId: 202, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
				})
				if err != nil {
					t.Fatal(err)
				}
				request.BoxList = boxes
				if missingClient {
					d.SyncClient = nil
				} else {
					push.nilResult = true
				}
				core := New(ctx, &svc.ServiceContext{Dao: d})
				if _, err := core.InboxSendUserMessageToInboxV2(proto.Clone(request).(*inbox.TLInboxSendUserMessageToInboxV2)); err == nil {
					t.Fatal("missing Sync acknowledgement succeeded")
				}
				pending, err := d.HasPendingInboxDelivery(ctx, 101, boxes[0].DialogMessageId, request.UserId)
				if err != nil || !pending {
					t.Fatalf("missing ack pending=%t err=%v", pending, err)
				}
				push.nilResult, d.SyncClient = false, push
				if _, err := core.InboxSendUserMessageToInboxV2(proto.Clone(request).(*inbox.TLInboxSendUserMessageToInboxV2)); err != nil {
					t.Fatal(err)
				}
				pending, err = d.HasPendingInboxDelivery(ctx, 101, boxes[0].DialogMessageId, request.UserId)
				if err != nil || pending {
					t.Fatalf("successful retry pending=%t err=%v", pending, err)
				}
			})
		}
	}
}
