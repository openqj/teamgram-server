package botapi

import (
	"net/url"
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

func TestSignWebAppInitDataRoundTrip(t *testing.T) {
	fields := url.Values{
		"auth_date": {"1700000000"},
		"query_id":  {"demo-query"},
		"user":      {`{"id":1}`},
	}
	initData, err := SignWebAppInitData(fields, "web-e2e-token")
	if err != nil {
		t.Fatalf("SignWebAppInitData() error = %v", err)
	}
	values, err := ValidateWebAppInitData(initData, "web-e2e-token", time.Unix(1791839207, 0), 0)
	if err != nil {
		t.Fatalf("signed initData did not validate: %v", err)
	}
	if got := values.Get("query_id"); got != "demo-query" {
		t.Fatalf("query_id = %q, want demo-query", got)
	}
	if got := values.Get("hash"); len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
}

func TestSignWebAppInitDataRejectsAmbiguousFields(t *testing.T) {
	if _, err := SignWebAppInitData(url.Values{"user": {"{}", "{}"}}, "web-e2e-token"); err == nil {
		t.Fatal("duplicate init data field was accepted")
	}
	if _, err := SignWebAppInitData(url.Values{"hash": {"forged"}}, "web-e2e-token"); err == nil {
		t.Fatal("caller-supplied hash was accepted")
	}
}

func TestAddWebAppInitDataPreservesURLAndRejectsDuplicate(t *testing.T) {
	got, err := AddWebAppInitData("https://mini.example/app?start=demo#fragment", "auth_date=1&hash=abc")
	if err != nil {
		t.Fatalf("AddWebAppInitData() error = %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Fragment != "fragment" || parsed.Query().Get("start") != "demo" || parsed.Query().Get("tgWebAppData") != "auth_date=1&hash=abc" {
		t.Fatalf("URL = %q, lost existing URL or init data", got)
	}
	if _, err := AddWebAppInitData(got, "auth_date=2&hash=def"); err == nil {
		t.Fatal("duplicate tgWebAppData was accepted")
	}
	if _, err := AddWebAppInitData("http://mini.example/app", "auth_date=1&hash=abc"); err == nil {
		t.Fatal("insecure Web App URL was accepted")
	}
}
