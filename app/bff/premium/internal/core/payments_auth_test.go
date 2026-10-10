package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/grpc/status"
)

func premiumRPCErrorCode(got, want error) bool {
	return got != nil && want != nil && status.Code(got) == status.Code(want) &&
		status.Convert(got).Message() == status.Convert(want).Message()
}

func TestPaymentMethodsRequireAuthentication(t *testing.T) {
	c := &PremiumCore{MD: &metadata.RpcMetadata{}}
	tests := []struct {
		name string
		call func() error
	}{
		{"assign App Store", func() error {
			_, err := c.PaymentsAssignAppStoreTransaction(nil)
			return err
		}},
		{"assign Play Market", func() error {
			_, err := c.PaymentsAssignPlayMarketTransaction(nil)
			return err
		}},
		{"can purchase store", func() error {
			_, err := c.PaymentsCanPurchaseStore(nil)
			return err
		}},
		{"can purchase Premium", func() error {
			_, err := c.PaymentsCanPurchasePremium(nil)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if !premiumRPCErrorCode(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("err=%v; want AUTH_KEY_UNREGISTERED", err)
			}
		})
	}
}

func TestPaymentMethodsRemainFailClosedAfterAuthentication(t *testing.T) {
	c := &PremiumCore{MD: &metadata.RpcMetadata{UserId: 81020}}
	if result, err := c.PaymentsAssignAppStoreTransaction(nil); result != nil || !premiumRPCErrorCode(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("App Store result=%#v err=%v; want nil and PAYMENT_PROVIDER_INVALID", result, err)
	}
	if result, err := c.PaymentsAssignPlayMarketTransaction(nil); result != nil || !premiumRPCErrorCode(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("Play Market result=%#v err=%v; want nil and PAYMENT_PROVIDER_INVALID", result, err)
	}
	if result, err := c.PaymentsAssignAppStoreTransaction(&mtproto.TLPaymentsAssignAppStoreTransaction{Receipt: []byte("unverified")}); result != nil || !premiumRPCErrorCode(err, mtproto.ErrPaymentUnsupported) {
		t.Fatalf("unverified App Store result=%#v err=%v; want nil and PAYMENT_UNSUPPORTED", result, err)
	}
	if result, err := c.PaymentsAssignPlayMarketTransaction(&mtproto.TLPaymentsAssignPlayMarketTransaction{Receipt: &mtproto.DataJSON{Data: "unverified"}}); result != nil || !premiumRPCErrorCode(err, mtproto.ErrPaymentUnsupported) {
		t.Fatalf("unverified Play Market result=%#v err=%v; want nil and PAYMENT_UNSUPPORTED", result, err)
	}
	if result, err := c.PaymentsCanPurchaseStore(nil); result != nil || !premiumRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("store result=%#v err=%v; want nil and METHOD_NOT_IMPL", result, err)
	}
	if result, err := c.PaymentsCanPurchasePremium(nil); result != nil || !premiumRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("Premium result=%#v err=%v; want nil and METHOD_NOT_IMPL", result, err)
	}
}
