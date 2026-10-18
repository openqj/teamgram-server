package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBotPaymentDeliveryReceiptIsStableAndScoped(t *testing.T) {
	updates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateBotShippingQuery(&mtproto.Update{
		QueryId_INT64:   990001,
		UserId:          990002,
		Payload_BYTES:   []byte("invoice"),
		ShippingAddress: mtproto.MakeTLPostAddress(&mtproto.PostAddress{CountryIso2: "US"}).To_PostAddress(),
	}).To_Update())
	first, ok := botPaymentDeliveryReceipt(updates)
	if !ok {
		t.Fatal("shipping query did not produce a delivery receipt")
	}
	second, ok := botPaymentDeliveryReceipt(updates)
	if !ok || first.ConsumerGroup != second.ConsumerGroup || first.Topic != second.Topic || first.Partition != second.Partition || first.Offset != second.Offset || string(first.RequestHash) != string(second.RequestHash) {
		t.Fatalf("receipt is not stable: first=%+v second=%+v", first, second)
	}

	precheckout, ok := botPaymentDeliveryReceipt(mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateBotPrecheckoutQuery(&mtproto.Update{
		QueryId_INT64:                 990001,
		UserId:                        990002,
		Currency:                      "USD",
		TotalAmount:                   100,
		Payload_BYTES:                 []byte("invoice"),
		Info_FLAGPAYMENTREQUESTEDINFO: mtproto.MakeTLPaymentRequestedInfo(&mtproto.PaymentRequestedInfo{}).To_PaymentRequestedInfo(),
	}).To_Update()))
	if !ok || precheckout.Topic == first.Topic || precheckout.Offset != first.Offset || string(precheckout.RequestHash) == string(first.RequestHash) {
		t.Fatalf("receipt is not event scoped: shipping=%+v precheckout=%+v", first, precheckout)
	}

	ordinary := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{
		Peer_PEER: mtproto.MakePeerUser(990002),
		Settings:  mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings(),
	}).To_Update())
	if _, ok := botPaymentDeliveryReceipt(ordinary); ok {
		t.Fatal("ordinary update unexpectedly received a payment delivery receipt")
	}
	mixed := mtproto.MakeUpdatesByUpdates(updates.GetUpdates()[0], ordinary.GetUpdates()[0])
	if _, ok := botPaymentDeliveryReceipt(mixed); ok {
		t.Fatal("mixed batch unexpectedly received a payment delivery receipt")
	}
}

func TestStripWebViewDataDedupeMarkersBeforeSessionFanout(t *testing.T) {
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Id:          17,
		PeerId:      mtproto.MakePeerUser(42),
		FromId:      mtproto.MakePeerUser(42),
		Action:      mtproto.MakeMessageActionWebViewDataSentMe("Open", "payload"),
		SavedPeerId: mtproto.MakePeerUser(1001),
	}).To_Message()
	updates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: message}).To_Update())
	stripWebViewDataDedupeMarkers(updates)
	if marker := updates.GetUpdates()[0].GetMessage_MESSAGE().GetSavedPeerId(); marker != nil {
		t.Fatalf("web app data random_id marker leaked to session update: %#v", marker)
	}
}
