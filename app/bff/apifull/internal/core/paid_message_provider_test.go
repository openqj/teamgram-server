package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestAccountGetPaidMessagesRevenueUsesVerifiedProvider(t *testing.T) {
	const callerID int64 = 981703
	const targetID int64 = 981704
	parent := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID, AccessHash: targetID + 100}).To_InputPeer()
	target := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID, AccessHash: targetID + 100}).To_InputUser()
	var request struct {
		Operation    string             `json:"operation"`
		CallerUserID int64              `json:"caller_user_id"`
		UserID       int64              `json:"user_id"`
		ParentPeer   *mtproto.InputPeer `json:"parent_peer"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
			return
		}
		writeSignedPaymentProviderResponse(t, w, paidMessagesRevenueProviderResponse{
			Verified: true, CallerUserID: callerID, UserID: targetID, StarsAmount: 37,
		})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{
			PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey,
		}},
		MD: &metadata.RpcMetadata{UserId: callerID},
	}
	got, err := c.AccountGetPaidMessagesRevenue(&mtproto.TLAccountGetPaidMessagesRevenue{ParentPeer: parent, UserId: target})
	if err != nil || got == nil || got.GetStarsAmount() != 37 {
		t.Fatalf("revenue = (%#v, %v)", got, err)
	}
	if request.Operation != "get_paid_messages_revenue" || request.CallerUserID != callerID || request.UserID != targetID || request.ParentPeer == nil || request.ParentPeer.GetUserId() != targetID {
		t.Fatalf("provider request = %+v", request)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("revenue did not encode: %v", err)
	}
}

func TestAccountGetPaidMessagesRevenueRejectsMalformedProviderData(t *testing.T) {
	const callerID int64 = 981705
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, paidMessagesRevenueProviderResponse{
			Verified: true, CallerUserID: callerID, UserID: callerID, StarsAmount: -1,
		})
	}))
	defer server.Close()
	c := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{
			PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey,
		}},
		MD: &metadata.RpcMetadata{UserId: callerID},
	}
	target := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	got, err := c.AccountGetPaidMessagesRevenue(&mtproto.TLAccountGetPaidMessagesRevenue{UserId: target})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("revenue = (%#v, %v), want provider-invalid refusal", got, err)
	}
}
