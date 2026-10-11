package dao

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/service/media/internal/dal/dataobject"
)

func TestGetVideoSizeERejectsMalformedMarkup(t *testing.T) {
	for _, sizeType := range []string{"e", "s"} {
		if _, err := getVideoSizeE(&dataobject.VideoSizesDO{SizeType: sizeType, FilePath: "{"}); err == nil {
			t.Fatalf("getVideoSizeE(%q) accepted malformed JSON", sizeType)
		}
	}
}
