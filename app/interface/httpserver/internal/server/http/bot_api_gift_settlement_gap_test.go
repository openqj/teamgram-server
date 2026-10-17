package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

func TestGiftSettlementMethodsRequireConfiguredServices(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"user_id":                json.RawMessage("7"),
		"gift_id":                json.RawMessage("101"),
		"star_count":             json.RawMessage("25"),
		"business_connection_id": json.RawMessage(`"business-1"`),
	}}
	for _, method := range []string{"sendGift", "giftPremiumSubscription", "transferBusinessAccountStars"} {
		t.Run(method, func(t *testing.T) {
			result, status, code, description := dispatchBotAPIMethod(
				&svc.ServiceContext{},
				httptest.NewRequest("POST", "/bot/token/"+method, nil),
				bot,
				strings.ToUpper(method),
				request,
			)
			want := "Not Implemented: payment service is not configured"
			if result != nil || status != 501 || code != 501 || description != want {
				t.Fatalf("dispatch = (%#v, %d, %d, %q), want service-config 501 %q", result, status, code, description, want)
			}
		})
	}
}

type giftSettlementPaymentsStub struct {
	mtproto.RPCPaymentsClient
}

type giftSettlementStarsStub struct {
	mtproto.RPCStarsClient
}

func TestGiftSettlementHTTPValidation(t *testing.T) {
	service := &svc.ServiceContext{
		PaymentsClient: &giftSettlementPaymentsStub{},
		StarsClient:    &giftSettlementStarsStub{},
	}
	request := func(values map[string]json.RawMessage) *botAPIRequest {
		return &botAPIRequest{values: values}
	}
	call := httptest.NewRequest("POST", "/bot/token/sendGift", nil)

	t.Run("gift id is parsed from a string", func(t *testing.T) {
		result, status, code, description := sendGift(service, call, 42, request(map[string]json.RawMessage{
			"gift_id": json.RawMessage(`"101"`),
			"user_id": json.RawMessage("42"),
			"text":    json.RawMessage(`"` + strings.Repeat("界", 129) + `"`),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: text is too long" {
			t.Fatalf("sendGift validation = (%#v, %d, %d, %q), want text length rejection", result, status, code, description)
		}
	})

	t.Run("premium star count is required", func(t *testing.T) {
		result, status, code, description := giftPremiumSubscription(service, call, 42, request(map[string]json.RawMessage{
			"user_id":     json.RawMessage("42"),
			"month_count": json.RawMessage("3"),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: star_count is required" {
			t.Fatalf("premium validation = (%#v, %d, %d, %q), want required star_count", result, status, code, description)
		}
	})

	t.Run("premium star count matches months", func(t *testing.T) {
		result, status, code, description := giftPremiumSubscription(service, call, 42, request(map[string]json.RawMessage{
			"user_id":     json.RawMessage("42"),
			"month_count": json.RawMessage("3"),
			"star_count":  json.RawMessage("1500"),
		}))
		if result != nil || status != 400 || code != 400 || description != "Bad Request: star_count does not match month_count" {
			t.Fatalf("premium validation = (%#v, %d, %d, %q), want pricing rejection", result, status, code, description)
		}
	})
}

func TestSavedGiftHTTPFiltersFailClosedWhenLedgerCannotRepresentThem(t *testing.T) {
	for _, name := range []string{
		"exclude_unlimited",
		"exclude_unique",
		"sort_by_value",
		"sort_by_price",
		"exclude_upgradable",
		"exclude_unupgradable",
		"exclude_limited_upgradable",
		"exclude_limited_non_upgradable",
		"exclude_from_blockchain",
	} {
		t.Run(name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				name: json.RawMessage("true"),
			}}
			if _, err := botAPISavedGiftRequest(request); err != errBotAPIGiftFilterUnsupported {
				t.Fatalf("filter error = %v, want %v", err, errBotAPIGiftFilterUnsupported)
			}
		})
	}
}
