package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/grpc"
)

type suggestedPostForwardClient struct {
	mtproto.RPCMessagesClient
	request *mtproto.TLMessagesForwardMessages
	send    *mtproto.TLMessagesSendMessage
}

func (s *suggestedPostForwardClient) MessagesForwardMessages(_ context.Context, in *mtproto.TLMessagesForwardMessages, _ ...grpc.CallOption) (*mtproto.Updates, error) {
	s.request = in
	return suggestedPostForwardUpdates(), nil
}

func (s *suggestedPostForwardClient) MessagesSendMessage(_ context.Context, in *mtproto.TLMessagesSendMessage, _ ...grpc.CallOption) (*mtproto.Updates, error) {
	s.send = in
	return suggestedPostForwardUpdates(), nil
}

func suggestedPostForwardUpdates() *mtproto.Updates {
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      1,
		PeerId:  mtproto.MakePeerUser(42),
		Message: "forwarded",
	}).To_Message()
	update := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: message}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{Updates: []*mtproto.Update{update}}).To_Updates()
}

func TestBotAPIForwardAndCopyPreserveSuggestedPost(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	for _, copyMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "forwardMessage", true: "copyMessage"}[copyMode], func(t *testing.T) {
			client := &suggestedPostForwardClient{}
			request := &botAPIRequest{values: map[string]json.RawMessage{
				"chat_id":                   json.RawMessage(`"42"`),
				"from_chat_id":              json.RawMessage(`"43"`),
				"message_id":                json.RawMessage(`7`),
				"suggested_post_parameters": json.RawMessage(`{}`),
			}}
			_, status, code, description := forwardMessage(&svc.ServiceContext{MessagesClient: client}, httptest.NewRequest("POST", "/", nil), bot, request, copyMode)
			if status != 200 || code != 0 || description != "" {
				t.Fatalf("forward status = (%d, %d, %q), want success", status, code, description)
			}
			if client.request == nil || client.request.GetSuggestedPost() == nil {
				t.Fatalf("forward request = %v, want suggested_post", client.request)
			}
		})
	}
}

func TestBotAPISendRichMessagePreservesSuggestedPost(t *testing.T) {
	client := &suggestedPostForwardClient{}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"chat_id":                   json.RawMessage(`"42"`),
		"rich_message":              json.RawMessage(`{"html":"hello"}`),
		"suggested_post_parameters": json.RawMessage(`{}`),
	}}
	_, status, code, description := sendRichMessage(&svc.ServiceContext{MessagesClient: client}, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, request)
	if status != 200 || code != 0 || description != "" {
		t.Fatalf("sendRichMessage status = (%d, %d, %q), want success", status, code, description)
	}
	if client.send == nil || client.send.GetSuggestedPost() == nil {
		t.Fatalf("sendRichMessage request = %v, want suggested_post", client.send)
	}
}

func TestBotAPIRejectsUnsupportedSuggestedPostMethods(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"suggested_post_parameters": json.RawMessage(`{}`),
	}}
	if _, status, code, _ := sendMediaGroup(nil, nil, bot, request); status != 501 || code != 501 {
		t.Fatalf("sendMediaGroup suggested post = (%d, %d), want 501", status, code)
	}
	if _, status, code, _ := forwardMessages(nil, nil, bot, request, false); status != 501 || code != 501 {
		t.Fatalf("forwardMessages suggested post = (%d, %d), want 501", status, code)
	}
}

func TestBotAPIRejectsSuggestedPostOnEphemeralSend(t *testing.T) {
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"chat_id":                      json.RawMessage(`"42"`),
		"text":                         json.RawMessage(`"hello"`),
		"ephemeral_message_parameters": json.RawMessage(`{"receiver_user_id":43}`),
		"suggested_post_parameters":    json.RawMessage(`{}`),
	}}
	_, status, code, description := sendMessage(nil, httptest.NewRequest("POST", "/", nil), &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}, request)
	if status != 501 || code != 501 || description != "Not Implemented: ephemeral messages with suggested_post_parameters are not supported" {
		t.Fatalf("ephemeral suggested post = (%d, %d, %q), want explicit 501", status, code, description)
	}
}

func TestBotAPIRejectsSuggestedPostDrafts(t *testing.T) {
	bot := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	request := &botAPIRequest{values: map[string]json.RawMessage{
		"suggested_post_parameters": json.RawMessage(`{}`),
	}}
	for _, draft := range []struct {
		name string
		call func() (any, int, int, string)
	}{
		{name: "text", call: func() (any, int, int, string) {
			return sendMessageDraft(nil, nil, bot, request)
		}},
		{name: "rich", call: func() (any, int, int, string) {
			return sendRichMessageDraft(nil, nil, bot, request)
		}},
	} {
		t.Run(draft.name, func(t *testing.T) {
			_, status, code, description := draft.call()
			if status != 501 || code != 501 || description != "Not Implemented: suggested_post_parameters is not supported by the Layer 229 adapter" {
				t.Fatalf("%s draft suggested post = (%d, %d, %q), want explicit 501", draft.name, status, code, description)
			}
		})
	}
}
