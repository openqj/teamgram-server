package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestChatCreateChat2PostgresRoundTrip(t *testing.T) {
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
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chat_invites WHERE admin_id = $1`, userID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chats WHERE creator_user_id = $1`, userID)
	})

	core := &ChatCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	result, err := core.ChatCreateChat2(&chat.TLChatCreateChat2{
		CreatorId:  userID,
		UserIdList: []int64{userID + 1, userID + 2},
		Title:      "PostgreSQL test chat",
		Bots:       []int64{userID + 3},
	})
	if err != nil {
		t.Fatalf("create chat = (%v, %v)", result, err)
	}
	if result == nil || result.Chat == nil || result.Chat.Id == 0 || len(result.ChatParticipants) != 4 {
		t.Fatalf("create chat result = %+v, want chat with four participants", result)
	}

	var chatCount, participantCount, permanentInviteCount int
	if err := pg.Pool.QueryRow(ctx, `SELECT count(*) FROM chats WHERE id = $1 AND creator_user_id = $2`, result.Chat.Id, userID).Scan(&chatCount); err != nil {
		t.Fatalf("read created chat: %v", err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT count(*) FROM chat_participants WHERE chat_id = $1`, result.Chat.Id).Scan(&participantCount); err != nil {
		t.Fatalf("read created participants: %v", err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT count(*) FROM chat_invites WHERE chat_id = $1 AND admin_id = $2 AND permanent = TRUE`, result.Chat.Id, userID).Scan(&permanentInviteCount); err != nil {
		t.Fatalf("read permanent invite: %v", err)
	}
	if chatCount != 1 || participantCount != 4 || permanentInviteCount != 1 {
		t.Fatalf("stored chat state = chat:%d participants:%d invites:%d, want 1:4:1", chatCount, participantCount, permanentInviteCount)
	}
}

func TestChatInvitePostgresRoundTrip(t *testing.T) {
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
	userID := time.Now().UnixNano()
	core := &ChatCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}, MD: &metadata.RpcMetadata{UserId: userID}}
	created, err := core.ChatCreateChat2(&chat.TLChatCreateChat2{CreatorId: userID, Title: "Invite test"})
	if err != nil || created == nil || created.Chat == nil {
		t.Fatalf("create invite fixture chat = (%v, %v)", created, err)
	}
	chatID := created.Chat.Id
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chat_invites WHERE chat_id = $1`, chatID)
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, chatID)
	})

	exported, err := core.ChatExportChatInvite(&chat.TLChatExportChatInvite{ChatId: chatID, AdminId: userID, Title: &wrapperspb.StringValue{Value: "initial"}})
	if err != nil || exported == nil || exported.Link == "" {
		t.Fatalf("export invite = (%v, %v)", exported, err)
	}
	checked, err := core.ChatCheckChatInvite(&chat.TLChatCheckChatInvite{SelfId: userID, Hash: chat.GetInviteHashByLink(exported.Link)})
	if err != nil || checked == nil {
		t.Fatalf("check invite = (%v, %v)", checked, err)
	}
	otherID := userID + 1
	core.MD.UserId = otherID
	imported, err := core.ChatImportChatInvite2(&chat.TLChatImportChatInvite2{SelfId: otherID, Hash: chat.GetInviteHashByLink(exported.Link)})
	if err != nil || imported == nil || imported.Chat == nil {
		t.Fatalf("import invite = (%v, %v)", imported, err)
	}
	core.MD.UserId = userID
	edited, err := core.ChatEditExportedChatInvite(&chat.TLChatEditExportedChatInvite{
		SelfId: userID, ChatId: chatID, Link: exported.Link,
		UsageLimit: &wrapperspb.Int32Value{Value: 3}, Title: &wrapperspb.StringValue{Value: "edited"},
	})
	if err != nil || edited == nil || len(edited.Datas) != 1 || edited.Datas[0].Title == nil || edited.Datas[0].Title.Value != "edited" {
		t.Fatalf("edit invite = (%v, %v)", edited, err)
	}
	hash := chat.GetInviteHashByLink(exported.Link)
	var title string
	var usage int32
	if err := pg.Pool.QueryRow(ctx, `SELECT title, usage_limit FROM chat_invites WHERE chat_id = $1 AND link = $2`, chatID, hash).Scan(&title, &usage); err != nil {
		t.Fatalf("read edited invite: %v", err)
	}
	if title != "edited" || usage != 3 {
		t.Fatalf("edited invite state = (%q, %d), want edited/3", title, usage)
	}
	if result, err := core.ChatDeleteExportedChatInvite(&chat.TLChatDeleteExportedChatInvite{SelfId: userID, ChatId: chatID, Link: exported.Link}); err != nil || result == nil {
		t.Fatalf("delete invite = (%v, %v)", result, err)
	}
	var inviteCount int
	if err := pg.Pool.QueryRow(ctx, `SELECT count(*) FROM chat_invites WHERE chat_id = $1 AND link = $2`, chatID, hash).Scan(&inviteCount); err != nil {
		t.Fatalf("read deleted invite: %v", err)
	}
	if inviteCount != 0 {
		t.Fatalf("deleted invite count = %d, want 0", inviteCount)
	}
}
