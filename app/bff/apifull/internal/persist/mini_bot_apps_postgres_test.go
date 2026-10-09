package persist

import (
	"os"
	"testing"
	"time"
)

func TestMiniBotPermissionAndWebViewRequestPostgres(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = ClosePostgres()
		Default = previous
	})
	userID, botID := int64(9818001), int64(9818002)
	if err := SetMiniBotPermission(userID, botID, false); err != nil {
		t.Fatal(err)
	}
	if got, err := GetMiniBotPermission(userID, botID); err != nil || got {
		t.Fatalf("default permission = %v, %v", got, err)
	}
	if err := SetMiniBotPermission(userID, botID, true); err != nil {
		t.Fatal(err)
	}
	if got, err := GetMiniBotPermission(userID, botID); err != nil || !got {
		t.Fatalf("updated permission = %v, %v", got, err)
	}
	request := WebViewRequest{
		RequestID: "mini-test-request",
		UserID:    userID,
		BotID:     botID,
		Kind:      "requestWebView",
		Payload:   `{"query_id":"9981","url":"https://example.test/app"}`,
		ExpiresAt:  time.Now().UTC().Add(2 * time.Minute),
	}
	if err := PutWebViewRequest(request); err != nil {
		t.Fatal(err)
	}
	got, err := GetWebViewRequest(userID, request.RequestID)
	if err != nil || got == nil || got.BotID != botID {
		t.Fatalf("request = %+v, %v", got, err)
	}
	if ok, err := ProlongWebViewRequest(userID, request.RequestID, 5*time.Minute); err != nil || !ok {
		t.Fatalf("prolong request = %v, %v", ok, err)
	}
	if ok, err := ProlongWebViewQuery(userID, 9981, 5*time.Minute); err != nil || !ok {
		t.Fatalf("prolong query = %v, %v", ok, err)
	}
	_, _ = Default.(*postgresStore).db.Exec(`DELETE FROM apifull_mini_bot_permission WHERE user_id = $1`, userID)
	_, _ = Default.(*postgresStore).db.Exec(`DELETE FROM apifull_webview_request WHERE user_id = $1`, userID)
}
