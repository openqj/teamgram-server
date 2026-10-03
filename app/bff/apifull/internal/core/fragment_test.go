package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAccountReorderUsernames(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.AccountReorderUsernames(&mtproto.TLAccountReorderUsernames{Order: []string{"prod-frag"}}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("reorder=(%#v, %v), want METHOD_NOT_IMPL", got, err)
	}
	if got, err := c.FragmentGetCollectibleInfo(nil); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("collectible=(%#v, %v), want METHOD_NOT_IMPL", got, err)
	}
}

func TestFragmentToggleUsernameFailsClosed(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 3}}
	ok, err := c.AccountToggleUsername(&mtproto.TLAccountToggleUsername{
		Username: "frag-name",
		Active:   mtproto.BoolTrue,
	})
	if ok != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("toggle=(%#v, %v), want METHOD_NOT_IMPL", ok, err)
	}
}
