package domain

import (
	"errors"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestSecretChatAuthoritativeLifecycle(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("APIFULL_MYSQL_DSN must point to an isolated test database")
	}
	if err := Open(dsn); err != nil {
		t.Fatal(err)
	}

	seed := time.Now().UnixNano()
	chatID := int32(seed & 0x3fffffff)
	if chatID == 0 {
		chatID = 1
	}
	adminID := seed
	participantID := seed + 1
	accessHash := seed + 2
	ga := []byte{2}
	gb := []byte{3}

	created, inserted, err := CreateSecretChat(SecretChat{
		ID:            chatID,
		AccessHash:    accessHash,
		AdminID:       adminID,
		ParticipantID: participantID,
		GA:            ga,
	})
	if err != nil || !inserted || created.State != SecretChatWaiting {
		t.Fatalf("create = (%+v, %v, %v)", created, inserted, err)
	}
	retried, inserted, err := CreateSecretChat(SecretChat{
		ID:            chatID,
		AccessHash:    accessHash + 99,
		AdminID:       adminID,
		ParticipantID: participantID,
		GA:            ga,
	})
	if err != nil || inserted || retried.AccessHash != accessHash {
		t.Fatalf("idempotent create = (%+v, %v, %v)", retried, inserted, err)
	}
	if _, err = AuthorizeSecretChat(chatID, accessHash, seed+50, false); !errors.Is(err, ErrSecretChatForbidden) {
		t.Fatalf("outsider authorize = %v", err)
	}
	if _, err = AuthorizeSecretChat(chatID, accessHash+1, adminID, false); !errors.Is(err, ErrSecretChatForbidden) {
		t.Fatalf("wrong hash authorize = %v", err)
	}
	if _, _, err = AcceptSecretChat(chatID, accessHash, adminID, gb, seed+3); !errors.Is(err, ErrSecretChatForbidden) {
		t.Fatalf("admin accept = %v", err)
	}
	active, changed, err := AcceptSecretChat(chatID, accessHash, participantID, gb, seed+3)
	if err != nil || !changed || active.State != SecretChatActive {
		t.Fatalf("accept = (%+v, %v, %v)", active, changed, err)
	}
	active, changed, err = AcceptSecretChat(chatID, accessHash, participantID, gb, seed+3)
	if err != nil || changed || active.State != SecretChatActive {
		t.Fatalf("idempotent accept = (%+v, %v, %v)", active, changed, err)
	}

	const messageCount = 8
	type result struct {
		message SecretMessage
		err     error
	}
	results := make(chan result, messageCount)
	var wg sync.WaitGroup
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			message, _, saveErr := SaveSecretMessage(chatID, accessHash, adminID, seed+100+int64(i), []byte{byte(i + 1)}, i == messageCount-1, nil)
			results <- result{message: message, err: saveErr}
		}(i)
	}
	wg.Wait()
	close(results)
	qts := make([]int, 0, messageCount)
	messageByQTS := make(map[int32]SecretMessage, messageCount)
	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent save: %v", item.err)
		}
		qts = append(qts, int(item.message.QTS))
		messageByQTS[item.message.QTS] = item.message
	}
	sort.Ints(qts)
	for i, q := range qts {
		if q != i+1 {
			t.Fatalf("qts = %v", qts)
		}
	}

	first := messageByQTS[1]
	duplicate, inserted, err := SaveSecretMessage(chatID, accessHash, adminID, first.RandomID, first.Data, first.Service, nil)
	if err != nil || inserted || duplicate.QTS != 1 {
		t.Fatalf("idempotent message = (%+v, %v, %v)", duplicate, inserted, err)
	}
	if _, _, err = SaveSecretMessage(chatID, accessHash, adminID, first.RandomID, []byte("conflict"), first.Service, nil); !errors.Is(err, ErrSecretMessageConflict) {
		t.Fatalf("message conflict = %v", err)
	}

	confirmed, err := ConfirmSecretQueue(participantID, 4)
	if err != nil || len(confirmed) != 4 {
		t.Fatalf("confirm first half = (%v, %v)", confirmed, err)
	}
	confirmed, err = ConfirmSecretQueue(participantID, 4)
	if err != nil || len(confirmed) != 0 {
		t.Fatalf("confirm retry = (%v, %v)", confirmed, err)
	}
	confirmed, err = ConfirmSecretQueue(participantID, messageCount)
	if err != nil || len(confirmed) != messageCount-4 {
		t.Fatalf("confirm second half = (%v, %v)", confirmed, err)
	}
	if _, err = ConfirmSecretQueue(participantID, messageCount+1); !errors.Is(err, ErrSecretQTSInvalid) {
		t.Fatalf("confirm past tail = %v", err)
	}

	readChat, peerID, err := ReadSecretHistory(chatID, accessHash, participantID, int32(time.Now().Unix())+1)
	if err != nil || readChat.ID != chatID || peerID != adminID {
		t.Fatalf("read = (%+v, %d, %v)", readChat, peerID, err)
	}
	discarded, peerID, err := DiscardSecretChat(chatID, adminID, true)
	if err != nil || discarded.State != SecretChatDiscarded || !discarded.HistoryDeleted || peerID != participantID {
		t.Fatalf("discard = (%+v, %d, %v)", discarded, peerID, err)
	}
	if _, err = AuthorizeSecretChat(chatID, accessHash, participantID, false); !errors.Is(err, ErrSecretChatDeclined) {
		t.Fatalf("authorize discarded = %v", err)
	}
}

func TestSecretChatDeviceRetryDoesNotRollbackNewerKey(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("APIFULL_MYSQL_DSN must point to an isolated test database")
	}
	if err := Open(dsn); err != nil {
		t.Fatal(err)
	}

	seed := time.Now().UnixNano()
	chatID := int32(seed & 0x3fffffff)
	if chatID == 0 {
		chatID = 1
	}
	adminID, participantID, accessHash := seed, seed+1, seed+2
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_secret_chat_device_key WHERE chat_id=?`, chatID)
		_, _ = db.Exec(`DELETE FROM apifull_secret_chat WHERE id=?`, chatID)
	})

	if _, inserted, err := CreateSecretChat(SecretChat{
		ID: chatID, AccessHash: accessHash, AdminID: adminID, ParticipantID: participantID, GA: []byte{2},
	}); err != nil || !inserted {
		t.Fatalf("create = inserted %v err %v", inserted, err)
	}
	firstDeviceID := -(seed + 10)
	secondDeviceID := -(seed + 11)
	first, changed, err := AcceptSecretChatOnDevice(chatID, accessHash, participantID, firstDeviceID, 1, []byte{4}, seed+4)
	if err != nil || !changed || string(first.GB) != string([]byte{4}) {
		t.Fatalf("first device accept = (%+v, %v, %v)", first, changed, err)
	}
	second, changed, err := AcceptSecretChatOnDevice(chatID, accessHash, participantID, secondDeviceID, 1, []byte{5}, seed+5)
	if err != nil || !changed || string(second.GB) != string([]byte{5}) {
		t.Fatalf("second device accept = (%+v, %v, %v)", second, changed, err)
	}
	retry, changed, err := AcceptSecretChatOnDevice(chatID, accessHash, participantID, firstDeviceID, 1, []byte{4}, seed+4)
	if err != nil || changed || string(retry.GB) != string([]byte{5}) {
		t.Fatalf("stale device retry = (%+v, %v, %v)", retry, changed, err)
	}

	var keyRows, zeroDates int
	if err = db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(created_at=0),0) FROM apifull_secret_chat_device_key WHERE chat_id=?`, chatID).Scan(&keyRows, &zeroDates); err != nil {
		t.Fatal("device key rows:", err)
	}
	if keyRows != 2 || zeroDates != 0 {
		t.Fatalf("device key rows=%d zero_dates=%d, want two durable keys", keyRows, zeroDates)
	}
}
