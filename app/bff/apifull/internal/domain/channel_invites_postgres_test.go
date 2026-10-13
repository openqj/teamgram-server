package domain

import (
	"fmt"
	"testing"
	"time"
)

func TestImportChannelInvitePostgresScansChannelFlags(t *testing.T) {
	requirePaymentLedgerDB(t)
	now := time.Now().Unix()
	channelID := time.Now().UnixNano()
	const owner, member int64 = 93201, 93202
	link := fmt.Sprintf("invite-%d", channelID)
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM chat_invite_participants WHERE chat_id=?`,
			`DELETE FROM chat_invites WHERE chat_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("cleanup invite fixture: %v", err)
			}
		}
	})

	if err := SaveChannel(Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "invite-import-test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateChannelInvite(ChannelInvite{
		ChannelID: channelID, AdminID: owner, Link: link, UsageLimit: 1, Date: now,
	}); err != nil {
		t.Fatal(err)
	}

	imported, err := ImportChannelInvite(link, member, now)
	if err != nil {
		t.Fatalf("import channel invite: %v", err)
	}
	if imported.Channel.ID != channelID || imported.RequestNeeded {
		t.Fatalf("import result = %+v, want channel %d without request", imported, channelID)
	}

	joined, found, err := LoadChannelMember(channelID, member)
	if err != nil || !found || joined.UserID != member {
		t.Fatalf("member = %+v found=%v err=%v, want persisted member", joined, found, err)
	}
	var participants int
	if err = db.QueryRow(`SELECT COUNT(*) FROM chat_invite_participants WHERE chat_id=? AND link=? AND user_id=? AND requested=FALSE`, channelID, link, member).Scan(&participants); err != nil {
		t.Fatal(err)
	}
	if participants != 1 {
		t.Fatalf("invite participant rows = %d, want 1", participants)
	}
}
