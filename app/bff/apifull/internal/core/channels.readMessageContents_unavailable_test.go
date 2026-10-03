package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestChannelsReadMessageContentsRejectsMalformedRequest(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	result, err := c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{})
	if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("ChannelsReadMessageContents() = (%v, %v), want nil result and INPUT_REQUEST_INVALID", result, err)
	}
}
