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
	"github.com/zeromicro/go-zero/core/logx"
)

func TestChatEditChatParticipantRankPostgresRoundTrip(t *testing.T) {
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
	chatCore := &ChatCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}, Logger: logx.WithContext(ctx)}
	created, err := chatCore.ChatCreateChat2(&chat.TLChatCreateChat2{
		CreatorId:  creatorID,
		UserIdList: []int64{memberID},
		Title:      "rank round trip",
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

	updated, err := chatCore.ChatEditChatParticipantRank(&chat.TLChatEditChatParticipantRank{
		SelfId:      creatorID,
		ChatId:      chatID,
		Participant: memberID,
		Rank:        "moderator",
	})
	if err != nil || updated == nil {
		t.Fatalf("edit rank = (%v, %v)", updated, err)
	}
	var rank string
	var version int32
	if err := pg.Pool.QueryRow(ctx, `SELECT rank2, version FROM chat_participants p JOIN chats c ON c.id = p.chat_id WHERE p.chat_id = $1 AND p.user_id = $2`, chatID, memberID).Scan(&rank, &version); err != nil {
		t.Fatalf("read rank: %v", err)
	}
	if rank != "moderator" || version < 2 {
		t.Fatalf("stored rank/version = (%q, %d), want moderator and version >= 2", rank, version)
	}

	if _, err := chatCore.ChatEditChatParticipantRank(&chat.TLChatEditChatParticipantRank{
		SelfId:      memberID,
		ChatId:      chatID,
		Participant: creatorID,
		Rank:        "owner",
	}); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("non-admin rank edit error = %v, want CHAT_ADMIN_REQUIRED", err)
	}
}
