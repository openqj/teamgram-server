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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGiveawayInfoUsesVerifiedPaymentProvider(t *testing.T) {
	const uid int64 = 902010
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Operation string             `json:"operation"`
			UserID    int64              `json:"user_id"`
			Peer      *mtproto.InputPeer `json:"peer"`
			MsgID     int32              `json:"msg_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode giveaway request: %v", err)
			return
		}
		if request.Operation != "get_giveaway_info" || request.UserID != uid || request.Peer == nil || request.MsgID != 19 {
			t.Errorf("provider request = %+v", request)
		}
		writeSignedPaymentProviderResponse(t, w, giveawayInfoProviderResponse{
			Verified: true,
			UserID:   uid,
			Info:     &mtproto.Payments_GiveawayInfo{Participating: true, StartDate: 123},
		})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetGiveawayInfo(&mtproto.TLPaymentsGetGiveawayInfo{Peer: peer, MsgId: 19})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_payments_giveawayInfo || !got.GetParticipating() || got.GetStartDate() != 123 {
		t.Fatalf("giveaway info = (%#v, %v)", got, err)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("giveaway info did not encode: %v", err)
	}
}

func TestGiveawayInfoRejectsMalformedProviderData(t *testing.T) {
	const uid int64 = 902011
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSignedPaymentProviderResponse(t, w, giveawayInfoProviderResponse{
			Verified: true,
			UserID:   uid,
			Info:     &mtproto.Payments_GiveawayInfo{Participating: true, Winner: true},
		})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetGiveawayInfo(&mtproto.TLPaymentsGetGiveawayInfo{Peer: peer, MsgId: 19})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("malformed giveaway info = (%#v, %v), want provider-invalid refusal", got, err)
	}
}

func TestGiveawayInfoRejectsNegativeStarsPrize(t *testing.T) {
	const uid int64 = 902012
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		info := &mtproto.Payments_GiveawayInfo{
			PredicateName: mtproto.Predicate_payments_giveawayInfoResults,
			StartDate:     123,
			StarsPrize:    wrapperspb.Int64(-1),
		}
		writeSignedPaymentProviderResponse(t, w, giveawayInfoProviderResponse{Verified: true, UserID: uid, Info: info})
	}))
	defer server.Close()

	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Config: config.Config{PaymentProviderEndpoint: server.URL, PaymentProviderSigningKey: paymentProviderTestSigningKey}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	got, err := c.PaymentsGetGiveawayInfo(&mtproto.TLPaymentsGetGiveawayInfo{Peer: peer, MsgId: 19})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentProviderInvalid) {
		t.Fatalf("negative stars prize = (%#v, %v), want provider-invalid refusal", got, err)
	}
}
