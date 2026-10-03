package domain

import (
	"errors"
	"testing"
	"time"
)

func TestMarkChannelMessageContentsReadIsDurableAndIdempotent(t *testing.T) {
	requireMigrationDB(t)
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	memberID := channelID + 2
	nonMemberID := channelID + 3
	accessHash := channelID + 10
	if err := SaveChannel(Channel{ID: channelID, AccessHash: accessHash, Creator: ownerID, Title: "content-read", Broadcast: true}); err != nil {
		t.Fatal(err)
	}
	if err := InviteChannelMembers(channelID, ownerID, []int64{memberID}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_content_read WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})
	first, err := InsertChannelMessage(channelID, ownerID, 0, "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := InsertChannelMessage(channelID, ownerID, 0, "two")
	if err != nil {
		t.Fatal(err)
	}
	third, err := InsertChannelMessage(channelID, ownerID, 0, "three")
	if err != nil {
		t.Fatal(err)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{first.MessageID, second.MessageID}); err != nil {
		t.Fatalf("mark content read: %v", err)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{first.MessageID}); err != nil {
		t.Fatalf("idempotent content read: %v", err)
	}
	countReceipts := func() int {
		t.Helper()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message_content_read WHERE user_id=? AND channel_id=?`, memberID, channelID).Scan(&count); err != nil {
			t.Fatalf("count content reads: %v", err)
		}
		return count
	}
	if count := countReceipts(); count != 2 {
		t.Fatalf("stored content reads = %d, want 2", count)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{third.MessageID, third.MessageID}); !errors.Is(err, ErrDuplicateMessageID) {
		t.Fatalf("duplicate message error = %v, want ErrDuplicateMessageID", err)
	}
	if count := countReceipts(); count != 2 {
		t.Fatalf("duplicate request changed receipt count to %d", count)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{third.MessageID, 999}); !errors.Is(err, ErrMessageMissing) {
		t.Fatalf("mixed missing message error = %v, want ErrMessageMissing", err)
	}
	if count := countReceipts(); count != 2 {
		t.Fatalf("mixed missing request changed receipt count to %d", count)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash+1, []int32{third.MessageID}); !errors.Is(err, ErrInvalidChannelAccessHash) {
		t.Fatalf("wrong access hash error = %v, want ErrInvalidChannelAccessHash", err)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, nil); !errors.Is(err, ErrInvalidMessageID) {
		t.Fatalf("empty IDs error = %v, want ErrInvalidMessageID", err)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{0}); !errors.Is(err, ErrInvalidMessageID) {
		t.Fatalf("invalid ID error = %v, want ErrInvalidMessageID", err)
	}
	var count int
	if err = MarkChannelMessageContentsRead(nonMemberID, channelID, accessHash, []int32{first.MessageID}); !errors.Is(err, ErrNotChannelMember) {
		t.Fatalf("nonmember error = %v, want ErrNotChannelMember", err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message_content_read WHERE user_id=? AND channel_id=?`, memberID, channelID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("unauthorized request changed receipt count to %d", count)
	}
}

func TestChannelMessageContentReadReceiptsFollowMessageDeletion(t *testing.T) {
	requireMigrationDB(t)
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	memberID := channelID + 2
	accessHash := channelID + 10
	if err := SaveChannel(Channel{ID: channelID, AccessHash: accessHash, Creator: ownerID, Title: "content-read-delete", Broadcast: false}); err != nil {
		t.Fatal(err)
	}
	if err := InviteChannelMembers(channelID, ownerID, []int64{memberID}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_content_read WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})
	countReceipts := func(userID int64) int {
		t.Helper()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_message_content_read WHERE user_id=? AND channel_id=?`, userID, channelID).Scan(&count); err != nil {
			t.Fatalf("count content reads: %v", err)
		}
		return count
	}
	first, err := InsertChannelMessage(channelID, ownerID, 0, "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := InsertChannelMessage(channelID, memberID, 0, "two")
	if err != nil {
		t.Fatal(err)
	}
	third, err := InsertChannelMessage(channelID, ownerID, 0, "three")
	if err != nil {
		t.Fatal(err)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{first.MessageID, second.MessageID, third.MessageID}); err != nil {
		t.Fatalf("mark content read: %v", err)
	}
	if _, _, err = DeleteChannelMessages(channelID, ownerID, []int32{first.MessageID}); err != nil {
		t.Fatalf("delete individual message: %v", err)
	}
	if count := countReceipts(memberID); count != 2 {
		t.Fatalf("individual delete left %d receipts, want 2", count)
	}
	if _, _, err = DeleteChannelHistory(channelID, ownerID, third.MessageID); err != nil {
		t.Fatalf("delete channel history: %v", err)
	}
	if count := countReceipts(memberID); count != 0 {
		t.Fatalf("history delete left %d receipts, want 0", count)
	}
	participantMessage, err := InsertChannelMessage(channelID, memberID, 0, "participant")
	if err != nil {
		t.Fatal(err)
	}
	if err = MarkChannelMessageContentsRead(ownerID, channelID, accessHash, []int32{participantMessage.MessageID}); err != nil {
		t.Fatalf("mark participant content read: %v", err)
	}
	if _, _, err = DeleteChannelParticipantHistory(channelID, ownerID, memberID); err != nil {
		t.Fatalf("delete participant history: %v", err)
	}
	if count := countReceipts(ownerID); count != 0 {
		t.Fatalf("participant history delete left %d receipts, want 0", count)
	}
	finalMessage, err := InsertChannelMessage(channelID, ownerID, 0, "final")
	if err != nil {
		t.Fatal(err)
	}
	if err = MarkChannelMessageContentsRead(memberID, channelID, accessHash, []int32{finalMessage.MessageID}); err != nil {
		t.Fatalf("mark final content read: %v", err)
	}
	if err = DeleteChannel(ownerID, channelID); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	if count := countReceipts(memberID); count != 0 {
		t.Fatalf("channel delete left %d receipts, want 0", count)
	}
}
