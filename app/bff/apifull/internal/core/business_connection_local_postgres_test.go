package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestBusinessConnectionUpdates(t *testing.T) {
	record := domain.BusinessConnectionRecord{
		ConnectionID: "0123456789abcdef",
		OwnerUserID:  1001,
		BotUserID:    2002,
		DCID:         2,
		Date:         1700000000,
		QTS:          7,
		CanReply:     true,
		Rights:       []byte(`{"reply":true}`),
	}
	updates, err := businessConnectionUpdates(record)
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates || len(updates.GetUpdates()) != 1 {
		t.Fatalf("build connection update: updates=%+v err=%v", updates, err)
	}
	update := updates.GetUpdates()[0]
	if update.GetPredicateName() != mtproto.Predicate_updateBotBusinessConnect || update.GetConnection() == nil {
		t.Fatalf("unexpected connection update: %+v", update)
	}
	connection := update.GetConnection()
	if connection.GetConnectionId() != record.ConnectionID || connection.GetUserId() != record.OwnerUserID || connection.GetDcId() != record.DCID || connection.GetDate() != record.Date || update.GetQts() != record.QTS || !connection.GetCanReply() {
		t.Fatalf("connection update mismatch: connection=%+v qts=%d record=%+v", connection, update.GetQts(), record)
	}
}
