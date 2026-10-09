package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func requireChatlistsDB(t *testing.T) int64 {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open chatlists database: %v", err)
	}
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_chatlist_invite WHERE owner_user_id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM apifull_chatlist_state WHERE user_id=$1`, userID)
		_ = Close()
	})
	return userID
}

func TestChatlistStateAndInviteIndexCommitTogether(t *testing.T) {
	userID := requireChatlistsDB(t)
	state := map[string]any{"next": 1, "joined": []any{}, "hidden": []any{}}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	peersJSON := []byte(`[]`)
	slug := fmt.Sprintf("chatlist-%d", userID)
	if err = SaveChatlistState(userID, stateJSON, []ChatlistInviteRecord{{
		Slug: slug, FilterID: 2, Title: "shared", PeersJSON: peersJSON,
	}}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	raw, err := LoadChatlistState(userID)
	if err != nil || string(raw) == "" {
		t.Fatalf("load state: %q %v", raw, err)
	}
	owner, invite, found, err := FindChatlistInvite(slug)
	if err != nil || !found || owner != userID || invite.FilterID != 2 || invite.Title != "shared" || string(invite.PeersJSON) != string(peersJSON) {
		t.Fatalf("invite lookup: owner=%d invite=%+v found=%v err=%v", owner, invite, found, err)
	}

	if err = SaveChatlistState(userID, []byte(`{"next":2}`), nil); err != nil {
		t.Fatalf("replace state: %v", err)
	}
	if _, _, found, err = FindChatlistInvite(slug); err != nil || found {
		t.Fatalf("stale invite after replacement: found=%v err=%v", found, err)
	}
}
