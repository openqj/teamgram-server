package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	inboxDeliveryPublishTimeout       = 30 * time.Second
	defaultDeliveryPublishConcurrency = 16
)

type inboxDeliveryClaim struct {
	id          int64
	token       string
	attempts    int
	maxAttempts int
	createdAt   time.Time
	payload     []byte
}

type inboxDeliveryPublication struct {
	done chan struct{}
	err  error
}

type deliveryPublicationKey struct {
	owner *Dao
	key   string
}

// Entries exist only while a producer call is active. The MQ adapter may
// ignore context cancellation, so retries must share that one in-flight call.
var inboxDeliveryPublications sync.Map

func (d *Dao) deliveryConcurrency() int {
	concurrency := d.DeliveryConcurrency
	if concurrency <= 0 {
		concurrency = defaultDeliveryPublishConcurrency
	}
	if concurrency > 128 {
		concurrency = 128
	}
	return concurrency
}

// runDeliveryWorkers claims and publishes independent intents concurrently.
// The claim queries retain one ordered row per recipient/user, so parallel
// workers only make progress on independent keys. A bounded worker count keeps
// broker and PostgreSQL pressure explicit.
func runDeliveryWorkers[T any](ctx context.Context, limit, concurrency int,
	claim func(context.Context) (*T, error), dispatch func(context.Context, *T) (int, error)) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > limit {
		concurrency = limit
	}
	var next atomic.Int32
	var published atomic.Int64
	var wg sync.WaitGroup
	var errorsMu sync.Mutex
	var dispatchErrors []error
	appendError := func(err error) {
		if err == nil {
			return
		}
		errorsMu.Lock()
		dispatchErrors = append(dispatchErrors, err)
		errorsMu.Unlock()
	}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				if int(next.Add(1)) > limit {
					return
				}
				item, err := claim(ctx)
				if err != nil {
					appendError(err)
					return
				}
				if item == nil {
					return
				}
				count, err := dispatch(ctx, item)
				published.Add(int64(count))
				appendError(err)
			}
		}()
	}
	wg.Wait()
	errorsMu.Lock()
	defer errorsMu.Unlock()
	return int(published.Load()), errors.Join(dispatchErrors...)
}

// EnqueueInboxDeliveryOn records one recipient intent alongside the sender's
// message. Only the consumer can acknowledge it after durable processing.
func (d *Dao) EnqueueInboxDeliveryOn(ctx context.Context, tx pgx.Tx, in *inbox.TLInboxSendUserMessageToInboxV2) error {
	if tx == nil {
		return errors.New("messenger/msg: inbox delivery requires a transaction")
	}
	if in == nil || len(in.BoxList) != 1 || in.BoxList[0] == nil || in.BoxList[0].Message == nil ||
		in.FromId <= 0 || in.UserId <= 0 || in.PeerId <= 0 ||
		(in.PeerType != mtproto.PEER_USER && in.PeerType != mtproto.PEER_CHAT) ||
		in.BoxList[0].MessageId <= 0 || in.BoxList[0].DialogMessageId <= 0 {
		return errors.New("messenger/msg: invalid inbox delivery intent")
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	box := in.BoxList[0]
	_, err = tx.Exec(ctx, `INSERT INTO msg_inbox_delivery_outbox
		(sender_user_id, sender_message_id, dialog_message_id, peer_type, peer_id, recipient_user_id, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)
		ON CONFLICT (sender_user_id, dialog_message_id, recipient_user_id) DO NOTHING`,
		in.FromId, box.MessageId, box.DialogMessageId, in.PeerType, in.PeerId, in.UserId, payload)
	return err
}

func (d *Dao) HasPendingInboxDelivery(ctx context.Context, senderID, dialogID, recipientID int64) (bool, error) {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil {
		return false, errors.New("messenger/msg: inbox delivery store is not configured")
	}
	var pending bool
	err := d.Postgres.Pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM msg_inbox_delivery_outbox
		WHERE sender_user_id=$1 AND dialog_message_id=$2 AND recipient_user_id=$3 AND state=0)`,
		senderID, dialogID, recipientID).Scan(&pending)
	return pending, err
}

// CompleteInboxDelivery is idempotent. A publication or stale worker claim
// cannot complete an intent; completion follows successful consumer handling.
func (d *Dao) CompleteInboxDelivery(ctx context.Context, senderID, dialogID, recipientID int64) error {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil {
		return errors.New("messenger/msg: inbox delivery store is not configured")
	}
	var elapsed float64
	completed := false
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE msg_inbox_delivery_outbox
			SET state=1, delivered_at=now(), payload='{}'::jsonb, lease_until=NULL, claim_token=NULL, last_error=''
			WHERE sender_user_id=$1 AND dialog_message_id=$2 AND recipient_user_id=$3 AND state=0
			RETURNING extract(epoch FROM delivered_at-created_at)::double precision`,
			senderID, dialogID, recipientID).Scan(&elapsed)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		completed = err == nil
		return err
	})
	if err == nil && completed {
		deliveryCompletion.WithLabelValues("inbox", "consumer_ack").Observe(max(elapsed, 0))
	}
	return err
}

func (d *Dao) claimInboxDelivery(ctx context.Context) (*inboxDeliveryClaim, error) {
	var claim *inboxDeliveryClaim
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		row := new(inboxDeliveryClaim)
		err := tx.QueryRow(ctx, `WITH terminalized AS (
			UPDATE msg_inbox_delivery_outbox
			SET state=2, dead_lettered_at=COALESCE(dead_lettered_at, now()),
				terminal_reason=CASE WHEN terminal_reason='' THEN 'max_attempts_exceeded' ELSE terminal_reason END,
				last_error=CASE WHEN last_error='' THEN 'max_attempts_exceeded' ELSE last_error END,
				lease_until=NULL, claim_token=NULL
			WHERE state=0 AND attempts>=max_attempts AND (lease_until IS NULL OR lease_until<=now())
			RETURNING id
		), due AS (
			SELECT o.id FROM msg_inbox_delivery_outbox o
			WHERE o.state=0 AND o.attempts<o.max_attempts AND o.available_at<=now() AND (o.lease_until IS NULL OR o.lease_until<=now())
			AND NOT EXISTS (
				SELECT 1 FROM msg_inbox_delivery_outbox prior
				WHERE prior.recipient_user_id=o.recipient_user_id AND prior.state=0 AND prior.id<o.id)
			ORDER BY o.id LIMIT 1 FOR UPDATE OF o SKIP LOCKED
		)
		UPDATE msg_inbox_delivery_outbox o
		SET claim_token=gen_random_uuid(), lease_until=now()+interval '60 seconds', attempts=o.attempts+1
		FROM due WHERE o.id=due.id
			RETURNING o.id, o.claim_token::text, o.attempts, o.max_attempts, o.created_at, o.payload`,
		).Scan(&row.id, &row.token, &row.attempts, &row.maxAttempts, &row.createdAt, &row.payload)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		claim = row
		return nil
	})
	if err == nil && claim != nil {
		observeDeliveryClaim("inbox", claim.createdAt)
	}
	return claim, err
}

func (d *Dao) publishInboxDelivery(ctx context.Context, key string, request *inbox.TLInboxSendUserMessageToInboxV2) error {
	return d.publishMessagePublication(ctx, "inbox", key, func() error {
		result, err := d.InboxClient.InboxSendUserMessageToInboxV2(ctx, request)
		if err == nil && result == nil {
			return errors.New("messenger/msg: inbox publication returned no acknowledgement")
		}
		return err
	})
}

func (d *Dao) publishMessagePublication(ctx context.Context, queue, key string, send func() error) error {
	publicationKey := deliveryPublicationKey{owner: d, key: queue + ":" + key}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		publication := &inboxDeliveryPublication{done: make(chan struct{})}
		active, loaded := inboxDeliveryPublications.LoadOrStore(publicationKey, publication)
		if loaded {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-active.(*inboxDeliveryPublication).done:
				continue
			}
		}
		go func() {
			deliveryPublicationInFlight.WithLabelValues(queue).Inc()
			started := time.Now()
			publication.err = send()
			observeDeliveryPublicationDuration(queue, started)
			deliveryPublicationInFlight.WithLabelValues(queue).Dec()
			inboxDeliveryPublications.CompareAndDelete(publicationKey, publication)
			close(publication.done)
		}()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-publication.done:
			return publication.err
		}
	}
}

func (d *Dao) releaseInboxDelivery(ctx context.Context, claim *inboxDeliveryClaim, cause error) error {
	delay := 30 * time.Second
	detail := ""
	if cause != nil {
		delay = time.Second << min(max(claim.attempts-1, 0), 9)
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
		detail = cause.Error()
		if len(detail) > 1024 {
			detail = detail[:1024]
		}
		detail = strings.ToValidUTF8(detail, "")
	}
	maxAttempts := claim.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	deadLetter := cause != nil && claim.attempts >= maxAttempts
	updated := false
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var result pgconn.CommandTag
		var err error
		if deadLetter {
			result, err = tx.Exec(ctx, `UPDATE msg_inbox_delivery_outbox
				SET state=2, dead_lettered_at=now(), terminal_reason=$3, last_error=$3,
					lease_until=NULL, claim_token=NULL, available_at=now()
				WHERE id=$1 AND claim_token=$2::uuid AND state=0`, claim.id, claim.token, detail)
		} else {
			result, err = tx.Exec(ctx, `UPDATE msg_inbox_delivery_outbox
				SET lease_until=NULL, claim_token=NULL, available_at=now()+($3::double precision * interval '1 second'), last_error=$4
				WHERE id=$1 AND claim_token=$2::uuid AND state=0`, claim.id, claim.token, delay.Seconds(), detail)
		}
		if err != nil {
			return err
		}
		updated = result.RowsAffected() == 1
		return nil
	})
	if err == nil && updated {
		if deadLetter {
			deliveryDeadLetters.WithLabelValues("inbox").Inc()
		} else if cause != nil {
			deliveryRetries.WithLabelValues("inbox").Inc()
		}
	}
	return err
}

// DispatchInboxDeliveries publishes due intents while retaining them until
// acknowledgement. Claims are taken concurrently, while the claim query keeps
// later rows for the same recipient behind the earlier row.
func (d *Dao) dispatchInboxDelivery(ctx context.Context, claim *inboxDeliveryClaim) (int, error) {
	request := new(inbox.TLInboxSendUserMessageToInboxV2)
	publishErr := json.Unmarshal(claim.payload, request)
	if publishErr == nil {
		publishCtx, cancel := context.WithTimeout(ctx, inboxDeliveryPublishTimeout)
		publishErr = d.publishInboxDelivery(publishCtx, fmt.Sprintf("%d", claim.id), request)
		cancel()
	}
	observeDeliveryPublication("inbox", publishErr)
	if errors.Is(publishErr, context.Canceled) || errors.Is(publishErr, context.DeadlineExceeded) {
		return 0, publishErr
	}
	// A completed call releases its lease even if the parent expires before
	// this follow-up write; ambiguous cancellations retain the lease above.
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	releaseErr := d.releaseInboxDelivery(releaseCtx, claim, publishErr)
	cancel()
	if publishErr == nil {
		if releaseErr != nil {
			return 1, releaseErr
		}
		if claim.attempts > 1 {
			deliveryRecovery.WithLabelValues("inbox").Observe(max(time.Since(claim.createdAt).Seconds(), 0))
		}
		return 1, nil
	}
	if releaseErr != nil {
		return 0, errors.Join(fmt.Errorf("inbox delivery %d: %w", claim.id, publishErr), releaseErr)
	}
	return 0, fmt.Errorf("inbox delivery %d: %w", claim.id, publishErr)
}

func (d *Dao) DispatchInboxDeliveries(ctx context.Context, limit int) (int, error) {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil {
		return 0, errors.New("messenger/msg: inbox delivery store is not configured")
	}
	if d.InboxClient == nil {
		return 0, errors.New("messenger/msg: inbox delivery client is not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return runDeliveryWorkers(ctx, limit, d.deliveryConcurrency(), d.claimInboxDelivery, d.dispatchInboxDelivery)
}

// StartInboxDeliveryWorker returns a stop function that waits for its current
// dispatch to finish before the owning service closes the PostgreSQL pool.
func (d *Dao) StartInboxDeliveryWorker(ctx context.Context) func() {
	workerCtx, cancel := context.WithCancel(ctx)
	waitMetrics := d.startDeliveryMetrics(workerCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		var lastProgress time.Time
		for {
			if workerCtx.Err() != nil {
				return
			}
			inboxPublished, err := d.DispatchInboxDeliveries(workerCtx, 50)
			if err != nil && workerCtx.Err() == nil {
				logx.WithContext(workerCtx).Errorf("inbox delivery worker: %v", err)
			}
			statePublished, err := d.DispatchMessageStateDeliveries(workerCtx, 50)
			if err != nil && workerCtx.Err() == nil {
				logx.WithContext(workerCtx).Errorf("message state delivery worker: %v", err)
			}
			if inboxPublished+statePublished > 0 {
				lastProgress = time.Now()
			}
			delay := time.Second
			// Ordered inbox rows wait for the asynchronous consumer ACK. Keep
			// checking briefly after progress so each row does not add 1s of lag.
			if time.Since(lastProgress) < time.Second {
				delay = 25 * time.Millisecond
			}
			timer.Reset(delay)
			select {
			case <-workerCtx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	var stop sync.Once
	return func() {
		stop.Do(cancel)
		<-done
		waitMetrics()
	}
}
