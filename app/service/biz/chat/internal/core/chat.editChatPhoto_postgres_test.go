package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func TestChatEditChatPhotoPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("CHAT_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("CHAT_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}

	pg, err := dao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	t.Cleanup(pg.Close)

	ctx := context.Background()
	creatorID := time.Now().UnixNano()
	memberID := creatorID + 1
	chatCore := &ChatCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	created, err := chatCore.ChatCreateChat2(&chat.TLChatCreateChat2{
		CreatorId:  creatorID,
		UserIdList: []int64{memberID},
		Title:      "photo round trip",
	})
	if err != nil || created == nil || created.Chat == nil {
		t.Fatalf("create chat = (%v, %v)", created, err)
	}
	chatID := created.Chat.Id
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chat_invites WHERE chat_id = $1`, chatID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chat_participants WHERE chat_id = $1`, chatID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, chatID)
	})

	updated, err := chatCore.ChatEditChatPhoto(&chat.TLChatEditChatPhoto{
		ChatId:     chatID,
		EditUserId: creatorID,
		ChatPhoto:  mtproto.MakeTLPhoto(&mtproto.Photo{Id: 88001, AccessHash: 99001}).To_Photo(),
	})
	if err != nil || updated == nil {
		t.Fatalf("edit chat photo = (%v, %v)", updated, err)
	}
	var photoID int64
	var version int32
	if err := pg.Pool.QueryRow(ctx, `SELECT photo_id, version FROM chats WHERE id = $1`, chatID).Scan(&photoID, &version); err != nil {
		t.Fatalf("read chat photo: %v", err)
	}
	if photoID != 88001 || version < 2 {
		t.Fatalf("stored photo/version = (%d, %d), want photo 88001 and version >= 2", photoID, version)
	}

	memberCore := &ChatCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	if _, err := memberCore.ChatEditChatPhoto(&chat.TLChatEditChatPhoto{
		ChatId:     chatID,
		EditUserId: memberID,
		ChatPhoto:  mtproto.MakeTLPhoto(&mtproto.Photo{Id: 88002, AccessHash: 99002}).To_Photo(),
	}); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("non-admin edit error = %v, want CHAT_ADMIN_REQUIRED", err)
	}
}
