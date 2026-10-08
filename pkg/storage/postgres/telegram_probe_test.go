package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestTelegramPostgresProbe exercises the PostgreSQL invariants that are
// visible at the Telegram update/message boundary. It is opt-in because the
// package's unit tests must remain runnable without a database. Set
// TEAMGRAM_POSTGRES_PROBE=1 to make an unavailable database fail the test.
func TestTelegramPostgresProbe(t *testing.T) {
	if os.Getenv("TEAMGRAM_POSTGRES_PROBE") != "1" {
		t.Skip("set TEAMGRAM_POSTGRES_PROBE=1 to run the PostgreSQL 18 probe")
	}
	dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_POSTGRES_DSN"))
	if dsn == "" {
		dsn = "postgres://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, Config{DSN: dsn, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("open PostgreSQL probe database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin probe transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const (
		userID    int64 = 910000001
		chatID    int64 = 910000002
		peerOne   int64 = 910000011
		peerTwo   int64 = 910000012
		peerThree int64 = 910000013
		senderID  int64 = 910000021
		randomID  int64 = 910000031
	)

	chatRowID := insertProbeChat(t, ctx, tx, chatID, userID)
	probeChatParticipants(t, ctx, tx, chatRowID, userID)
	probeDialogs(t, ctx, tx, userID, peerOne, peerTwo, peerThree)
	probeMessageRandomID(t, ctx, tx, senderID, randomID)
	probePtsOrdering(t, ctx, tx, userID)
	probeReadOutboxMonotonicity(t, ctx, tx, userID)
	probeMediaJSONB(t, ctx, tx)
}

func insertProbeChat(t *testing.T, ctx context.Context, tx pgx.Tx, chatID, creatorID int64) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO chats (id, creator_user_id, access_hash, random_id, title, date)
		VALUES ($1, $2, $3, $4, 'postgres probe', 100)
		RETURNING id`, chatID, creatorID, chatID+100, chatID+200).Scan(&id)
	if err != nil {
		t.Fatalf("insert probe chat: %v", err)
	}
	return id
}

func probeChatParticipants(t *testing.T, ctx context.Context, tx pgx.Tx, chatID, userID int64) {
	t.Helper()
	const insert = `
		INSERT INTO chat_participants (chat_id, user_id, participant_type, state, invited_at)
		VALUES ($1, $2, 0, $3, $4)
		ON CONFLICT (chat_id, user_id) DO UPDATE
		SET state = EXCLUDED.state, invited_at = EXCLUDED.invited_at`
	if _, err := tx.Exec(ctx, insert, chatID, userID, 1, 100); err != nil {
		t.Fatalf("insert chat participant: %v", err)
	}
	if _, err := tx.Exec(ctx, insert, chatID, userID, 2, 200); err != nil {
		t.Fatalf("idempotent chat participant upsert: %v", err)
	}
	var count int
	var state int
	err := tx.QueryRow(ctx, `SELECT count(*), max(state) FROM chat_participants WHERE chat_id=$1 AND user_id=$2`, chatID, userID).Scan(&count, &state)
	if err != nil {
		t.Fatalf("read chat participant: %v", err)
	}
	if count != 1 || state != 2 {
		t.Fatalf("chat participant uniqueness/state: count=%d state=%d", count, state)
	}
}

func probeDialogs(t *testing.T, ctx context.Context, tx pgx.Tx, userID, peerOne, peerTwo, peerThree int64) {
	t.Helper()
	const upsert = `
		INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, top_message, date2)
		VALUES ($1, 0, $2, $2, $3, $4)
		ON CONFLICT (user_id, peer_dialog_id) DO UPDATE
		SET top_message=EXCLUDED.top_message, date2=EXCLUDED.date2, deleted=FALSE`
	for _, item := range []struct {
		peer int64
		date int64
		top  int32
	}{{peerOne, 100, 1}, {peerTwo, 200, 2}, {peerThree, 300, 3}} {
		if _, err := tx.Exec(ctx, upsert, userID, item.peer, item.top, item.date); err != nil {
			t.Fatalf("upsert dialog %d: %v", item.peer, err)
		}
	}
	if _, err := tx.Exec(ctx, upsert, userID, peerTwo, 22, 350); err != nil {
		t.Fatalf("repeat dialog upsert: %v", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT peer_dialog_id, top_message FROM dialogs
		WHERE user_id=$1 AND deleted=FALSE
		ORDER BY date2 DESC, peer_dialog_id DESC LIMIT 2`, userID)
	if err != nil {
		t.Fatalf("paginate dialogs: %v", err)
	}
	defer rows.Close()
	type dialog struct {
		peer int64
		top  int32
	}
	got := make([]dialog, 0, 2)
	for rows.Next() {
		var item dialog
		if err := rows.Scan(&item.peer, &item.top); err != nil {
			t.Fatalf("scan dialog page: %v", err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read dialog page: %v", err)
	}
	if len(got) != 2 || got[0].peer != peerTwo || got[0].top != 22 || got[1].peer != peerThree {
		t.Fatalf("dialog order/page = %#v", got)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM dialogs WHERE user_id=$1 AND peer_dialog_id=$2`, userID, peerTwo).Scan(&count); err != nil {
		t.Fatalf("count upserted dialog: %v", err)
	}
	if count != 1 {
		t.Fatalf("dialog upsert created %d rows", count)
	}
}

func probeMessageRandomID(t *testing.T, ctx context.Context, tx pgx.Tx, senderID, randomID int64) {
	t.Helper()
	const insert = `
		INSERT INTO messages (user_id, user_message_box_id, sender_user_id, peer_type, peer_id, random_id, date2)
		VALUES ($1, $2, $3, 0, $4, $5, $6)
		ON CONFLICT (sender_user_id, random_id) WHERE random_id <> 0 DO NOTHING`
	for _, boxID := range []int32{1, 2} {
		if _, err := tx.Exec(ctx, insert, senderID, boxID, senderID, senderID+100, randomID, int64(boxID)); err != nil {
			t.Fatalf("insert random-id message: %v", err)
		}
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM messages WHERE sender_user_id=$1 AND random_id=$2`, senderID, randomID).Scan(&count); err != nil {
		t.Fatalf("count random-id message: %v", err)
	}
	if count != 1 {
		t.Fatalf("random-id idempotency count=%d", count)
	}
}

func probePtsOrdering(t *testing.T, ctx context.Context, tx pgx.Tx, userID int64) {
	t.Helper()
	const insert = `
		INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2)
		VALUES ($1, $2, 1, 1, '{}', $3)
		ON CONFLICT (user_id, pts) DO NOTHING`
	for _, pts := range []int32{3, 1, 2, 2} {
		if _, err := tx.Exec(ctx, insert, userID, pts, int64(pts)); err != nil {
			t.Fatalf("insert pts=%d: %v", pts, err)
		}
	}
	rows, err := tx.Query(ctx, `SELECT pts FROM user_pts_updates WHERE user_id=$1 ORDER BY pts ASC`, userID)
	if err != nil {
		t.Fatalf("select pts updates: %v", err)
	}
	defer rows.Close()
	got := make([]int32, 0, 3)
	for rows.Next() {
		var pts int32
		if err := rows.Scan(&pts); err != nil {
			t.Fatalf("scan pts update: %v", err)
		}
		got = append(got, pts)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read pts updates: %v", err)
	}
	want := []int32{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("pts rows=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pts order=%v", got)
		}
	}
}

func probeReadOutboxMonotonicity(t *testing.T, ctx context.Context, tx pgx.Tx, userID int64) {
	t.Helper()
	const upsert = `
		INSERT INTO message_read_outbox (user_id, peer_dialog_id, read_user_id, read_outbox_max_id, read_outbox_max_date)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, peer_dialog_id, read_user_id) DO UPDATE SET
		read_outbox_max_id=GREATEST(message_read_outbox.read_outbox_max_id, EXCLUDED.read_outbox_max_id),
		read_outbox_max_date=GREATEST(message_read_outbox.read_outbox_max_date, EXCLUDED.read_outbox_max_date)`
	for _, item := range [][2]int64{{10, 100}, {5, 50}, {20, 200}} {
		if _, err := tx.Exec(ctx, upsert, userID, 910000040, 910000041, item[0], item[1]); err != nil {
			t.Fatalf("upsert read outbox %v: %v", item, err)
		}
	}
	var maxID, maxDate int64
	if err := tx.QueryRow(ctx, `SELECT read_outbox_max_id, read_outbox_max_date FROM message_read_outbox WHERE user_id=$1 AND peer_dialog_id=$2 AND read_user_id=$3`, userID, 910000040, 910000041).Scan(&maxID, &maxDate); err != nil {
		t.Fatalf("read outbox state: %v", err)
	}
	if maxID != 20 || maxDate != 200 {
		t.Fatalf("read outbox regressed: id=%d date=%d", maxID, maxDate)
	}
}

func probeMediaJSONB(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	const documentID int64 = 910000051
	if _, err := tx.Exec(ctx, `
		INSERT INTO documents (document_id, access_hash, dc_id, file_path, attributes)
		VALUES ($1, $2, 2, 'probe.bin', '{"mime":"application/octet-stream","v":1}'::jsonb)`, documentID, documentID+1); err != nil {
		t.Fatalf("insert media document: %v", err)
	}
	var mime string
	var version int
	if err := tx.QueryRow(ctx, `SELECT attributes->>'mime', (attributes->>'v')::int FROM documents WHERE document_id=$1`, documentID).Scan(&mime, &version); err != nil {
		t.Fatalf("read media JSONB: %v", err)
	}
	if mime != "application/octet-stream" || version != 1 {
		t.Fatalf("media JSONB = mime=%q version=%d", mime, version)
	}
}
