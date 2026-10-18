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

func TestAddWebAppInitDataWritesOfficialHashAndClearsStaleFields(t *testing.T) {
	got, err := AddWebAppInitData("https://mini.example/app?start=demo&tgWebAppData=old&tgWebAppStartParam=old#fragment?keep=yes&tgWebAppData=old&tgWebAppStartParam=old", "auth_date=1&hash=abc")
	if err != nil {
		t.Fatalf("AddWebAppInitData() error = %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("start") != "demo" || parsed.Query().Get("tgWebAppData") != "" || parsed.Query().Get("tgWebAppStartParam") != "" {
		t.Fatalf("URL query = %q, stale Telegram fields were retained", parsed.RawQuery)
	}
	values := parseWebAppHashValues(t, parsed)
	if values.Get("keep") != "yes" || values.Get("tgWebAppData") != "auth_date=1&hash=abc" || values.Get("tgWebAppStartParam") != "" {
		t.Fatalf("URL hash = %q, lost existing fields or init data", parsed.EscapedFragment())
	}
	if strings.Contains(parsed.EscapedFragment(), "tgWebAppData=old") {
		t.Fatalf("stale hash init data was retained: %q", parsed.EscapedFragment())
	}

	replaced, err := AddWebAppInitData(got, "auth_date=2&hash=def")
	if err != nil {
		t.Fatalf("replacing Web App init data: %v", err)
	}
	if got := parseWebAppHashValues(t, mustParseWebAppURL(t, replaced)).Get("tgWebAppData"); got != "auth_date=2&hash=def" {
		t.Fatalf("replaced tgWebAppData = %q", got)
	}
	if _, err := AddWebAppInitData("http://mini.example/app", "auth_date=1&hash=abc"); err == nil {
		t.Fatal("insecure Web App URL was accepted")
	}
}

func TestAddWebAppInitDataPreservesPlainFragmentAndHashQuery(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		path string
	}{
		{name: "plain fragment", url: "https://mini.example/app#fragment", path: "fragment"},
		{name: "existing hash query", url: "https://mini.example/app#fragment?view=compact", path: "fragment"},
		{name: "query-only hash", url: "https://mini.example/app#view=compact", path: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := AddWebAppInitData(test.url, "auth_date=1&hash=abc")
			if err != nil {
				t.Fatalf("AddWebAppInitData() error = %v", err)
			}
			parsed := mustParseWebAppURL(t, got)
			path, _, _ := splitWebAppHash(parsed.EscapedFragment())
			if path != test.path {
				t.Fatalf("hash path = %q, want %q", path, test.path)
			}
			if got := parseWebAppHashValues(t, parsed).Get("tgWebAppData"); got != "auth_date=1&hash=abc" {
				t.Fatalf("tgWebAppData = %q", got)
			}
		})
	}
}

func TestAddWebAppStartParamWritesHashAndClearsStaleFields(t *testing.T) {
	got, err := AddWebAppStartParam("https://mini.example/app?tgWebAppStartParam=old#fragment?tgWebAppData=data&tgWebAppStartParam=old&keep=yes", "launch")
	if err != nil {
		t.Fatalf("AddWebAppStartParam() error = %v", err)
	}
	parsed := mustParseWebAppURL(t, got)
	if parsed.Query().Get("tgWebAppStartParam") != "" {
		t.Fatalf("stale query start param retained: %q", parsed.RawQuery)
	}
	values := parseWebAppHashValues(t, parsed)
	if values.Get("tgWebAppData") != "data" || values.Get("tgWebAppStartParam") != "launch" || values.Get("keep") != "yes" {
		t.Fatalf("hash values = %v", values)
	}
}

func TestAddWebAppStartParamKeepsQueryOnlyHashCanonical(t *testing.T) {
	got, err := AddWebAppStartParam("https://mini.example/app#tgWebAppData=data", "launch")
	if err != nil {
		t.Fatalf("AddWebAppStartParam() error = %v", err)
	}
	parsed := mustParseWebAppURL(t, got)
	if strings.HasPrefix(parsed.EscapedFragment(), "?") {
		t.Fatalf("query-only hash gained a leading question mark: %q", parsed.EscapedFragment())
	}
	values := parseWebAppHashValues(t, parsed)
	if values.Get("tgWebAppData") != "data" || values.Get("tgWebAppStartParam") != "launch" {
		t.Fatalf("hash values = %v", values)
	}
}

func parseWebAppHashValues(t *testing.T, parsed *url.URL) url.Values {
	t.Helper()
	_, query, _ := splitWebAppHash(parsed.EscapedFragment())
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse hash query %q: %v", query, err)
	}
	return values
}

func mustParseWebAppURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	return parsed
}
