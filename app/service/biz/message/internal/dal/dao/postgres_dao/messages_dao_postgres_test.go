package postgres_dao

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
)

func TestSelectByMessageDataIdUserIdListEmptyIsEmpty(t *testing.T) {
	rows, err := NewMessagesDAO(nil).SelectByMessageDataIdUserIdList(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows == nil || len(rows) != 0 {
		t.Fatalf("empty user vector returned %v, want an empty result", rows)
	}
}

func TestCountByMediaTypeIncludesPhotoVideoVariants(t *testing.T) {
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
	userID := base
	dialogID1 := base + 1
	dialogID2 := base + 2
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3`, userID, dialogID1, dialogID2)
	}()

	for i, mediaType := range []int32{mtproto.MEDIA_PHOTOVIDEO, mtproto.MEDIA_PHOTOS_ONLY, mtproto.MEDIA_VIDEOS_ONLY} {
		_, _, err := NewMessagesDAO(pool).InsertOrReturnId(ctx, &dataobject.MessagesDO{
			UserId: userID, UserMessageBoxId: int32(i + 1), DialogId1: dialogID1, DialogId2: dialogID2,
			SenderUserId: userID, MessageFilterType: mediaType, MessageData: "{}",
		})
		if err != nil {
			t.Fatalf("insert media row %d: %v", i, err)
		}
	}

	count, err := NewMessagesDAO(pool).CountByMediaType(ctx, userID, dialogID1, dialogID2, mtproto.MEDIA_PHOTOVIDEO)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("photo/video count = %d, want 3", count)
	}
	count, err = NewMessagesDAO(pool).CountByMediaType(ctx, userID, dialogID1, dialogID2, mtproto.MEDIA_PHOTOS_ONLY)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("photo-only count = %d, want 1", count)
	}
}

func TestMessagePaginationUsesStrictTelegramBoundaries(t *testing.T) {
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
	userID, dialogID1, dialogID2 := base, base+1, base+2
	savedPeerType, savedPeerID := int32(1), base+3
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM messages WHERE user_id = $1 AND dialog_id1 = $2 AND dialog_id2 = $3`, userID, dialogID1, dialogID2)
	}()

	dao := NewMessagesDAO(pool)
	for i, date := range []int64{100, 200, 300, 400} {
		_, _, err := dao.InsertOrReturnId(ctx, &dataobject.MessagesDO{
			UserId: userID, UserMessageBoxId: int32(i + 10), DialogId1: dialogID1, DialogId2: dialogID2,
			SenderUserId: userID, Date2: date, Mentioned: true, MediaUnread: true,
			SavedPeerType: savedPeerType, SavedPeerId: savedPeerID, MessageData: "{}",
		})
		if err != nil {
			t.Fatalf("insert pagination row %d: %v", i, err)
		}
	}

	rows, err := dao.SelectBackwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, 300, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].UserMessageBoxId != 11 || rows[1].UserMessageBoxId != 10 {
		t.Fatalf("backward date rows = %v, want ids [11 10] strictly before date 300", messageBoxIDs(rows))
	}
	rows, err = dao.SelectBackwardSavedByOffsetDateLimit(ctx, userID, savedPeerType, savedPeerID, 300, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].UserMessageBoxId != 11 || rows[1].UserMessageBoxId != 10 {
		t.Fatalf("backward saved date rows = %v, want ids [11 10] strictly before date 300", messageBoxIDs(rows))
	}

	rows, err = dao.SelectForwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, 10, 0, 13, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].UserMessageBoxId != 10 || rows[1].UserMessageBoxId != 11 || rows[2].UserMessageBoxId != 12 {
		t.Fatalf("forward unread rows = %v, want ids [10 11 12] with zero min and exclusive max", messageBoxIDs(rows))
	}
	rows, err = dao.SelectBackwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, 13, 0, 12, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].UserMessageBoxId != 11 || rows[1].UserMessageBoxId != 10 {
		t.Fatalf("backward unread rows = %v, want ids [11 10] with exclusive max", messageBoxIDs(rows))
	}
	rows, err = dao.SelectBackwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, 13, 10, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].UserMessageBoxId != 12 || rows[1].UserMessageBoxId != 11 {
		t.Fatalf("backward unread min rows = %v, want ids [12 11] with exclusive min", messageBoxIDs(rows))
	}
}

func messageBoxIDs(rows []dataobject.MessagesDO) []int32 {
	ids := make([]int32, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].UserMessageBoxId)
	}
	return ids
}

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
