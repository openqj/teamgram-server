package domain

import (
	"errors"
	"testing"
	"time"
)

func TestPaidGroupCallMessageSettlesAndAggregates(t *testing.T) {
	requireMigrationDB(t)
	callID := time.Now().UnixNano()
	creatorID := callID + 1
	senderID := callID + 2
	randomID := callID + 3
	_, err := db.Exec(`INSERT INTO apifull_group_call
		(id, access_hash, creator_user_id, channel_id, title, participants, created_at)
		VALUES ($1,$2,$3,0,'paid-stars-test',$4,$5)`, callID, callID+4, creatorID, "[]", time.Now().Unix())
	if err != nil {
		t.Fatalf("insert call: %v", err)
	}
	cleanup := func() {
		if _, cleanupErr := db.Exec(`DELETE FROM apifull_group_call_message WHERE call_id=$1`, callID); cleanupErr != nil {
			t.Errorf("cleanup group call messages: %v", cleanupErr)
		}
		if _, cleanupErr := db.Exec(`DELETE FROM apifull_group_call WHERE id=$1`, callID); cleanupErr != nil {
			t.Errorf("cleanup group call: %v", cleanupErr)
		}
		if _, cleanupErr := db.Exec(`DELETE FROM apifull_star_tx WHERE user_id IN ($1,$2)`, senderID, creatorID); cleanupErr != nil {
			t.Errorf("cleanup star transactions: %v", cleanupErr)
		}
		if _, cleanupErr := db.Exec(`DELETE FROM apifull_stars WHERE user_id IN ($1,$2)`, senderID, creatorID); cleanupErr != nil {
			t.Errorf("cleanup stars: %v", cleanupErr)
		}
	}
	t.Cleanup(cleanup)
	if _, err = ApplyStars(senderID, 20, "group-call-seed"); err != nil {
		t.Fatalf("seed sender balance: %v", err)
	}
	messageJSON := `{"text":"paid"}`
	first, created, err := CreatePaidGroupCallMessage(callID, senderID, randomID, messageJSON, "", 7)
	if err != nil || !created || first.PaidStars == nil || *first.PaidStars != 7 {
		t.Fatalf("first paid message: %+v created=%v err=%v", first, created, err)
	}
	if balance, err := StarsBalance(senderID); err != nil || balance != 13 {
		t.Fatalf("sender balance=%d err=%v", balance, err)
	}
	if balance, err := StarsBalance(creatorID); err != nil || balance != 7 {
		t.Fatalf("creator balance=%d err=%v", balance, err)
	}
	replay, created, err := CreatePaidGroupCallMessage(callID, senderID, randomID, messageJSON, "", 7)
	if err != nil || created || replay.ID != first.ID {
		t.Fatalf("idempotent replay: %+v created=%v err=%v", replay, created, err)
	}
	if balance, err := StarsBalance(senderID); err != nil || balance != 13 {
		t.Fatalf("replay changed sender balance=%d err=%v", balance, err)
	}
	if _, _, err = CreatePaidGroupCallMessage(callID, senderID, randomID, messageJSON, "", 8); !errors.Is(err, ErrRandomIDConflict) {
		t.Fatalf("conflicting replay err=%v, want ErrRandomIDConflict", err)
	}
	total, donors, err := LoadGroupCallStars(callID)
	if err != nil || total != 7 || len(donors) != 1 || donors[0].UserID != senderID || donors[0].Stars != 7 {
		t.Fatalf("group call stars total=%d donors=%+v err=%v", total, donors, err)
	}
}
