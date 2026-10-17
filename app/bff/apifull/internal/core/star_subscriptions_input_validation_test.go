package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestStarsSubscriptionMethodsRejectMissingPeerBeforeProvider(t *testing.T) {
	const uid int64 = 903201
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&requests, 1)
	}))
	defer server.Close()

	core := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	zeroHashPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid}).To_InputPeer()
	tests := []struct {
		name string
		call func() (any, error)
	}{
		{
			name: "get nil peer",
			call: func() (any, error) {
				return core.PaymentsGetStarsSubscriptions(&mtproto.TLPaymentsGetStarsSubscriptions{Offset: "0"})
			},
		},
		{
			name: "get zero hash",
			call: func() (any, error) {
				return core.PaymentsGetStarsSubscriptions(&mtproto.TLPaymentsGetStarsSubscriptions{Peer: zeroHashPeer})
			},
		},
		{
			name: "change nil peer",
			call: func() (any, error) {
				return core.PaymentsChangeStarsSubscription(&mtproto.TLPaymentsChangeStarsSubscription{SubscriptionId: "sub", Canceled: mtproto.BoolTrue})
			},
		},
		{
			name: "change zero hash",
			call: func() (any, error) {
				return core.PaymentsChangeStarsSubscription(&mtproto.TLPaymentsChangeStarsSubscription{Peer: zeroHashPeer, SubscriptionId: "sub", Canceled: mtproto.BoolTrue})
			},
		},
		{
			name: "fulfill nil peer",
			call: func() (any, error) {
				return core.PaymentsFulfillStarsSubscription(&mtproto.TLPaymentsFulfillStarsSubscription{SubscriptionId: "sub"})
			},
		},
		{
			name: "fulfill zero hash",
			call: func() (any, error) {
				return core.PaymentsFulfillStarsSubscription(&mtproto.TLPaymentsFulfillStarsSubscription{Peer: zeroHashPeer, SubscriptionId: "sub"})
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("result=(%v, %v), want nil result and PEER_ID_INVALID", got, err)
			}
		})
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("provider request count = %d, want 0", got)
	}
}
