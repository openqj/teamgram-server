package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDialogMarkSavedHistoryReadFailsClosedWithoutSavedDialogStorage(t *testing.T) {
	request := &dialog.TLDialogInsertOrUpdateDialog{
		UserId:         910061,
		PeerType:       mtproto.PEER_CHAT,
		PeerId:         910062,
		ReadInboxMaxId: wrapperspb.Int32(1),
	}

	if got, err := (&DialogCore{}).DialogMarkSavedHistoryRead(request); got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("missing saved-dialog storage = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}

	var nilCore *DialogCore
	if got, err := nilCore.DialogMarkSavedHistoryRead(request); got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("nil dialog core = (%v, %v), want METHOD_NOT_IMPL", got, err)
	}
}
