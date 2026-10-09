package domain

import (
	"testing"
	"time"
)

func TestListLeftChannelsUsesLatestLeaveEvent(t *testing.T) {
	requirePaymentLedgerDB(t)
	base := time.Now().UnixNano()
	owner, member := base+1, base+2
	leftID, activeID := base+3, base+4
	cleanup := func(channelID int64) {
		_, _ = db.Exec(`DELETE FROM apifull_channel_admin_log WHERE channel_id=?`, channelID)
		_, _ = db.Exec(`DELETE FROM apifull_channel_member WHERE channel_id=?`, channelID)
		_, _ = db.Exec(`DELETE FROM apifull_channel WHERE id=?`, channelID)
	}
	t.Cleanup(func() {
		cleanup(leftID)
		cleanup(activeID)
	})
	for _, channelID := range []int64{leftID, activeID} {
		if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "takeout"}); err != nil {
			t.Fatal(err)
		}
		if err := JoinChannel(channelID, member); err != nil {
			t.Fatal(err)
		}
	}
	if err := LeaveChannel(leftID, member); err != nil {
		t.Fatal(err)
	}
	left, err := ListLeftChannels(member, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].ID != leftID {
		t.Fatalf("left channels = %+v, want [%d]", left, leftID)
	}
	if got, err := ListLeftChannels(member, 1, 100); err != nil || len(got) != 0 {
		t.Fatalf("offset page = %+v, err=%v, want empty", got, err)
	}
	if err := JoinChannel(leftID, member); err != nil {
		t.Fatal(err)
	}
	left, err = ListLeftChannels(member, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("rejoined channel remained left: %+v", left)
	}
}
