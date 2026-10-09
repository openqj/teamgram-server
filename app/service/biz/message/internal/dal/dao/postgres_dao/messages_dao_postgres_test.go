package postgres_dao

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
)

func TestMessagesRandomIDUniquenessIsScopedToUser(t *testing.T) {
	dsn := os.Getenv("MESSENGER_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MESSENGER_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	base := time.Now().UnixNano()
	senderID := base
	userIDs := []int64{base + 1, base + 2}
	randomID := base + 3
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM messages WHERE user_id = ANY($1::bigint[]) AND sender_user_id = $2 AND random_id = $3`, userIDs, senderID, randomID)
	}()

	dao := NewMessagesDAO(pool)
	insert := func(userID int64, messageBoxID int32) (int64, int64, error) {
		return dao.InsertOrReturnId(ctx, &dataobject.MessagesDO{
			UserId: userID, UserMessageBoxId: messageBoxID, SenderUserId: senderID,
			PeerType: 1, PeerId: senderID + 10, RandomId: randomID,
			MessageData: `{"message":"same request"}`, Message: "same request",
		})
	}

	firstID, firstAffected, err := insert(userIDs[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == 0 || firstAffected != 1 {
		t.Fatalf("first insert = (%d, %d), want a new row", firstID, firstAffected)
	}

	secondID, secondAffected, err := insert(userIDs[1], 1)
	if err != nil {
		t.Fatal(err)
	}
	if secondID == 0 || secondAffected != 1 || secondID == firstID {
		t.Fatalf("different recipient insert = (%d, %d), want a distinct new row", secondID, secondAffected)
	}

	duplicateID, duplicateAffected, err := insert(userIDs[0], 2)
	if err != nil {
		t.Fatal(err)
	}
	if duplicateID != firstID || duplicateAffected != 0 {
		t.Fatalf("duplicate insert = (%d, %d), want existing id %d and no insert", duplicateID, duplicateAffected, firstID)
	}

	existing, err := dao.SelectByRandomIdForUser(ctx, pool, userIDs[0], senderID, randomID)
	if err != nil {
		t.Fatal(err)
	}
	if existing == nil || existing.UserMessageBoxId != 1 {
		t.Fatalf("stored duplicate row = %#v, want original message box id 1", existing)
	}
	if _, err := pool.Exec(ctx, `UPDATE messages SET deleted = TRUE WHERE id = $1`, firstID); err != nil {
		t.Fatal(err)
	}
	deletedRetryID, deletedRetryAffected, err := insert(userIDs[0], 3)
	if err != nil {
		t.Fatal(err)
	}
	if deletedRetryID != firstID || deletedRetryAffected != 0 {
		t.Fatalf("deleted message retry = (%d, %d), want existing id %d", deletedRetryID, deletedRetryAffected, firstID)
	}
	if visible, err := dao.SelectByMessageId(ctx, userIDs[0], 1); err != nil || visible != nil {
		t.Fatalf("deleted message became visible: %#v, %v", visible, err)
	}
}
