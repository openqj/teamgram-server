package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

func TestBotAPIProfileCatalogMethodsRoute(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	for _, method := range []string{"getForumTopicIconStickers", "getUserPersonalChatMessages"} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/bot/token/"+method, nil)
			request := &botAPIRequest{values: map[string]json.RawMessage{}}
			if method == "getUserPersonalChatMessages" {
				request.values["user_id"] = json.RawMessage("100")
			}
			_, status, code, description := dispatchBotAPIMethod(&svc.ServiceContext{}, r, bot, strings.ToUpper(method), request)
			wantDescription := "Not Implemented: custom emoji service is not configured"
			if method == "getUserPersonalChatMessages" {
				wantDescription = "Bad Request: limit is required"
			}
			if method == "getForumTopicIconStickers" && (status != 501 || code != 501 || description != wantDescription) {
				t.Fatalf("route status = (%d, %d, %q), want custom emoji configuration error", status, code, description)
			}
			if method == "getUserPersonalChatMessages" && (status != 400 || code != 400 || description != wantDescription) {
				t.Fatalf("route status = (%d, %d, %q), want missing limit error", status, code, description)
			}
		})
	}
}

func TestBotAPIUserPersonalChatMessagesLimit(t *testing.T) {
	base := func(limit string) *botAPIRequest {
		return &botAPIRequest{values: map[string]json.RawMessage{
			"user_id": json.RawMessage("100"),
			"limit":   json.RawMessage(limit),
		}}
	}
	for _, test := range []struct {
		name  string
		input *botAPIRequest
		want  string
	}{
		{name: "zero", input: base("0"), want: "Bad Request: limit is invalid"},
		{name: "above maximum", input: base("21"), want: "Bad Request: limit is invalid"},
		{name: "invalid", input: base(`"many"`), want: "Bad Request: limit is invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, status, code, description := getUserPersonalChatMessages(&svc.ServiceContext{}, httptest.NewRequest("POST", "/", nil), 42, test.input)
			if status != 400 || code != 400 || description != test.want {
				t.Fatalf("result = (%d, %d, %q), want (%d, %d, %q)", status, code, description, 400, 400, test.want)
			}
		})
	}
}
