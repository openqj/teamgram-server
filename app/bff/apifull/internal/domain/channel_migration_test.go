package domain

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func requireMigrationDB(t *testing.T) {
	t.Helper()
	requirePaymentLedgerDB(t)
}

func cleanupMigrationChannel(t *testing.T, channelID int64) {
	t.Helper()
	t.Cleanup(func() {
		if db == nil {
			return
		}
		for _, query := range []string{
			`DELETE FROM apifull_channel_delivery_recipient WHERE delivery_id IN (SELECT id FROM apifull_channel_delivery_outbox WHERE channel_id=?)`,
			`DELETE FROM apifull_channel_delivery_outbox WHERE channel_id=?`,
			`DELETE FROM apifull_channel_event WHERE channel_id=?`,
			`DELETE FROM apifull_channel_admin_log WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_request WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_content_read WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("cleanup channel fixture: %v", err)
			}
		}
	})
}

func TestImportMigratedChannelIsIdempotentAndPreservesRoster(t *testing.T) {
	requireMigrationDB(t)
	channelID := time.Now().UnixNano()
	chatID := channelID + 1
	creatorID := channelID + 2
	memberID := channelID + 3
	cleanupMigrationChannel(t, channelID)

	input := MigratedChannel{
		ChatID:     chatID,
		ChannelID:  channelID,
		AccessHash: channelID + 10,
		CreatorID:  creatorID,
		Title:      "migration-test",
		CreatedAt:  time.Now().Unix(),
		Members: []MigratedChannelMember{{
			UserID:    memberID,
			InvitedBy: creatorID,
			JoinedAt:  time.Now().Unix(),
			AdminRights: &ChannelAdminRights{
				PostMessages: true,
				BanUsers:     true,
			},
			Rank: "moderator",
		}},
	}
	if err := ImportMigratedChannel(input); err != nil {
		t.Fatal(err)
	}
	if err := ImportMigratedChannel(input); err != nil {
		t.Fatalf("repeating migration: %v", err)
	}

	member, ok, err := LoadChannelMember(channelID, memberID)
	if err != nil || !ok {
		t.Fatalf("load migrated member: member=%+v ok=%v err=%v", member, ok, err)
	}
	if member.AdminRights == nil || !member.AdminRights.PostMessages || !member.AdminRights.BanUsers || member.Rank != "moderator" {
		t.Fatalf("migrated member rights were not preserved: %+v", member)
	}

	conflict := input
	conflict.AccessHash++
	if err := ImportMigratedChannel(conflict); !errors.Is(err, ErrChannelMigrationConflict) {
		t.Fatalf("conflicting migration error = %v", err)
	}
	channel, ok, err := LoadChannel(channelID)
	if err != nil || !ok || channel.AccessHash != input.AccessHash || channel.MigratedFromChatID != chatID {
		t.Fatalf("stored migration changed after conflict: channel=%+v ok=%v err=%v", channel, ok, err)
	}
}

func TestMigratedChannelMessageIDsAndReadCursorAreMonotonic(t *testing.T) {
	requireMigrationDB(t)
	channelID := time.Now().UnixNano()
	creatorID := channelID + 1
	memberID := channelID + 2
	cleanupMigrationChannel(t, channelID)
	if err := ImportMigratedChannel(MigratedChannel{
		ChatID:     channelID + 3,
		ChannelID:  channelID,
		AccessHash: channelID + 4,
		CreatorID:  creatorID,
		Title:      "sequence-test",
		CreatedAt:  time.Now().Unix(),
		Members:    []MigratedChannelMember{{UserID: memberID}},
	}); err != nil {
		t.Fatal(err)
	}

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := InsertChannelMessage(channelID, memberID, 0, fmt.Sprintf("message-%d", i))
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	var count, minID, maxID, lastID, pts int32
	if err := db.QueryRow(`SELECT COUNT(*), MIN(message_id), MAX(message_id) FROM apifull_channel_message WHERE channel_id=?`, channelID).
		Scan(&count, &minID, &maxID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT last_message_id, pts FROM apifull_channel_message_seq WHERE channel_id=?`, channelID).
		Scan(&lastID, &pts); err != nil {
		t.Fatal(err)
	}
	if count != writers || minID != 1 || maxID != writers || lastID != writers || pts != writers {
		t.Fatalf("message sequence = count:%d min:%d max:%d last:%d pts:%d", count, minID, maxID, lastID, pts)
	}

	if got, err := MarkChannelReadHistory(memberID, channelID, writers); err != nil || got != writers {
		t.Fatalf("mark read at top = %d, %v", got, err)
	}
	if got, err := MarkChannelReadHistory(memberID, channelID, 1); err != nil || got != writers {
		t.Fatalf("read cursor regressed = %d, %v", got, err)
	}
	if got, err := ChannelReadMaxID(memberID, channelID); err != nil || got != writers {
		t.Fatalf("stored read cursor = %d, %v", got, err)
	}
}

func TestConvertChannelToGigagroupPostgresRoundTrip(t *testing.T) {
	requireMigrationDB(t)
	channelID := time.Now().UnixNano()
	creatorID := channelID + 1
	cleanupMigrationChannel(t, channelID)
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID + 2, Creator: creatorID, Title: "gigagroup-test", Broadcast: true}); err != nil {
		t.Fatal(err)
	}
	converted, err := ConvertChannelToGigagroup(creatorID, channelID, channelID+2)
	if err != nil {
		t.Fatal(err)
	}
	if converted.Broadcast || !converted.Megagroup || converted.ID != channelID {
		t.Fatalf("converted channel = %+v", converted)
	}
	if _, err = ConvertChannelToGigagroup(creatorID, channelID, channelID+3); !errors.Is(err, ErrInvalidChannelAccessHash) {
		t.Fatalf("stale access hash error = %v", err)
	}
}
