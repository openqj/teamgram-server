package botapi

import "testing"

func TestAllowedUpdateTypesIncludeWebAppQuery(t *testing.T) {
	if !IsValidAllowedUpdateType("web_app_query") {
		t.Fatal("web_app_query must be accepted by allowed_updates")
	}
	if IsValidAllowedUpdateType("unknown_update") {
		t.Fatal("unknown update type must be rejected")
	}
}
