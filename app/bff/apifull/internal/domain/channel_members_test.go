package domain

import (
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestCommonChannelIDsIncludesActiveMembers(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	if err := Open(dsn); err != nil {
		t.Fatal(err)
	}
	cleanupDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })
	channelID := time.Now().UnixNano()
	const owner, member, outsider int64 = 93101, 93102, 93103
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := cleanupDB.Exec(query, channelID); err != nil {
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
