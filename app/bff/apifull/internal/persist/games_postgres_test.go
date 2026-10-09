package persist

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestGameScoresPostgresPersistence(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	db, err := OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := &postgresStore{db: db}
	key := fmt.Sprintf("game-score-test:%d", time.Now().UnixNano())
	userID := time.Now().UnixNano()
	defer func() {
		_, _ = db.Exec(`DELETE FROM apifull_game_score WHERE scope = 'peer' AND game_key = $1`, key)
	}()

	if updated, err := store.storeGameScore("peer", key, userID, 42, false); err != nil || !updated {
		t.Fatalf("first score updated=%t err=%v", updated, err)
	}
	if updated, err := store.storeGameScore("peer", key, userID, 41, false); err != nil || updated {
		t.Fatalf("lower score updated=%t err=%v, want unchanged", updated, err)
	}
	if updated, err := store.storeGameScore("peer", key, userID, 7, true); err != nil || !updated {
		t.Fatalf("forced score updated=%t err=%v", updated, err)
	}
	if updated, err := store.storeGameScore("peer", key, userID+1, 7, false); err != nil || !updated {
		t.Fatalf("tied score updated=%t err=%v", updated, err)
	}

	scores, err := store.loadGameScores("peer", key)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 2 || scores[0].UserID != userID || scores[1].UserID != userID+1 {
		t.Fatalf("scores=%+v, want tied scores ordered by user ID", scores)
	}
}
