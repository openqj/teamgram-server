package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMediaGetPhotoSizeListListRejectsNilRequest(t *testing.T) {
	var core *MediaCore
	if _, err := core.MediaGetPhotoSizeListList(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MediaGetPhotoSizeListList(nil) error = %v, want INPUT_REQUEST_INVALID", err)
	}
}

func TestPhotoSizeListsInRequestOrder(t *testing.T) {
	sizes := map[int64][]*mtproto.PhotoSize{
		11: {{Type: "a"}},
		22: {{Type: "b"}},
	}
	got := photoSizeListsInRequestOrder([]int64{22, 11, 22, 33}, sizes)
	if len(got) != 2 {
		t.Fatalf("photo-size list count = %d, want 2", len(got))
	}
	if got[0].SizeId != 22 || got[1].SizeId != 11 {
		t.Fatalf("photo-size list IDs = [%d %d], want [22 11]", got[0].SizeId, got[1].SizeId)
	}
}
