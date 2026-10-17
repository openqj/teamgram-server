package dialog

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func testDialogExt(order int64, topMessage int32, userID int64) *DialogExt {
	return &DialogExt{
		Order: order,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:       mtproto.MakePeerUser(userID),
			TopMessage: topMessage,
		}).To_Dialog(),
	}
}

func TestDialogExtListGetDialogsByOffsetLimitIncludesPageAfterFirstCursor(t *testing.T) {
	first := testDialogExt(300, 30, 101)
	second := testDialogExt(200, 20, 102)
	third := testDialogExt(100, 10, 103)
	list := DialogExtList{first, second, third}

	tests := []struct {
		name       string
		offsetDate int32
		offsetID   int32
		offsetPeer *mtproto.PeerUtil
		wantTopIDs []int32
	}{
		{
			name:       "empty peer cursor at first dialog",
			offsetDate: 300,
			offsetID:   30,
			offsetPeer: mtproto.MakePeerUtil(mtproto.PEER_EMPTY, 0),
			wantTopIDs: []int32{20, 10},
		},
		{
			name:       "peer cursor at first dialog",
			offsetDate: 300,
			offsetID:   30,
			offsetPeer: mtproto.MakePeerUtil(mtproto.PEER_USER, 101),
			wantTopIDs: []int32{20, 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := list.GetDialogsByOffsetLimit(tt.offsetDate, tt.offsetID, tt.offsetPeer, 2)
			if len(got) != len(tt.wantTopIDs) {
				t.Fatalf("got %d dialogs, want %d", len(got), len(tt.wantTopIDs))
			}
			for i, want := range tt.wantTopIDs {
				if got[i].GetDialog().GetTopMessage() != want {
					t.Errorf("dialog[%d] top message = %d, want %d", i, got[i].GetDialog().GetTopMessage(), want)
				}
			}
		})
	}
}
