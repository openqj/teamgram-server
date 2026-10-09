package dao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	inboxclient "github.com/teamgram/teamgram-server/app/messenger/msg/inbox/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type inboxDeliveryTestClient struct {
	inboxclient.InboxClient
	invoke func(context.Context, *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error)
}

func (c inboxDeliveryTestClient) InboxSendUserMessageToInboxV2(ctx context.Context, in *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
	return c.invoke(ctx, in)
}

func newInboxDeliveryPostgresFixture(t *testing.T) *Dao {
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
	if err = admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL 18 required, got %d", version)
	}
	schema := pgx.Identifier{fmt.Sprintf("inbox_delivery_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean delivery test schema: %v", err)
		}
	})
	if _, err = admin.Exec(ctx, `CREATE TABLE `+schema+`.msg_inbox_delivery_outbox (LIKE public.msg_inbox_delivery_outbox INCLUDING ALL)`); err != nil {
		t.Fatal(err)
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
	return &Dao{Postgres: &Postgres{Pool: pool}}
}

func inboxDeliveryRequest(senderID, dialogID, recipientID int64) *inbox.TLInboxSendUserMessageToInboxV2 {
	return &inbox.TLInboxSendUserMessageToInboxV2{
		UserId: recipientID, FromId: senderID, PeerType: mtproto.PEER_USER, PeerId: recipientID,
		BoxList: []*mtproto.MessageBox{{
			UserId: senderID, SenderUserId: senderID, MessageId: 7, DialogMessageId: dialogID,
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Id: 7, Date: 1, Out: true, PeerId: mtproto.MakePeerUser(recipientID), Message: "durable inbox delivery",
			}).To_Message(),
		}},
		ClientReqMsgId: wrapperspb.Int64(9007199254740993),
	}
}

func enqueueInboxDelivery(t *testing.T, d *Dao, in *inbox.TLInboxSendUserMessageToInboxV2) {
	t.Helper()
	if err := d.Postgres.InTx(context.Background(), func(tx pgx.Tx) error {
		return d.EnqueueInboxDeliveryOn(context.Background(), tx, in)
	}); err != nil {
		t.Fatal(err)
	}
}

func inboxDeliveryDueNow(t *testing.T, d *Dao) {
	t.Helper()
	if _, err := d.Pool.Exec(context.Background(), `UPDATE msg_inbox_delivery_outbox
		SET available_at=now()-interval '1 second', lease_until=NULL, claim_token=NULL WHERE state=0`); err != nil {
		t.Fatal(err)
	}
}

func TestInboxDeliveryPostgresEnqueueRollsBackAndDeduplicates(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	request := inboxDeliveryRequest(101, 9007199254740995, 202)
	cause := errors.New("abort sender transaction")
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if err := d.EnqueueInboxDeliveryOn(ctx, tx, request); err != nil {
			return err
		}
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("rollback result = %v", err)
	}
	pending, err := d.HasPendingInboxDelivery(ctx, 101, request.BoxList[0].DialogMessageId, 202)
	if err != nil || pending {
		t.Fatalf("rolled-back intent pending = %v, %v", pending, err)
	}
	enqueueInboxDelivery(t, d, request)
	enqueueInboxDelivery(t, d, request)
	var count int
	if err = d.Pool.QueryRow(ctx, `SELECT count(*) FROM msg_inbox_delivery_outbox`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("deduplicated intent count = %d, %v", count, err)
	}
	if err = d.EnqueueInboxDeliveryOn(ctx, nil, request); err == nil {
		t.Fatal("nil transaction accepted")
	}
	request.BoxList = append(request.BoxList, request.BoxList[0])
	if err = d.Postgres.InTx(ctx, func(tx pgx.Tx) error { return d.EnqueueInboxDeliveryOn(ctx, tx, request) }); err == nil {
		t.Fatal("multi-box intent accepted")
	}
}

func TestInboxDeliveryPostgresPublishWaitsForConsumerAck(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	request := inboxDeliveryRequest(9007199254740993, 9007199254740995, 9007199254740997)
	enqueueInboxDelivery(t, d, request)
	var calls atomic.Int32
	d.InboxClient = inboxDeliveryTestClient{invoke: func(_ context.Context, in *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		calls.Add(1)
		if in.FromId != request.FromId || in.UserId != request.UserId || len(in.BoxList) != 1 ||
			in.BoxList[0].DialogMessageId != request.BoxList[0].DialogMessageId || in.ClientReqMsgId.GetValue() != request.ClientReqMsgId.GetValue() {
			return nil, errors.New("intent payload changed during roundtrip")
		}
		return mtproto.EmptyVoid, nil
	}}
	if count, err := d.DispatchInboxDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("published count = %d, %v", count, err)
	}
	pending, err := d.HasPendingInboxDelivery(ctx, request.FromId, request.BoxList[0].DialogMessageId, request.UserId)
	if err != nil || !pending {
		t.Fatalf("published but unacknowledged intent = %v, %v", pending, err)
	}
	if count, err := d.DispatchInboxDeliveries(ctx, 10); err != nil || count != 0 {
		t.Fatalf("early resend count = %d, %v", count, err)
	}
	inboxDeliveryDueNow(t, d)
	if count, err := d.DispatchInboxDeliveries(ctx, 10); err != nil || count != 1 || calls.Load() != 2 {
		t.Fatalf("unacknowledged resend count = %d calls = %d, %v", count, calls.Load(), err)
	}
	for i := 0; i < 2; i++ {
		if err := d.CompleteInboxDelivery(ctx, request.FromId, request.BoxList[0].DialogMessageId, request.UserId); err != nil {
			t.Fatal(err)
		}
	}
	pending, err = d.HasPendingInboxDelivery(ctx, request.FromId, request.BoxList[0].DialogMessageId, request.UserId)
	if err != nil || pending {
		t.Fatalf("acknowledged intent pending = %v, %v", pending, err)
	}
	var state int16
	var deliveredAt *time.Time
	var payloadCleared bool
	if err := d.Pool.QueryRow(ctx, `SELECT state, delivered_at, payload='{}'::jsonb FROM msg_inbox_delivery_outbox`).Scan(&state, &deliveredAt, &payloadCleared); err != nil || state != 1 || deliveredAt == nil || !payloadCleared {
		t.Fatalf("completion state = %d at = %v payload cleared = %v, %v", state, deliveredAt, payloadCleared, err)
	}
}

func TestInboxDeliveryPostgresMissingPublishResultRemainsPending(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 1, 202))
	d.InboxClient = inboxDeliveryTestClient{invoke: func(context.Context, *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		return nil, nil
	}}
	if count, err := d.DispatchInboxDeliveries(ctx, 1); err == nil || count != 0 {
		t.Fatalf("missing result publication = %d, %v", count, err)
	}
	if pending, err := d.HasPendingInboxDelivery(ctx, 101, 1, 202); err != nil || !pending {
		t.Fatalf("missing publish acknowledgement discarded intent: %v, %v", pending, err)
	}
}

func TestInboxDeliveryPostgresPublishFailureRetriesAndPreservesOrder(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	for _, request := range []*inbox.TLInboxSendUserMessageToInboxV2{
		inboxDeliveryRequest(101, 1, 202), inboxDeliveryRequest(101, 2, 202), inboxDeliveryRequest(101, 3, 203),
	} {
		enqueueInboxDelivery(t, d, request)
	}
	d.InboxClient = inboxDeliveryTestClient{invoke: func(_ context.Context, in *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		if in.UserId == 202 {
			return nil, errors.New("broker unavailable")
		}
		return mtproto.EmptyVoid, nil
	}}
	if count, err := d.DispatchInboxDeliveries(ctx, 10); count != 1 || err == nil || !strings.Contains(err.Error(), "broker unavailable") {
		t.Fatalf("failure dispatch count = %d, %v", count, err)
	}
	var attempts int
	var lastError string
	var availableAt time.Time
	if err := d.Pool.QueryRow(ctx, `SELECT attempts, last_error, available_at FROM msg_inbox_delivery_outbox WHERE dialog_message_id=1`).Scan(&attempts, &lastError, &availableAt); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastError != "broker unavailable" || !availableAt.After(time.Now().Add(-500*time.Millisecond)) {
		t.Fatalf("retry metadata = %d %q %v", attempts, lastError, availableAt)
	}
	if err := d.Pool.QueryRow(ctx, `SELECT attempts FROM msg_inbox_delivery_outbox WHERE dialog_message_id=2`).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("later recipient intent attempts = %d, %v", attempts, err)
	}
	if err := d.CompleteInboxDelivery(ctx, 101, 1, 202); err != nil {
		t.Fatal(err)
	}
	d.InboxClient = inboxDeliveryTestClient{invoke: func(_ context.Context, in *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		if in.BoxList[0].DialogMessageId != 2 {
			return nil, errors.New("wrong next recipient message")
		}
		return mtproto.EmptyVoid, nil
	}}
	if count, err := d.DispatchInboxDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("ordered next dispatch = %d, %v", count, err)
	}
}

func TestInboxDeliveryPostgresLeaseRecoveryRejectsStaleRelease(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 1, 202))
	first, err := d.claimInboxDelivery(ctx)
	if err != nil || first == nil {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	if second, err := d.claimInboxDelivery(ctx); err != nil || second != nil {
		t.Fatalf("unexpired lease claim = %v, %v", second, err)
	}
	if _, err = d.Pool.Exec(ctx, `UPDATE msg_inbox_delivery_outbox SET lease_until=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	second, err := d.claimInboxDelivery(ctx)
	if err != nil || second == nil || second.token == first.token || second.attempts != 2 {
		t.Fatalf("recovered claim = %v, %v", second, err)
	}
	if err = d.releaseInboxDelivery(ctx, first, errors.New("stale failure")); err != nil {
		t.Fatal(err)
	}
	var token, lastError string
	if err = d.Pool.QueryRow(ctx, `SELECT claim_token::text, last_error FROM msg_inbox_delivery_outbox`).Scan(&token, &lastError); err != nil || token != second.token || lastError != "" {
		t.Fatalf("stale release changed current claim: %q %q, %v", token, lastError, err)
	}
	second.attempts = 100
	if err = d.releaseInboxDelivery(ctx, second, errors.New("repeated failure")); err != nil {
		t.Fatal(err)
	}
	var delaySeconds float64
	if err = d.Pool.QueryRow(ctx, `SELECT extract(epoch from available_at-now()) FROM msg_inbox_delivery_outbox`).Scan(&delaySeconds); err != nil || delaySeconds < 299 || delaySeconds > 301 {
		t.Fatalf("capped retry delay = %v, %v", delaySeconds, err)
	}
}

func TestInboxDeliveryPostgresAckRacesWorkerRelease(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	request := inboxDeliveryRequest(101, 1, 202)
	enqueueInboxDelivery(t, d, request)
	d.InboxClient = inboxDeliveryTestClient{invoke: func(ctx context.Context, _ *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		if err := d.CompleteInboxDelivery(ctx, 101, 1, 202); err != nil {
			return nil, err
		}
		return mtproto.EmptyVoid, nil
	}}
	if count, err := d.DispatchInboxDeliveries(ctx, 10); err != nil || count != 1 {
		t.Fatalf("ack during publish count = %d, %v", count, err)
	}
	pending, err := d.HasPendingInboxDelivery(ctx, 101, 1, 202)
	if err != nil || pending {
		t.Fatalf("worker release resurrected completion: %v, %v", pending, err)
	}
}

func TestInboxDeliveryPostgresConcurrentClaimsRespectRecipientOrder(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	for recipientID := int64(202); recipientID < 205; recipientID++ {
		enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 1, recipientID))
		enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 2, recipientID))
	}
	locked, err := d.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Rollback(ctx)
	if _, err = locked.Exec(ctx, `SELECT id FROM msg_inbox_delivery_outbox WHERE recipient_user_id=202 AND dialog_message_id=1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	type result struct {
		claim *inboxDeliveryClaim
		err   error
	}
	results := make(chan result, 6)
	for i := 0; i < cap(results); i++ {
		go func() {
			claim, err := d.claimInboxDelivery(ctx)
			results <- result{claim, err}
		}()
	}
	claimed := map[int64]bool{}
	for i := 0; i < cap(results); i++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.claim == nil {
			continue
		}
		var recipientID, dialogID int64
		if err := d.Pool.QueryRow(ctx, `SELECT recipient_user_id, dialog_message_id FROM msg_inbox_delivery_outbox WHERE id=$1`, result.claim.id).Scan(&recipientID, &dialogID); err != nil {
			t.Fatal(err)
		}
		if recipientID == 202 || claimed[recipientID] || dialogID != 1 {
			t.Fatalf("claim skipped lock or predecessor incorrectly: recipient=%d dialog=%d", recipientID, dialogID)
		}
		claimed[recipientID] = true
	}
	if len(claimed) != 2 {
		t.Fatalf("unique concurrent recipient claims = %d, want 2", len(claimed))
	}
	if err = locked.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if claim, err := d.claimInboxDelivery(ctx); err != nil || claim == nil {
		t.Fatalf("released recipient claim = %v, %v", claim, err)
	}
}

func TestInboxDeliveryPostgresTimeoutKeepsOneInflightPublication(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 1, 202))
	started := make(chan struct{}, 1)
	unblock := make(chan struct{})
	completed := make(chan struct{})
	var calls atomic.Int32
	d.InboxClient = inboxDeliveryTestClient{invoke: func(_ context.Context, _ *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		calls.Add(1)
		started <- struct{}{}
		<-unblock
		close(completed)
		return mtproto.EmptyVoid, nil
	}}
	t.Cleanup(func() {
		close(unblock)
		select {
		case <-completed:
		case <-time.After(time.Second):
			t.Error("blocked publication did not finish")
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := d.DispatchInboxDeliveries(ctx, 1)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("publication did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled dispatch = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch ignored cancellation")
	}
	var lease *time.Time
	if err := d.Pool.QueryRow(context.Background(), `SELECT lease_until FROM msg_inbox_delivery_outbox`).Scan(&lease); err != nil || lease == nil {
		t.Fatalf("ambiguous publication lost lease: %v, %v", lease, err)
	}
	for i := 0; i < 2; i++ {
		inboxDeliveryDueNow(t, d)
		retryCtx, cancelRetry := context.WithTimeout(context.Background(), 25*time.Millisecond)
		_, err := d.DispatchInboxDeliveries(retryCtx, 1)
		cancelRetry()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("in-flight retry = %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent publications after timeout = %d", calls.Load())
	}
}

func TestInboxDeliveryPostgresWorkerStopsBeforePoolClose(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 1, 202))
	started := make(chan struct{}, 1)
	d.InboxClient = inboxDeliveryTestClient{invoke: func(ctx context.Context, _ *inbox.TLInboxSendUserMessageToInboxV2) (*mtproto.Void, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	stop := d.StartInboxDeliveryWorker(context.Background())
	t.Cleanup(stop)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker stop did not wait and finish")
	}
	if pending, err := d.HasPendingInboxDelivery(context.Background(), 101, 1, 202); err != nil || !pending {
		t.Fatalf("cancelled worker discarded intent: %v, %v", pending, err)
	}
}
