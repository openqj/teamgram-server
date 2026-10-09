package dao

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
	"github.com/teamgram/teamgram-server/pkg/mqconsumer"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type messageIDGenStub struct {
	idgen.UnimplementedRPCIdgenServer
	mu     sync.Mutex
	values map[string]int64
}

func TestPostgresMessageStateReceiptReplayDoesNotEnqueueAgain(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := mqconsumer.WithMetadata(context.Background(), mqconsumer.Metadata{
		ConsumerGroup: "msg-test", Topic: "msg-state", Partition: 0, Offset: 42,
	})
	var calls int
	mutate := func() error {
		_, _, err := d.MutateMessageStateOnce(ctx, 202, "delete", func(pgx.Tx) ([]*mtproto.Update, error) {
			calls++
			return []*mtproto.Update{mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{Messages: []int32{1}, PtsCount: 1}).To_Update()}, nil
		})
		return err
	}
	if err := mutate(); err != nil {
		t.Fatal(err)
	}
	if err := mutate(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("receipt replay invoked mutation %d times", calls)
	}
	var outbox, pts int
	if err := d.Pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM msg_state_delivery_outbox), (SELECT count(*) FROM user_pts_updates)`).Scan(&outbox, &pts); err != nil {
		t.Fatal(err)
	}
	if outbox != 1 || pts != 1 {
		t.Fatalf("receipt replay duplicated durable work: outbox=%d pts=%d", outbox, pts)
	}
}

func (s *messageIDGenStub) next(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key]++
	return s.values[key]
}

func (s *messageIDGenStub) current(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key]
}

func (s *messageIDGenStub) IdgenGetNextSeqId(_ context.Context, in *idgen.TLIdgenGetNextSeqId) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: s.next(in.Key)}, nil
}

func (s *messageIDGenStub) IdgenGetCurrentSeqId(_ context.Context, in *idgen.TLIdgenGetCurrentSeqId) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: s.current(in.Key)}, nil
}

func (s *messageIDGenStub) IdgenGetNextIdValList(_ context.Context, in *idgen.TLIdgenGetNextIdValList) (*idgen.Vector_IdVal, error) {
	result := &idgen.Vector_IdVal{}
	for _, input := range in.Id {
		key := input.Key
		if input.PredicateName == idgen.Predicate_inputId {
			key = "dialog_message_id"
		}
		result.Datas = append(result.Datas, idgen.MakeTLSeqIdVal(&idgen.IdVal{Id_INT64: s.next(key)}).To_IdVal())
	}
	return result, nil
}

type messageTestRPCClient struct{ conn *grpc.ClientConn }

func (c messageTestRPCClient) Conn() *grpc.ClientConn { return c.conn }

func newMessagePostgresFixture(t *testing.T) (*Dao, *messageIDGenStub) {
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
	if err := admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL 18 required, got %d", version)
	}
	schema := pgx.Identifier{fmt.Sprintf("messenger_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	})
	for _, table := range []string{"messages", "dialogs", "hash_tags", "user_pts_updates", "msg_inbox_delivery_outbox", "msg_state_delivery_outbox", "msg_inbox_consumer_receipts", "idgen_counters", "chats", "chat_participants", "users", "user_peer_blocks", "message_read_outbox", "saved_dialogs"} {
		name := pgx.Identifier{table}.Sanitize()
		if _, err := admin.Exec(ctx, `CREATE TABLE `+schema+`.`+name+` (LIKE public.`+name+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `INSERT INTO users(id, phone) VALUES (101,'fixture-101'),(202,'fixture-202'),(203,'fixture-203')`); err != nil {
		t.Fatal(err)
	}
	stub := &messageIDGenStub{values: make(map[string]int64)}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	idgen.RegisterRPCIdgenServer(server, stub)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &Dao{
		Postgres:     &Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)},
		IDGenClient2: idgen_client.NewIDGenClient2(messageTestRPCClient{conn: conn}),
	}, stub
}

func messageCounterValue(t *testing.T, d *Dao, key string) int64 {
	t.Helper()
	value, err := counter.NewCounterStore(d.Pool).Current(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func messageTestBox(userID, peerID, randomID int64, messageID int32) *mtproto.MessageBox {
	dialogID := mtproto.MakeDialogId(userID, mtproto.PEER_USER, peerID)
	return &mtproto.MessageBox{
		UserId: userID, SenderUserId: userID, PeerType: mtproto.PEER_USER, PeerId: peerID,
		MessageId: messageID, RandomId: randomID, DialogId1: dialogID.A, DialogId2: dialogID.B,
		DialogMessageId: 77, PtsCount: 1,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id: messageID, Out: true, PeerId: mtproto.MakePeerUser(peerID),
			Message: "postgres retry", Date: 1,
		}).To_Message(),
	}
}

func TestPostgresReadHistoryDoesNotAdvanceForOutgoingTarget(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	const userID, peerID = int64(101), int64(202)
	dialogID := mtproto.MakeDialogId(userID, mtproto.PEER_USER, peerID)
	if _, err := d.Pool.Exec(ctx, `
INSERT INTO messages
 (user_id,user_message_box_id,dialog_id1,dialog_id2,dialog_message_id,sender_user_id,peer_type,peer_id,message_data,message,date2)
 VALUES ($1,1,$2,$3,77,$1,$4,$5,'{}','outgoing',1)`, userID, dialogID.A, dialogID.B, mtproto.PEER_USER, peerID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Store.Dialogs.InsertOrUpdate(ctx, &dataobject.DialogsDO{
		UserId: userID, PeerType: mtproto.PEER_USER, PeerId: peerID,
		PeerDialogId: mtproto.MakePeerDialogId(mtproto.PEER_USER, peerID), TopMessage: 1,
		DraftMessageData: "null",
	}); err != nil {
		t.Fatal(err)
	}
	updates, pts, _, err := d.ReadHistoryState(ctx, userID, mtproto.MakePeerUtil(mtproto.PEER_USER, peerID), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 0 || pts != 0 {
		t.Fatalf("outgoing target produced updates=%d pts=%d", len(updates), pts)
	}
	var readMax int32
	if err := d.Pool.QueryRow(ctx, `SELECT read_inbox_max_id FROM dialogs WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, userID, mtproto.PEER_USER, peerID).Scan(&readMax); err != nil {
		t.Fatal(err)
	}
	if readMax != 0 {
		t.Fatalf("outgoing target advanced read_inbox_max_id to %d", readMax)
	}
}

func TestPostgresReadHistoryPersistsGroupReceiptDate(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	const readerID, senderID, chatID = int64(101), int64(202), int64(303)
	if _, err := d.Pool.Exec(ctx, `INSERT INTO chats (id, creator_user_id, access_hash, random_id, participant_count)
 VALUES ($1,$2,1,1,2)`, chatID, readerID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO chat_participants (chat_id,user_id,participant_type,state)
		 VALUES ($1,$2,$3,$4),($1,$5,$6,$4)`, chatID, readerID, mtproto.ChatMemberCreator, mtproto.ChatMemberStateNormal, senderID, mtproto.ChatMemberNormal); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO dialogs (user_id,peer_type,peer_id,peer_dialog_id,top_message)
	 VALUES ($1,$2,$3,$4,7),($5,$2,$3,$4,3)`, readerID, mtproto.PEER_CHAT, chatID, mtproto.MakePeerDialogId(mtproto.PEER_CHAT, chatID), senderID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO messages
 (user_id,user_message_box_id,dialog_id1,dialog_id2,dialog_message_id,sender_user_id,peer_type,peer_id,message_data,message,date2)
		 VALUES ($1,7,0,0,900,$2,$3,$4,'{}','incoming',1),
	        ($2,3,0,0,900,$2,$3,$4,'{}','incoming',1)`, readerID, senderID, mtproto.PEER_CHAT, chatID); err != nil {
		t.Fatal(err)
	}
	updates, _, _, err := d.ReadHistoryState(ctx, readerID, mtproto.MakePeerUtil(mtproto.PEER_CHAT, chatID), 7)
	if err != nil || len(updates) != 1 {
		t.Fatalf("read group history = updates=%d err=%v", len(updates), err)
	}
	var readUser, maxID, readDate int64
	if err := d.Pool.QueryRow(ctx, `SELECT read_user_id,read_outbox_max_id,read_outbox_max_date FROM message_read_outbox WHERE user_id=$1 AND peer_dialog_id=$2`, senderID, mtproto.MakePeerDialogId(mtproto.PEER_CHAT, chatID)).Scan(&readUser, &maxID, &readDate); err != nil {
		t.Fatal(err)
	}
	if readUser != readerID || maxID != 3 || readDate <= 0 {
		t.Fatalf("group receipt = user=%d max_id=%d date=%d, want reader=%d max_id=3 and positive date", readUser, maxID, readDate, readerID)
	}
}

func TestPostgresOutboxConcurrentRetryWritesOneUpdate(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	const userID, peerID, randomID = int64(101), int64(202), int64(303)
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, peerID)
	const retries = 8
	type result struct {
		box      *mtproto.MessageBox
		inserted bool
		err      error
	}
	results := make(chan result, retries)
	for i := 0; i < retries; i++ {
		go func() {
			box, inserted, err := d.SendUserMessage(ctx, userID, peerID, &msg.OutboxMessage{
				RandomId: randomID, Message: messageTestBox(userID, peerID, randomID, 0).Message,
			})
			results <- result{box, inserted, err}
		}()
	}
	var originalID int32
	insertions := 0
	for i := 0; i < retries; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("send failed: %v", r.err)
			continue
		}
		if originalID == 0 {
			originalID = r.box.MessageId
		}
		if r.box.MessageId != originalID {
			t.Errorf("retry message id = %d, want %d", r.box.MessageId, originalID)
		}
		if r.inserted {
			insertions++
		}
	}
	if insertions != 1 || messageCounterValue(t, d, counter.PtsKey(userID)) != 1 {
		t.Fatalf("insertions=%d pts allocations=%d", insertions, messageCounterValue(t, d, counter.PtsKey(userID)))
	}
	var messageCount, updateCount int
	if err := d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM messages), (SELECT count(*) FROM user_pts_updates)`).Scan(&messageCount, &updateCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 1 || updateCount != 1 {
		t.Fatalf("message/update counts = %d/%d, want 1/1", messageCount, updateCount)
	}
	stored, err := d.Store.Dialogs.SelectDialog(ctx, userID, peer.PeerType, peerID)
	if err != nil || stored == nil || stored.TopMessage != originalID {
		t.Fatalf("dialog projection = %#v, %v", stored, err)
	}
}

func TestPostgresInboxRetryAndRecipientIsolation(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	const senderID, randomID = int64(101), int64(303)
	for _, recipientID := range []int64{202, 203} {
		peer := mtproto.MakePeerUtil(mtproto.PEER_USER, recipientID)
		box := messageTestBox(recipientID, senderID, randomID, 1)
		data, err := jsonx.Marshal(box.Message)
		if err != nil {
			t.Fatal(err)
		}
		first, err := d.persistInboxPostgres(ctx, senderID, peer, recipientID, box, box.Message, data, 1)
		if err != nil {
			t.Fatal(err)
		}
		if first.Pts != 1 || first.PtsCount != 1 {
			t.Fatalf("new inbox pts = %d/%d", first.Pts, first.PtsCount)
		}
		retryBox := messageTestBox(recipientID, senderID, randomID, 2)
		retry, err := d.persistInboxPostgres(ctx, senderID, peer, recipientID, retryBox, retryBox.Message, []byte(`{}`), 1)
		if err != nil {
			t.Fatal(err)
		}
		if retry.MessageId != 1 || retry.PtsCount != 0 || messageCounterValue(t, d, "pts_updates_ngen_"+strconv.FormatInt(recipientID, 10)) != 1 {
			t.Fatalf("retry = %#v, allocated another pts", retry)
		}
		dialog, err := d.Store.Dialogs.SelectDialog(ctx, recipientID, mtproto.PEER_USER, senderID)
		if err != nil || dialog == nil || dialog.UnreadCount != 1 || dialog.TopMessage != 1 {
			t.Fatalf("retry dialog = %#v, %v", dialog, err)
		}
	}
	var messages, updates int
	if err := d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM messages), (SELECT count(*) FROM user_pts_updates)`).Scan(&messages, &updates); err != nil {
		t.Fatal(err)
	}
	if messages != 2 || updates != 2 {
		t.Fatalf("recipient rows = %d messages / %d updates, want 2 / 2", messages, updates)
	}
}

func TestPostgresMessageUpdateFailureRollsBack(t *testing.T) {
	for _, outbox := range []bool{false, true} {
		t.Run(fmt.Sprintf("outbox_%t", outbox), func(t *testing.T) {
			d, _ := newMessagePostgresFixture(t)
			ctx := context.Background()
			if _, err := d.Pool.Exec(ctx, `ALTER TABLE user_pts_updates ADD CONSTRAINT reject_test_update CHECK (user_id <> 202)`); err != nil {
				t.Fatal(err)
			}
			box := messageTestBox(202, 101, 303, 1)
			box.Message.Entities = []*mtproto.MessageEntity{mtproto.MakeTLMessageEntityHashtag(&mtproto.MessageEntity{Url: "postgres"}).To_MessageEntity()}
			peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 101)
			var err error
			if outbox {
				_, _, err = d.SendUserMessage(ctx, 202, 101, &msg.OutboxMessage{RandomId: 303, Message: box.Message})
			} else {
				_, err = d.persistInboxPostgres(ctx, 101, peer, 202, box, box.Message, []byte(`{}`), 1)
			}
			if err == nil {
				t.Fatal("update failure was not propagated")
			}
			if !strings.Contains(err.Error(), "reject_test_update") {
				t.Fatalf("unexpected failure before update constraint: %v", err)
			}
			for _, table := range []string{"messages", "dialogs", "hash_tags", "user_pts_updates"} {
				var count int
				if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Errorf("%s retained %d rows after rollback", table, count)
				}
			}
		})
	}
}

func TestPostgresPtsCollisionCannotCommitMessage(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	if _, _, err := d.Store.UserPtsUpdates.Insert(ctx, &dataobject.UserPtsUpdatesDO{
		UserId: 101, Pts: 1, PtsCount: 1, UpdateType: 0, UpdateData: `{}`, Date2: 1,
	}); err != nil {
		t.Fatal(err)
	}
	box := messageTestBox(101, 202, 303, 1)
	box.Pts = 1
	inserted, err := d.SendMessageToOutboxV1(ctx, 101, mtproto.MakePeerUtil(mtproto.PEER_USER, 202), box)
	if err == nil || inserted {
		t.Fatalf("pts collision result = %t, %v", inserted, err)
	}
	var count int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM messages`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("pts collision left %d messages", count)
	}
}

func TestPostgresMissingPtsStoreFails(t *testing.T) {
	d := &Dao{}
	if _, err := d.AddToPtsQueueOn(context.Background(), nil, 101, 1, 1, mtproto.MakeTLUpdateNewMessage(nil).To_Update()); err == nil {
		t.Fatal("unconfigured pts store must fail")
	}
}

func TestPostgresSendBatchCommitsDeliveriesAndRetriesOriginalPts(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 202)
	recipients := []*inbox.TLInboxSendUserMessageToInboxV2{
		{UserId: 101, Out: true, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
		{UserId: 202, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
	}
	messages := []*msg.OutboxMessage{
		{RandomId: 303, Message: messageTestBox(101, 202, 303, 0).Message},
		{RandomId: 304, Message: messageTestBox(101, 202, 304, 0).Message},
	}
	boxes, err := d.SendMessagesWithDeliveries(ctx, 101, peer, messages, recipients)
	if err != nil {
		t.Fatal(err)
	}
	if len(boxes) != 2 || boxes[0].Pts != 1 || boxes[1].Pts != 2 {
		t.Fatalf("batch boxes = %#v", boxes)
	}
	var messageCount, updateCount, deliveryCount int
	if err := d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM messages), (SELECT count(*) FROM user_pts_updates), (SELECT count(*) FROM msg_inbox_delivery_outbox)`).Scan(&messageCount, &updateCount, &deliveryCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 2 || updateCount != 2 || deliveryCount != 4 {
		t.Fatalf("batch state = %d messages / %d updates / %d deliveries", messageCount, updateCount, deliveryCount)
	}
	retry, err := d.SendMessagesWithDeliveries(ctx, 101, peer, messages, recipients)
	if err != nil {
		t.Fatal(err)
	}
	for i, box := range retry {
		if box.MessageId != boxes[i].MessageId || box.Pts != boxes[i].Pts || box.DialogMessageId != boxes[i].DialogMessageId {
			t.Fatalf("retry %d changed original message: %#v", i, box)
		}
	}
	if messageCounterValue(t, d, counter.PtsKey(101)) != 2 {
		t.Fatal("batch retry allocated another pts")
	}
}

func TestPostgresSendBatchRollbackRemovesEarlierMessageAndIntents(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	if _, err := d.Pool.Exec(ctx, `ALTER TABLE user_pts_updates ADD CONSTRAINT reject_second_update CHECK (pts <> 2)`); err != nil {
		t.Fatal(err)
	}
	_, err := d.SendMessagesWithDeliveries(ctx, 101, mtproto.MakePeerUtil(mtproto.PEER_USER, 202), []*msg.OutboxMessage{
		{RandomId: 303, Message: messageTestBox(101, 202, 303, 0).Message},
		{RandomId: 304, Message: messageTestBox(101, 202, 304, 0).Message},
	}, []*inbox.TLInboxSendUserMessageToInboxV2{
		{UserId: 101, Out: true, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
		{UserId: 202, FromId: 101, PeerType: mtproto.PEER_USER, PeerId: 202},
	})
	if err == nil || !strings.Contains(err.Error(), "reject_second_update") {
		t.Fatalf("batch failure = %v", err)
	}
	for _, table := range []string{"messages", "dialogs", "hash_tags", "user_pts_updates", "msg_inbox_delivery_outbox", "idgen_counters"} {
		var count int
		if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("batch rollback left %d rows in %s", count, table)
		}
	}
}

func TestPostgresZeroRandomIDConsumerReplay(t *testing.T) {
	for _, sender := range []bool{false, true} {
		t.Run(fmt.Sprintf("sender_%t", sender), func(t *testing.T) {
			d, _ := newMessagePostgresFixture(t)
			ctx := context.Background()
			const senderID, recipientID = int64(101), int64(202)
			peer := mtproto.MakePeerUtil(mtproto.PEER_USER, recipientID)
			outbox := messageTestBox(senderID, recipientID, 0, 1)
			if sender {
				inserted, err := d.SendMessageToOutboxV1(ctx, senderID, peer, outbox)
				if err != nil || !inserted {
					t.Fatalf("first sender write=%t err=%v", inserted, err)
				}
				for i := 0; i < 3; i++ {
					replay := messageTestBox(senderID, recipientID, 0, int32(i+1))
					inserted, err := d.SendMessageToOutboxV1(ctx, senderID, peer, replay)
					if err != nil || inserted || replay.MessageId != 1 {
						t.Fatalf("sender replay=%t id=%d err=%v", inserted, replay.MessageId, err)
					}
				}
			} else {
				for i := 0; i < 3; i++ {
					box, err := d.SendUserMessageToInbox(ctx, senderID, recipientID, outbox.DialogMessageId, 0, outbox.Message)
					if err != nil || box.MessageId != 1 || box.Pts != 1 {
						t.Fatalf("recipient replay=%#v err=%v", box, err)
					}
					if i > 0 && box.PtsCount != 0 {
						t.Fatalf("recipient replay allocated new pts: %#v", box)
					}
				}
			}
			var messageCount, updateCount int
			if err := d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM messages), (SELECT count(*) FROM user_pts_updates)`).Scan(&messageCount, &updateCount); err != nil {
				t.Fatal(err)
			}
			if messageCount != 1 || updateCount != 1 {
				t.Fatalf("replay messages/updates=%d/%d", messageCount, updateCount)
			}
		})
	}
}

func TestPostgresOutgoingPreservesDraftAndUnreadCount(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 202)
	if _, _, err := d.Store.Dialogs.InsertOrUpdateOn(ctx, d.Pool, &dataobject.DialogsDO{
		UserId: 101, PeerType: peer.PeerType, PeerId: peer.PeerId,
		PeerDialogId: mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId),
		UnreadCount:  4, DraftMessageData: `{"message":"keep this draft"}`, Date2: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.SendUserMessage(ctx, 101, 202, &msg.OutboxMessage{RandomId: 303, Message: messageTestBox(101, 202, 303, 0).Message}); err != nil {
		t.Fatal(err)
	}
	var hasDraft bool
	var unread, top int32
	if err := d.Pool.QueryRow(ctx, `SELECT draft_message_data->>'message' = 'keep this draft', unread_count, top_message FROM dialogs WHERE user_id=101`).Scan(&hasDraft, &unread, &top); err != nil {
		t.Fatal(err)
	}
	if !hasDraft || unread != 4 || top != 1 {
		t.Fatalf("sender destroyed projection: hasDraft=%t unread=%d top=%d", hasDraft, unread, top)
	}
}

func TestPostgresConcurrentIncomingOutgoingOrdering(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	const operations = 16
	errors := make(chan error, operations)
	for i := 0; i < operations; i++ {
		go func(i int) {
			var err error
			if i%2 == 0 {
				_, _, err = d.SendUserMessage(ctx, 101, 202, &msg.OutboxMessage{RandomId: int64(300 + i), Message: messageTestBox(101, 202, int64(300+i), 0).Message})
			} else {
				_, err = d.SendUserMessageToInbox(ctx, 202, 101, int64(700+i), int64(300+i), messageTestBox(202, 101, int64(300+i), 0).Message)
			}
			errors <- err
		}(i)
	}
	for i := 0; i < operations; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	var top, unread, messageCount, updateCount int32
	if err := d.Pool.QueryRow(ctx, `SELECT top_message, unread_count,
 (SELECT count(*) FROM messages WHERE user_id=101), (SELECT count(*) FROM user_pts_updates WHERE user_id=101)
 FROM dialogs WHERE user_id=101 AND peer_id=202`).Scan(&top, &unread, &messageCount, &updateCount); err != nil {
		t.Fatal(err)
	}
	if top != operations || unread != operations/2 || messageCount != operations || updateCount != operations {
		t.Fatalf("concurrent top/unread/messages/updates=%d/%d/%d/%d", top, unread, messageCount, updateCount)
	}
	rows, err := d.Pool.Query(ctx, `SELECT pts, (update_data::jsonb->'message_MESSAGE'->>'id')::integer FROM user_pts_updates WHERE user_id=101 ORDER BY pts`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ordinal int32
	for rows.Next() {
		ordinal++
		var pts, messageID int32
		if err := rows.Scan(&pts, &messageID); err != nil {
			t.Fatal(err)
		}
		if pts != ordinal || messageID != ordinal {
			t.Fatalf("committed pts/message=%d/%d, want %d", pts, messageID, ordinal)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresClearMentionsWithoutLegacyCache(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	peer := mtproto.MakePeerUtil(mtproto.PEER_CHAT, 303)
	box := messageTestBox(202, 101, 0, 1)
	box.PeerType, box.PeerId, box.Mentioned = peer.PeerType, peer.PeerId, true
	box.Message.PeerId, box.Message.Mentioned = mtproto.MakePeerChat(303), true
	data, err := jsonx.Marshal(box.Message)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.persistInboxPostgres(ctx, 101, peer, 202, box, box.Message, data, 1); err != nil {
		t.Fatal(err)
	}
	cleared, err := d.ClearMentions(ctx, 202, 303, 1, true)
	if err != nil || cleared != 1 {
		t.Fatalf("clear mentions=%d err=%v", cleared, err)
	}
	var unread int32
	if err := d.Pool.QueryRow(ctx, `SELECT unread_mentions_count FROM dialogs WHERE user_id=202 AND peer_id=303`).Scan(&unread); err != nil || unread != 0 {
		t.Fatalf("remaining mentions=%d err=%v", unread, err)
	}
}

func TestPostgresChatPermissionAndRecipientSnapshot(t *testing.T) {
	for _, state := range []int32{mtproto.ChatMemberStateNormal, mtproto.ChatMemberStateLeft, mtproto.ChatMemberStateKicked, mtproto.ChatMemberStateMigrated} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			d, _ := newMessagePostgresFixture(t)
			ctx := context.Background()
			if _, err := d.Pool.Exec(ctx, `INSERT INTO chats(id,creator_user_id,access_hash,random_id) VALUES (303,101,1,1);
 INSERT INTO chat_participants(chat_id,user_id,state) VALUES (303,101,0),(303,202,0);`); err != nil {
				t.Fatal(err)
			}
			if _, err := d.Pool.Exec(ctx, `UPDATE chat_participants SET state=$1 WHERE user_id=101`, state); err != nil {
				t.Fatal(err)
			}
			peer := mtproto.MakePeerUtil(mtproto.PEER_CHAT, 303)
			recipients := []*inbox.TLInboxSendUserMessageToInboxV2{
				{UserId: 101, Out: true, FromId: 101, PeerType: peer.PeerType, PeerId: peer.PeerId},
				{UserId: 202, FromId: 101, PeerType: peer.PeerType, PeerId: peer.PeerId},
			}
			out := &msg.OutboxMessage{RandomId: 304, Message: mtproto.MakeTLMessage(&mtproto.Message{PeerId: mtproto.MakePeerChat(303), Message: "member send", Date: 1}).To_Message()}
			_, err := d.SendMessagesWithDeliveries(ctx, 101, peer, []*msg.OutboxMessage{out}, recipients)
			if state == mtproto.ChatMemberStateNormal {
				if err != nil {
					t.Fatal(err)
				}
			} else if err != mtproto.ErrChatWriteForbidden {
				t.Fatalf("inactive sender allowed: %v", err)
			}
			if state != mtproto.ChatMemberStateNormal {
				for _, table := range []string{"messages", "dialogs", "user_pts_updates", "msg_inbox_delivery_outbox", "idgen_counters"} {
					var count int
					if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
						t.Fatalf("rejected send retained %s rows=%d err=%v", table, count, err)
					}
				}
			}
		})
	}
}

func TestPostgresSendRetryRechecksPermission(t *testing.T) {
	for _, kind := range []string{"deleted_user", "kicked_member", "migrated_chat"} {
		t.Run(kind, func(t *testing.T) {
			d, _ := newMessagePostgresFixture(t)
			ctx := context.Background()
			peer := mtproto.MakePeerUtil(mtproto.PEER_USER, 202)
			want := mtproto.ErrInputUserDeactivated
			if kind != "deleted_user" {
				peer = mtproto.MakePeerUtil(mtproto.PEER_CHAT, 303)
				want = mtproto.ErrChatWriteForbidden
				if _, err := d.Pool.Exec(ctx, `INSERT INTO chats(id,creator_user_id,access_hash,random_id) VALUES (303,101,1,1);
 INSERT INTO chat_participants(chat_id,user_id,state) VALUES (303,101,0),(303,202,0)`); err != nil {
					t.Fatal(err)
				}
			}
			recipients := []*inbox.TLInboxSendUserMessageToInboxV2{
				{UserId: 101, Out: true, FromId: 101, PeerType: peer.PeerType, PeerId: peer.PeerId},
				{UserId: 202, FromId: 101, PeerType: peer.PeerType, PeerId: peer.PeerId},
			}
			out := &msg.OutboxMessage{RandomId: 304, Message: mtproto.MakeTLMessage(&mtproto.Message{
				PeerId: mtproto.MakePeer(peer.PeerType, peer.PeerId), Message: "authorized send", Date: 1,
			}).To_Message()}
			if _, err := d.SendMessagesWithDeliveries(ctx, 101, peer, []*msg.OutboxMessage{out}, recipients); err != nil {
				t.Fatal(err)
			}
			var query string
			switch kind {
			case "deleted_user":
				query = `UPDATE users SET deleted=TRUE WHERE id=202`
			case "kicked_member":
				query = `UPDATE chat_participants SET state=2 WHERE user_id=101`
			case "migrated_chat":
				query = `UPDATE chats SET migrated_to_id=404 WHERE id=303`
			}
			if _, err := d.Pool.Exec(ctx, query); err != nil {
				t.Fatal(err)
			}
			if _, err := d.SendMessagesWithDeliveries(ctx, 101, peer, []*msg.OutboxMessage{out}, recipients); err != want {
				t.Fatalf("retry permission error=%v, want %v", err, want)
			}
			if value := messageCounterValue(t, d, counter.PtsKey(101)); value != 1 {
				t.Fatalf("rejected retry advanced pts to %d", value)
			}
		})
	}
}
