package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/zeromicro/go-zero/core/logx"
)

const inboxDeliveryPublishTimeout = 30 * time.Second

type inboxDeliveryClaim struct {
	id       int64
	token    string
	attempts int
	payload  []byte
}

type inboxDeliveryPublication struct {
	done chan struct{}
	err  error
}

// Entries exist only while a producer call is active. The MQ adapter may
// ignore context cancellation, so retries must share that one in-flight call.
var inboxDeliveryPublications sync.Map

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
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE msg_inbox_delivery_outbox
			SET state=1, delivered_at=now(), payload='{}'::jsonb, lease_until=NULL, claim_token=NULL, last_error=''
			WHERE sender_user_id=$1 AND dialog_message_id=$2 AND recipient_user_id=$3 AND state=0`,
			senderID, dialogID, recipientID)
		return err
	})
}

func (d *Dao) claimInboxDelivery(ctx context.Context) (*inboxDeliveryClaim, error) {
	var claim *inboxDeliveryClaim
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		row := new(inboxDeliveryClaim)
		err := tx.QueryRow(ctx, `WITH due AS (
			SELECT o.id FROM msg_inbox_delivery_outbox o
			WHERE o.state=0 AND o.available_at<=now() AND (o.lease_until IS NULL OR o.lease_until<=now())
			AND NOT EXISTS (
				SELECT 1 FROM msg_inbox_delivery_outbox prior
				WHERE prior.recipient_user_id=o.recipient_user_id AND prior.state=0 AND prior.id<o.id)
			ORDER BY o.id LIMIT 1 FOR UPDATE OF o SKIP LOCKED
		)
		UPDATE msg_inbox_delivery_outbox o
		SET claim_token=gen_random_uuid(), lease_until=now()+interval '60 seconds', attempts=o.attempts+1
		FROM due WHERE o.id=due.id
		RETURNING o.id, o.claim_token::text, o.attempts, o.payload`,
		).Scan(&row.id, &row.token, &row.attempts, &row.payload)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		claim = row
		return nil
	})
	return claim, err
}

func (d *Dao) publishInboxDelivery(ctx context.Context, request *inbox.TLInboxSendUserMessageToInboxV2) error {
	return d.publishMessagePublication(ctx, func() error {
		result, err := d.InboxClient.InboxSendUserMessageToInboxV2(ctx, request)
		if err == nil && result == nil {
			return errors.New("messenger/msg: inbox publication returned no acknowledgement")
		}
		return err
	})
}

func (d *Dao) publishMessagePublication(ctx context.Context, send func() error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		publication := &inboxDeliveryPublication{done: make(chan struct{})}
		active, loaded := inboxDeliveryPublications.LoadOrStore(d, publication)
		if loaded {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-active.(*inboxDeliveryPublication).done:
				continue
			}
		}
		go func() {
			publication.err = send()
			inboxDeliveryPublications.CompareAndDelete(d, publication)
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
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE msg_inbox_delivery_outbox
			SET lease_until=NULL, claim_token=NULL, available_at=now()+($3::double precision * interval '1 second'), last_error=$4
			WHERE id=$1 AND claim_token=$2::uuid AND state=0`,
			claim.id, claim.token, delay.Seconds(), detail)
		return err
	})
}

// DispatchInboxDeliveries publishes due intents while retaining them until
// acknowledgement. Claims are taken one at a time so later rows do not spend
// their lease waiting for an earlier publication.
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
	published := 0
	var dispatchErr error
	for i := 0; i < limit; i++ {
		if err := ctx.Err(); err != nil {
			return published, errors.Join(dispatchErr, err)
		}
		claim, err := d.claimInboxDelivery(ctx)
		if err != nil || claim == nil {
			return published, errors.Join(dispatchErr, err)
		}
		request := new(inbox.TLInboxSendUserMessageToInboxV2)
		publishErr := json.Unmarshal(claim.payload, request)
		if publishErr == nil {
			publishCtx, cancel := context.WithTimeout(ctx, inboxDeliveryPublishTimeout)
			publishErr = d.publishInboxDelivery(publishCtx, request)
			cancel()
		}
		if errors.Is(publishErr, context.Canceled) || errors.Is(publishErr, context.DeadlineExceeded) {
			return published, errors.Join(dispatchErr, publishErr)
		}
		// A completed call releases its lease even if the parent expires before
		// this follow-up write; ambiguous cancellations retain the lease above.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		releaseErr := d.releaseInboxDelivery(releaseCtx, claim, publishErr)
		cancel()
		if publishErr == nil {
			published++
		} else {
			dispatchErr = errors.Join(dispatchErr, fmt.Errorf("inbox delivery %d: %w", claim.id, publishErr))
		}
		if releaseErr != nil {
			return published, errors.Join(dispatchErr, releaseErr)
		}
	}
	return published, dispatchErr
}

// StartInboxDeliveryWorker returns a stop function that waits for its current
// dispatch to finish before the owning service closes the PostgreSQL pool.
func (d *Dao) StartInboxDeliveryWorker(ctx context.Context) func() {
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if workerCtx.Err() != nil {
				return
			}
			if _, err := d.DispatchInboxDeliveries(workerCtx, 50); err != nil && workerCtx.Err() == nil {
				logx.WithContext(workerCtx).Errorf("inbox delivery worker: %v", err)
			}
			if _, err := d.DispatchMessageStateDeliveries(workerCtx, 50); err != nil && workerCtx.Err() == nil {
				logx.WithContext(workerCtx).Errorf("message state delivery worker: %v", err)
			}
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	var stop sync.Once
	return func() {
		stop.Do(cancel)
		<-done
	}
}
