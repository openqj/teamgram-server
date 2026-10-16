package http

import (
	"encoding/json"
	"testing"
)

func TestParseBotAPIAcceptedGiftTypes(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"accepted_gift_types": json.RawMessage(`{"unlimited_gifts":true,"limited_gifts":false,"unique_gifts":true,"premium_subscription":false,"gifts_from_channels":true}`),
	}}
	accepted, err := parseBotAPIAcceptedGiftTypes(request)
	if err != nil {
		t.Fatalf("parseBotAPIAcceptedGiftTypes() error = %v", err)
	}
	if !accepted.UnlimitedGifts || accepted.LimitedGifts || !accepted.UniqueGifts || accepted.PremiumSubscription || !accepted.GiftsFromChannels {
		t.Fatalf("accepted gift types = %#v", accepted)
	}
	disallowed := disallowedGiftsFromBotAPIAcceptedTypes(accepted)
	if disallowed == nil || disallowed.GetDisallowUnlimitedStargifts() || !disallowed.GetDisallowLimitedStargifts() || disallowed.GetDisallowUniqueStargifts() || !disallowed.GetDisallowPremiumGifts() || disallowed.GetDisallowStargiftsFromChannels() {
		t.Fatalf("disallowed gift types = %#v", disallowed)
	}
}

func TestParseBotAPIAcceptedGiftTypesRequiresObject(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"missing": nil,
		"null":    json.RawMessage("null"),
		"array":   json.RawMessage("[]"),
	} {
		t.Run(name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{}}
			if raw != nil {
				request.values["accepted_gift_types"] = raw
			}
			if _, err := parseBotAPIAcceptedGiftTypes(request); err == nil {
				t.Fatal("parseBotAPIAcceptedGiftTypes() accepted invalid input")
			}
		})
	}
}

func TestRequiredBotAPIBool(t *testing.T) {
	if _, err := requiredBotAPIBool(&botAPIRequest{values: map[string]json.RawMessage{}}, "show_gift_button"); err == nil {
		t.Fatal("requiredBotAPIBool() accepted omitted value")
	}
	value, err := requiredBotAPIBool(&botAPIRequest{values: map[string]json.RawMessage{
		"show_gift_button": json.RawMessage("false"),
	}}, "show_gift_button")
	if err != nil || value {
		t.Fatalf("requiredBotAPIBool() = (%v, %v), want (false, nil)", value, err)
	}
}
