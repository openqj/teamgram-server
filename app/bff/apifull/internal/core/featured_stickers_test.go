package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestGetOldFeaturedStickersRejectsInvalidPage(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 229903}}
	cases := []struct {
		name string
		in   *mtproto.TLMessagesGetOldFeaturedStickers
		want error
	}{
		{name: "negative offset", in: &mtproto.TLMessagesGetOldFeaturedStickers{Offset: -1, Limit: 1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative limit", in: &mtproto.TLMessagesGetOldFeaturedStickers{Offset: 0, Limit: -1}, want: mtproto.ErrLimitInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := c.MessagesGetOldFeaturedStickers(tc.in)
			if !nilRPCResult(result) || !sameRPCErrorCode(err, tc.want) {
				t.Fatalf("result=%v err=%v, want nil result and %v", result, err, tc.want)
			}
		})
	}
}
