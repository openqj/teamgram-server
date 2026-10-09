package persist

import (
	"context"
	"errors"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/encoding/protojson"
)

// SaveAppLogEvents stores client telemetry as an append-only batch. App logs
// are deliberately kept outside the generic KV document so that retention and
// indexing can be managed independently from caller state.
func SaveAppLogEvents(ctx context.Context, userID int64, events []*mtproto.InputAppEvent) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errors.New("apifull: PostgreSQL app-log store is unavailable")
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const query = `INSERT INTO apifull_app_log
		(user_id, event_time, event_type, peer_id, data)
		VALUES ($1, $2, $3, $4, $5::jsonb)`
	for _, event := range events {
		if event == nil || event.GetType() == "" {
			continue
		}
		data := []byte(`{}`)
		if event.GetData() != nil {
			data, err = protojson.Marshal(event.GetData())
			if err != nil {
				return fmt.Errorf("encode app-log data: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, query, userID, event.GetTime(), event.GetType(), event.GetPeer(), data); err != nil {
			return err
		}
	}
	return tx.Commit()
}
