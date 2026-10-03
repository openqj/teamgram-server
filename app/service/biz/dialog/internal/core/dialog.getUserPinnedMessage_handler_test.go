package core

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

func TestPinnedMessageValue(t *testing.T) {
	if got := pinnedMessageValue(nil).GetV(); got != 0 {
		t.Fatalf("nil dialog: got %d, want 0", got)
	}
	if got := pinnedMessageValue(&dataobject.DialogsDO{PinnedMsgId: 42}).GetV(); got != 42 {
		t.Fatalf("stored pinned message: got %d, want 42", got)
	}
}
