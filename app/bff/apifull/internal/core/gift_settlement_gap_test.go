package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestStarsSettlementRejectsGiftInvoiceConstructors(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	tests := []struct {
		name    string
		invoice *mtproto.InputInvoice
	}{
		{
			name:    "star gift",
			invoice: mtproto.MakeTLInputInvoiceStarGift(&mtproto.InputInvoice{GiftId: 101}).To_InputInvoice(),
		},
		{
			name:    "premium gift",
			invoice: mtproto.MakeTLInputInvoicePremiumGiftStars(&mtproto.InputInvoice{UserId: mtproto.MakeTLInputUserSelf(nil).To_InputUser(), Months: 3}).To_InputInvoice(),
		},
		{
			name:    "business stars transfer",
			invoice: mtproto.MakeTLInputInvoiceBusinessBotTransferStars(&mtproto.InputInvoice{Bot: mtproto.MakeTLInputUserSelf(nil).To_InputUser(), Stars: 25}).To_InputInvoice(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := c.PaymentsSendStarsForm(&mtproto.TLPaymentsSendStarsForm{FormId: 1, Invoice: tc.invoice})
			if result != nil || !errors.Is(err, mtproto.ErrPaymentUnsupported) {
				t.Fatalf("settlement = (%#v, %v), want nil result and PAYMENT_UNSUPPORTED", result, err)
			}
		})
	}
}

func TestSavedStarGiftsRejectChatPeer(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 42}}
	chat := mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 99}).To_InputPeer()
	result, err := c.PaymentsGetSavedStarGifts(&mtproto.TLPaymentsGetSavedStarGifts{Peer: chat})
	if result != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("chat saved gifts = (%#v, %v), want nil result and PEER_ID_INVALID", result, err)
	}
}
