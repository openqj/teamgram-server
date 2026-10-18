package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
)

func TestLocalStarsInvoiceRejectsPremiumGiftWithoutAccessHash(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	invoice := mtproto.MakeTLInputInvoicePremiumGiftStars(&mtproto.InputInvoice{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43}).To_InputUser(),
		Months: 3,
	}).To_InputInvoice()

	_, err := core.localStarsInvoice(invoice)
	if !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("premium gift invoice error = %v, want USER_ID_INVALID", err)
	}
}

func TestLocalStarsInvoiceRejectsBusinessTransferWithoutAccessHash(t *testing.T) {
	ctx := businessctx.WithScope(context.Background(), businessctx.Scope{ConnectionID: "test", OwnerID: 99, CanTransferStars: true})
	core := &ApiFullCore{ctx: ctx, MD: &metadata.RpcMetadata{UserId: 42}}
	invoice := mtproto.MakeTLInputInvoiceBusinessBotTransferStars(&mtproto.InputInvoice{
		Bot:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 42}).To_InputUser(),
		Stars: 25,
	}).To_InputInvoice()

	_, err := core.localStarsInvoice(invoice)
	if !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("business transfer invoice error = %v, want USER_ID_INVALID", err)
	}
}
