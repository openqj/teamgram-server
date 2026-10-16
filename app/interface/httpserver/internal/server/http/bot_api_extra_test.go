package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

func TestBotAPISenderChatMethodsRoute(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"chat_id":        json.RawMessage("-1001234567890"),
		"sender_chat_id": json.RawMessage("-1009876543210"),
	}}
	for _, method := range []string{"banChatSenderChat", "unbanChatSenderChat"} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/bot/token/"+method, nil)
			_, status, code, description := dispatchBotAPIMethod(&svc.ServiceContext{}, r, bot, strings.ToUpper(method), request)
			if status != 501 || code != 501 || description != "Not Implemented: channel service is not configured" {
				t.Fatalf("route status = (%d, %d, %q), want channel service configuration error", status, code, description)
			}
		})
	}
}
