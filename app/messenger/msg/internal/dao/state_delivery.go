package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"google.golang.org/protobuf/proto"
)

func (d *Dao) EnqueueMessageStateDeliveryOn(ctx context.Context, tx pgx.Tx, userID int64, request proto.Message) error {
	if tx == nil || userID <= 0 || request == nil {
		return errors.New("messenger/msg: state delivery requires a transaction and request")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO msg_state_delivery_outbox(user_id,method,payload) VALUES ($1,$2,$3::jsonb)`, userID, string(proto.MessageName(request)), payload)
	return err
}

func (d *Dao) publishMessageState(ctx context.Context, method string, payload []byte) error {
	if method != string(proto.MessageName((*syncpb.TLSyncPushUpdates)(nil))) && d.InboxClient == nil {
		return errors.New("messenger/msg: Inbox client is not configured")
	}
	switch method {
	case string(proto.MessageName((*syncpb.TLSyncPushUpdates)(nil))):
		if d.SyncClient == nil {
			return errors.New("messenger/msg: Sync client is not configured")
		}
		return publishStateRequest(ctx, payload, new(syncpb.TLSyncPushUpdates), d.SyncClient.SyncPushUpdates)
	case string(proto.MessageName((*inbox.TLInboxDeleteMessagesToInbox)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxDeleteMessagesToInbox), d.InboxClient.InboxDeleteMessagesToInbox)
	case string(proto.MessageName((*inbox.TLInboxEditMessageToInboxV2)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxEditMessageToInboxV2), d.InboxClient.InboxEditMessageToInboxV2)
	case string(proto.MessageName((*inbox.TLInboxReadOutboxHistory)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxReadOutboxHistory), d.InboxClient.InboxReadOutboxHistory)
	case string(proto.MessageName((*inbox.TLInboxReadMediaUnreadToInboxV2)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxReadMediaUnreadToInboxV2), d.InboxClient.InboxReadMediaUnreadToInboxV2)
	case string(proto.MessageName((*inbox.TLInboxUpdatePinnedMessage)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxUpdatePinnedMessage), d.InboxClient.InboxUpdatePinnedMessage)
	case string(proto.MessageName((*inbox.TLInboxUnpinAllMessages)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxUnpinAllMessages), d.InboxClient.InboxUnpinAllMessages)
	case string(proto.MessageName((*inbox.TLInboxDeleteUserHistoryToInbox)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxDeleteUserHistoryToInbox), d.InboxClient.InboxDeleteUserHistoryToInbox)
	case string(proto.MessageName((*inbox.TLInboxDeleteChatHistoryToInbox)(nil))):
		return publishStateRequest(ctx, payload, new(inbox.TLInboxDeleteChatHistoryToInbox), d.InboxClient.InboxDeleteChatHistoryToInbox)
	default:
		return fmt.Errorf("messenger/msg: unknown state delivery method %q", method)
	}
}

func publishStateRequest[T proto.Message](ctx context.Context, data []byte, request T, send func(context.Context, T) (*mtproto.Void, error)) error {
	if err := json.Unmarshal(data, request); err != nil {
		return err
	}
	result, err := send(ctx, request)
	if err == nil && result == nil {
		return errors.New("messenger/msg: state publication returned no acknowledgement")
	}
	return err
}

func (d *Dao) DispatchMessageStateDeliveries(ctx context.Context, limit int) (int, error) {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil {
		return 0, errors.New("messenger/msg: PostgreSQL state delivery is not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	published := 0
	for i := 0; i < limit; i++ {
		var id int64
		var token, method string
		var payload []byte
		var attempts int
		err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `WITH due AS (
 SELECT o.id FROM msg_state_delivery_outbox o
 WHERE o.state=0 AND o.available_at<=now() AND (o.lease_until IS NULL OR o.lease_until<=now())
 AND NOT EXISTS (SELECT 1 FROM msg_state_delivery_outbox prior WHERE prior.user_id=o.user_id AND prior.state=0 AND prior.id<o.id)
 AND NOT EXISTS (SELECT 1 FROM msg_inbox_delivery_outbox pending WHERE pending.sender_user_id=o.user_id AND pending.state=0 AND pending.created_at<=o.created_at)
 ORDER BY o.id LIMIT 1 FOR UPDATE OF o SKIP LOCKED)
 UPDATE msg_state_delivery_outbox o SET claim_token=gen_random_uuid(),lease_until=now()+interval '60 seconds',attempts=o.attempts+1
 FROM due WHERE o.id=due.id RETURNING o.id,o.claim_token::text,o.method,o.payload,o.attempts`).Scan(&id, &token, &method, &payload, &attempts)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return published, nil
		}
		if err != nil {
			return published, err
		}
		publishCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = d.publishMessagePublication(publishCtx, func() error { return d.publishMessageState(publishCtx, method, payload) })
		cancel()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return published, err
		}
		if ctx.Err() != nil {
			return published, ctx.Err()
		}
		delay := time.Second << min(attempts-1, 8)
		if err != nil {
			_, releaseErr := d.Postgres.Pool.Exec(ctx, `UPDATE msg_state_delivery_outbox SET lease_until=NULL,claim_token=NULL,available_at=now()+($3::double precision*interval '1 second'),last_error=$4
 WHERE id=$1 AND claim_token=$2::uuid AND state=0`, id, token, delay.Seconds(), "publication failed")
			return published, errors.Join(err, releaseErr)
		}
		tag, err := d.Postgres.Pool.Exec(ctx, `UPDATE msg_state_delivery_outbox SET state=1,delivered_at=now(),payload='{}'::jsonb,lease_until=NULL,claim_token=NULL,last_error=''
 WHERE id=$1 AND claim_token=$2::uuid AND state=0`, id, token)
		if err != nil {
			return published, err
		}
		if tag.RowsAffected() != 1 {
			return published, errors.New("messenger/msg: state delivery lease was lost")
		}
		published++
	}
	return published, nil
}
