package channelview

import (
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestHistoryAndEditDataCheckInputPeerAuthorization(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("APIFULL_MYSQL_DSN must point to an isolated test database")
	}
	if err := domain.Open(dsn); err != nil {
		t.Fatal(err)
	}
	cleanupDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })

	channelID := time.Now().UnixNano()
	const owner, member, outsider int64 = 91001, 91002, 91003
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := cleanupDB.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "history-auth-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	if _, err := Post(owner, channelID, "stored", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}

	input := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputPeer()
	got, err := HistoryForInputPeer(member, input, 0, 20)
	if err != nil || len(got.GetMessages()) != 1 {
		t.Fatalf("member history: messages=%d err=%v", len(got.GetMessages()), err)
	}
	search, err := SearchForInputPeer(member, input, "stored", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(search.GetMessages()) != 1 {
		t.Fatalf("member search: messages=%d err=%v", len(search.GetMessages()), err)
	}

	editData, err := GetMessageEditData(owner, input, got.GetMessages()[0].GetId())
	if err != nil || editData == nil || editData.GetCaption() {
		t.Fatalf("creator edit data: result=%v err=%v", editData, err)
	}
	if _, err = GetMessageEditData(member, input, got.GetMessages()[0].GetId()); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-creator edit data: got %v", err)
	}
	if _, err = GetMessageEditData(owner, input, got.GetMessages()[0].GetId()+10); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("missing message edit data: got %v", err)
	}

	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputPeer()
	if _, err = HistoryForInputPeer(member, badHash, 0, 20); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash: got %v", err)
	}
	if _, err = SearchForInputPeer(member, badHash, "stored", 0, 0, 0, 0, 0, 0, 0, 20); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad search access hash: got %v", err)
	}
	if _, err = GetMessageEditData(owner, badHash, 1); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad edit-data access hash: got %v", err)
	}
	if _, err = HistoryForInputPeer(outsider, input, 0, 20); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("non-member history: got %v", err)
	}
	if _, err = SearchForInputPeer(outsider, input, "stored", 0, 0, 0, 0, 0, 0, 0, 20); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("non-member search: got %v", err)
	}
	if _, err = GetMessageEditData(outsider, input, 1); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("non-member edit data: got %v", err)
	}
}
