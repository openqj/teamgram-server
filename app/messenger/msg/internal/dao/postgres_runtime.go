package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	chatpg "github.com/teamgram/teamgram-server/app/service/biz/chat"
	dialogpg "github.com/teamgram/teamgram-server/app/service/biz/dialog"
	messagepg "github.com/teamgram/teamgram-server/app/service/biz/message"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres is the message service's PostgreSQL runtime boundary. Keeping the
// pool and aggregate stores together makes it possible for an inbox delivery
// to persist a message, its hash tags, and pts state on one pgx transaction.
//
// The generated handlers still use the legacy Mysql field on Dao until their
// transaction callbacks are converted. New code must use this boundary and
// must not add methods to the MySQL DAO.
type Postgres struct {
	Pool  *pgxpool.Pool
	Store *postgres_dao.Store
}

// NewPostgres opens and health-checks the service-owned PostgreSQL pool.
func NewPostgres(cfg postgres.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	if err := chatpg.VerifyPostgresSchema(context.Background(), pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := dialogpg.VerifyPostgresSchema(context.Background(), pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := messagepg.VerifyPostgresSchema(context.Background(), pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := postgres.VerifySchema(context.Background(), pool,
		`SELECT id,user_id,pts,pts_count,update_type,update_data,date2 FROM user_pts_updates LIMIT 0`,
		`SELECT key,value,updated_at FROM idgen_counters LIMIT 0`,
		`SELECT id,sender_user_id,sender_message_id,dialog_message_id,peer_type,peer_id,recipient_user_id,payload,state,available_at,lease_until,claim_token,attempts,last_error,delivered_at FROM msg_inbox_delivery_outbox LIMIT 0`,
		`SELECT id,user_id,method,payload,state,available_at,lease_until,claim_token,attempts,last_error,delivered_at,created_at FROM msg_state_delivery_outbox LIMIT 0`,
		`SELECT consumer_group,topic,partition,message_offset,user_id,operation FROM msg_inbox_consumer_receipts LIMIT 0`,
		`SELECT id,deleted FROM users LIMIT 0`,
		`SELECT user_id,peer_type,peer_id,deleted FROM user_peer_blocks LIMIT 0`); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{Pool: pool, Store: postgres_dao.NewStore(pool)}, nil
}

// InTx executes fn in a database transaction. Message persistence code should
// use the transaction passed to the DAO *On methods so delivery and update
// state remain atomic.
func (d *Postgres) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if d == nil || d.Pool == nil {
		return errors.New("messenger/msg: postgres store is not configured")
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Close releases the pool during graceful service shutdown.
func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
