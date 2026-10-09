package domain

import (
	"testing"
	"time"
)

func TestCommonChannelIDsIncludesActiveMembers(t *testing.T) {
	requirePaymentLedgerDB(t)
	channelID := time.Now().UnixNano()
	const owner, member, outsider int64 = 93101, 93102, 93103
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "common-channel-test"}); err != nil {
		t.Fatal(err)
	}
	if err := JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	ids, err := CommonChannelIDs(owner, member)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != channelID {
		t.Fatalf("common channels: got %v, want [%d]", ids, channelID)
	}
	ids, err = CommonChannelIDs(owner, outsider)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("outsider common channels: got %v", ids)
	}
}
