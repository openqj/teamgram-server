package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestStarGiftCollectionsFailClosedWithoutCollectionStore(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81011}}
	result, err := c.PaymentsCreateStarGiftCollection(&mtproto.TLPaymentsCreateStarGiftCollection{Title: "aurora"})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("create = (%#v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}

	listed, err := c.PaymentsGetStarGiftCollections(nil)
	if listed != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("list = (%#v, %v), want (nil, METHOD_NOT_IMPL)", listed, err)
	}
}
