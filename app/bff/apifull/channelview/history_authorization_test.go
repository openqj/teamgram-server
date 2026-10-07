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
	legacyChannelID := channelID + 1
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
		for _, query := range []string{
			`DELETE FROM channel_messages WHERE channel_id=?`,
			`DELETE FROM channel_participants WHERE channel_id=?`,
			`DELETE FROM channels WHERE id=?`,
		} {
			if _, err := cleanupDB.Exec(query, legacyChannelID); err != nil {
				t.Errorf("cleanup legacy channel fixture: %v", err)
			}
		}
		if _, err := cleanupDB.Exec(`DELETE FROM apifull_channel WHERE id=?`, legacyChannelID); err != nil {
			t.Errorf("cleanup colliding APIFull channel fixture: %v", err)
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
	if author, err := ChannelMessageAuthor(member, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel(), 1); err != nil || author != owner {
		t.Fatalf("APIFull channel author: author=%d err=%v, want %d", author, err, owner)
	}
	if _, err := ChannelMessageAuthor(member, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputChannel(), 1); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad APIFull channel access hash: got %v", err)
	}

	legacyAccessHash := channelID + 2
	if _, err := cleanupDB.Exec(`INSERT INTO channels (id, creator_user_id, access_hash, title, broadcast, megagroup, date)
		VALUES (?, ?, ?, ?, 1, 0, ?)`, legacyChannelID, owner, legacyAccessHash, "legacy-author-test", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanupDB.Exec(`INSERT INTO channel_participants (channel_id, user_id, participant_type, joined_at, state)
		VALUES (?, ?, 0, ?, 0)`, legacyChannelID, member, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanupDB.Exec(`INSERT INTO channel_messages
		(channel_id, message_id, dialog_message_id, sender_user_id, message_data, message, date2)
		VALUES (?, 1, ?, ?, ?, 'legacy', ?), (?, 2, ?, ?, ?, 'anonymous', ?)`,
		legacyChannelID, legacyChannelID, outsider, `{"from_id":{"predicate_name":"peerUser","user_id":91003}}`, time.Now().Unix(),
		legacyChannelID, legacyChannelID+1, outsider, `{"from_id":{"predicate_name":"peerChannel","channel_id":77}}`, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if author, found, err := domain.ChannelMessageAuthor(member, legacyChannelID, legacyAccessHash, 1); err != nil || !found || author != outsider {
		t.Fatalf("legacy channel author: author=%d found=%t err=%v, want %d", author, found, err, outsider)
	}
	if author, found, err := domain.ChannelMessageAuthor(member, legacyChannelID, legacyAccessHash, 2); err != nil || !found || author != 0 {
		t.Fatalf("anonymous legacy channel author: author=%d found=%t err=%v, want an empty author", author, found, err)
	}
	if _, _, err := domain.ChannelMessageAuthor(outsider, legacyChannelID, legacyAccessHash, 1); !errors.Is(err, domain.ErrNotChannelMember) {
		t.Fatalf("legacy channel outsider: got %v, want ErrNotChannelMember", err)
	}
	if _, _, err := domain.ChannelMessageAuthor(member, legacyChannelID, legacyAccessHash+1, 1); !errors.Is(err, domain.ErrChannelMissing) {
		t.Fatalf("legacy channel wrong access hash: got %v, want ErrChannelMissing", err)
	}
	if err := domain.SaveChannel(domain.Channel{
		ID: legacyChannelID, AccessHash: legacyAccessHash + 1, Creator: owner, Title: "colliding-apifull-author-test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ChannelMessageAuthor(member, mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: legacyChannelID, AccessHash: legacyAccessHash + 1,
	}).To_InputChannel(), 1); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("legacy ID collision with APIFull channel: got %v, want CHANNEL_INVALID", err)
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
	if _, err := Post(owner, channelID, "announcement #topic", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	hashtagSearch, err := SearchForInputPeer(member, input, "#topic", 0, 0, 0, 0, 0, 0, 0, 20)
	if err != nil || len(hashtagSearch.GetMessages()) != 1 || hashtagSearch.GetMessages()[0].GetMessage() != "announcement #topic" {
		t.Fatalf("member hashtag search: messages=%d err=%v", len(hashtagSearch.GetMessages()), err)
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
