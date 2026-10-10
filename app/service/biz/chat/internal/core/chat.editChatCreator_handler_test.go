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

func TestChatEditChatCreatorPostgresAtomicRoundTrip(t *testing.T) {
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
	owner, target := time.Now().UnixNano(), time.Now().UnixNano()+1
	other := target + 1
	chatCore := &ChatCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	created, err := chatCore.ChatCreateChat2(&chat.TLChatCreateChat2{
		CreatorId:  owner,
		UserIdList: []int64{target, other},
		Title:      "creator transfer test",
	})
	if err != nil || created == nil || created.Chat == nil {
		t.Fatalf("create fixture chat = (%v, %v)", created, err)
	}
	chatID := created.Chat.Id
	t.Cleanup(func() { _, _ = pg.Pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, chatID) })
	if _, err = pg.Pool.Exec(ctx, `UPDATE chat_participants SET participant_type = $1, admin_rights = $2 WHERE chat_id = $3 AND user_id = $4`, mtproto.ChatMemberAdmin, 123, chatID, target); err != nil {
		t.Fatalf("prepare admin target: %v", err)
	}

	updated, err := chatCore.ChatEditChatAdmin(&chat.TLChatEditChatAdmin{
		ChatId: chatID, OperatorId: owner, EditChatAdminId: target,
	})
	if err != nil || updated == nil || updated.Creator() != target {
		t.Fatalf("transfer = (%v, %v), want target creator", updated, err)
	}
	var creator int64
	if err := pg.Pool.QueryRow(ctx, `SELECT creator_user_id FROM chats WHERE id = $1`, chatID).Scan(&creator); err != nil {
		t.Fatalf("read creator: %v", err)
	}
	if creator != target {
		t.Fatalf("creator_user_id = %d, want %d", creator, target)
	}
	rows, err := pg.Pool.Query(ctx, `SELECT user_id, participant_type, admin_rights FROM chat_participants WHERE chat_id = $1 ORDER BY user_id`, chatID)
	if err != nil {
		t.Fatalf("read participants: %v", err)
	}
	defer rows.Close()
	got := map[int64]int32{}
	for rows.Next() {
		var userID int64
		var participantType, adminRights int32
		if err := rows.Scan(&userID, &participantType, &adminRights); err != nil {
			t.Fatalf("scan participant: %v", err)
		}
		got[userID] = participantType
		if adminRights != 0 {
			t.Fatalf("participant %d retained admin_rights=%d after creator transfer", userID, adminRights)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("participants rows: %v", err)
	}
	if got[owner] != mtproto.ChatMemberNormal || got[target] != mtproto.ChatMemberCreator || got[other] != mtproto.ChatMemberNormal {
		t.Fatalf("participant types = %#v, want owner normal, target creator, and other normal", got)
	}

	if _, err = chatCore.ChatEditChatAdmin(&chat.TLChatEditChatAdmin{
		ChatId: chatID, OperatorId: owner, EditChatAdminId: target,
	}); err == nil {
		t.Fatal("old creator unexpectedly transferred ownership a second time")
	}
}
