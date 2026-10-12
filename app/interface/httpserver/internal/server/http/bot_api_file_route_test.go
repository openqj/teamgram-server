package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/router"
)

func TestBotAPIFileRouteSplitsTelegramFilePath(t *testing.T) {
	var got botAPIFilePath
	routes := router.NewRouter()
	if err := routes.Handle(http.MethodGet, "/file/:token/:kind/:file_name", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := httpx.ParsePath(r, &got); err != nil {
			t.Fatalf("ParsePath() error = %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/file/bot123:secret/document/42_7.dat", nil)
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("route status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got.Token != "bot123:secret" || got.Kind != "document" || got.FileName != "42_7.dat" {
		t.Fatalf("route params = %#v, want canonical Telegram file path", got)
	}
}
