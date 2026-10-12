package botapi

import (
	"strings"
	"testing"
	"time"
)

func TestValidateWebAppInitDataTelegramVector(t *testing.T) {
	initData := "auth_date=1700000000&query_id=demo-query&user=%7B%22id%22%3A1%7D&hash=e26567ba6b79bf04837f37c65e099d56bd9951cc0d6fda57ad26b03225a71f91"
	values, err := ValidateWebAppInitData(initData, "web-e2e-token", time.Unix(1791839207, 0), 0)
	if err != nil {
		t.Fatalf("ValidateWebAppInitData() error = %v", err)
	}
	if got := values.Get("query_id"); got != "demo-query" {
		t.Fatalf("query_id = %q, want demo-query", got)
	}
}

func TestValidateWebAppInitDataRejectsExpiredOrTamperedData(t *testing.T) {
	initData := "auth_date=1700000000&query_id=demo-query&user=%7B%22id%22%3A1%7D&hash=e26567ba6b79bf04837f37c65e099d56bd9951cc0d6fda57ad26b03225a71f91"
	if _, err := ValidateWebAppInitData(initData, "web-e2e-token", time.Unix(1791839207, 0), 24*time.Hour); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired initData error = %v, want expiration error", err)
	}
	tampered := strings.Replace(initData, "demo-query", "tampered", 1)
	if _, err := ValidateWebAppInitData(tampered, "web-e2e-token", time.Unix(1791839207, 0), 0); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered initData error = %v, want hash mismatch", err)
	}
}
