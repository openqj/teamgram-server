package dao

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/botapi/store"
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
	BotAPI *store.Store
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
		`SELECT consumer_group,topic,partition_id,message_offset,user_id,request_hash,delivery_data,delivered,delivered_at FROM sync_delivery_receipts LIMIT 0`,
		`SELECT user_id,state,pending_name,pending_bot_id,pending_menu_text,pending_menu_url,pending_app_title,pending_app_short_name,pending_app_description,pending_app_url,last_message_id,last_reply,updated_at FROM botfather_sessions LIMIT 0`,
		`SELECT bot_id,next_update_id FROM bot_api_update_counters LIMIT 0`,
		`SELECT bot_id,update_id,update_data,created_at FROM bot_api_updates LIMIT 0`,
		`SELECT bot_id,dedupe_key,first_update_id,created_at FROM bot_api_update_dedup LIMIT 0`,
		`SELECT bot_id,url,max_connections,allowed_updates,secret_token,last_error_date,last_error_message,last_synchronization_error_date FROM bot_api_webhooks LIMIT 0`); err != nil {
		// The Bot API menu button is shared by BotFather and HTTP Bot API.
		// Verify its scope table at startup so a partial migration fails closed.
		pool.Close()
		return nil, err
	}
	if err := postgres.VerifySchema(context.Background(), pool,
		`SELECT bot_id,chat_id,button_type,button_text,button_url FROM bot_api_chat_menu_buttons LIMIT 0`,
		`SELECT id,bot_id,short_name,title,description,url,domain,created_at,updated_at FROM bot_mini_apps LIMIT 0`); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{
		Pool:              pool,
		AuthSeqUpdatesDAO: postgres_dao.NewAuthSeqUpdatesDAO(pool),
		UserPtsUpdatesDAO: postgres_dao.NewUserPtsUpdatesDAO(pool),
		BotAPI:            store.NewWithPool(pool),
	}, nil
}

func (d *Postgres) Close() {
	if d != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
