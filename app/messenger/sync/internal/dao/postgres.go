package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

// Postgres contains the durable update queues used by sync. The pool is kept
// on the DAO so a future message/update transaction can share one connection.
type Postgres struct {
	Pool *pgxpool.Pool
	*postgres_dao.AuthSeqUpdatesDAO
	*postgres_dao.UserPtsUpdatesDAO
}

func newPostgresDao(c config.Config) (*Postgres, error) {
	pool, err := postgres.NewPool(context.Background(), c.Postgres)
	if err != nil {
		return nil, err
	}
	if err := postgres.VerifySchema(context.Background(), pool,
		`SELECT id,auth_id,user_id,seq,update_type,update_data,date2 FROM auth_seq_updates LIMIT 0`,
		`SELECT id,user_id,pts,pts_count,update_type,update_data,date2 FROM user_pts_updates LIMIT 0`,
		// PrepareUpdates locks the permanent authorization rows before it
		// journals a delivery. Verify every table used by that transaction at
		// startup so a partially migrated database cannot accept Kafka work.
		`SELECT auth_key_id,auth_key_type,deleted FROM auth_key_infos LIMIT 0`,
		`SELECT auth_key_id,user_id,deleted FROM auth_users LIMIT 0`,
		`SELECT auth_key_id,deleted FROM auth_keys LIMIT 0`,
		`SELECT key,value,updated_at FROM idgen_counters LIMIT 0`,
		`SELECT consumer_group,topic,partition_id,message_offset,user_id,request_hash,delivery_data,delivered,delivered_at FROM sync_delivery_receipts LIMIT 0`); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{
		Pool:              pool,
		AuthSeqUpdatesDAO: postgres_dao.NewAuthSeqUpdatesDAO(pool),
		UserPtsUpdatesDAO: postgres_dao.NewUserPtsUpdatesDAO(pool),
	}, nil
}

func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
