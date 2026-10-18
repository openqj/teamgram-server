// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBotAPIEphemeralParameters(t *testing.T) {
	tests := []struct {
		name    string
		raw     json.RawMessage
		want    *botAPIEphemeralParameters
		wantErr string
	}{
		{name: "omitted", want: nil},
		{name: "null", raw: json.RawMessage(`null`), want: nil},
		{
			name: "valid",
			raw:  json.RawMessage(`{"receiver_user_id":42,"callback_query_id":"abc","replace_callback_query_message":true}`),
			want: &botAPIEphemeralParameters{
				ReceiverUserID:              42,
				CallbackQueryID:             "abc",
				ReplaceCallbackQueryMessage: true,
			},
		},
		{name: "malformed", raw: json.RawMessage(`{"receiver_user_id":`), wantErr: "ephemeral_message_parameters is invalid"},
		{name: "missing receiver", raw: json.RawMessage(`{"callback_query_id":"abc"}`), wantErr: "ephemeral_message_parameters.receiver_user_id is invalid"},
		{name: "replace without callback is valid shape", raw: json.RawMessage(`{"receiver_user_id":42,"replace_callback_query_message":true}`), want: &botAPIEphemeralParameters{ReceiverUserID: 42, ReplaceCallbackQueryMessage: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{}}
			if test.raw != nil {
				request.values["ephemeral_message_parameters"] = test.raw
			}
			got, err := botAPIEphemeralParametersValue(request)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("parameters error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parameters error = %v", err)
			}
			if test.want == nil {
				if got != nil {
					t.Fatalf("parameters = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.ReceiverUserID != test.want.ReceiverUserID || got.CallbackQueryID != test.want.CallbackQueryID || got.ReplaceCallbackQueryMessage != test.want.ReplaceCallbackQueryMessage {
				t.Fatalf("parameters = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBotAPIEphemeralUnsupportedDeliveryFlags(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "callback query", raw: `{"receiver_user_id":42,"callback_query_id":"abc"}`, want: errBotAPIEphemeralCallbackUnsupported.Error()},
		{name: "replace callback message", raw: `{"receiver_user_id":42,"replace_callback_query_message":true}`, want: errBotAPIEphemeralReplaceUnsupported.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				"ephemeral_message_parameters": json.RawMessage(test.raw),
			}}
			_, status, code, description := sendBotAPIEphemeral(nil, nil, 1, request, mtproto.PEER_CHAT, 2, "text", nil, nil, nil, nil, nil)
			if status != http.StatusNotImplemented || code != 501 || !strings.Contains(description, test.want) {
				t.Fatalf("unsupported flag = (%d, %d, %q), want 501 containing %q", status, code, description, test.want)
			}
		})
	}
}

func TestBotAPIMessageTopID(t *testing.T) {
	forum := &botAPIRequest{values: map[string]json.RawMessage{
		"message_thread_id": json.RawMessage(`123`),
	}}
	topID, err := botAPIMessageTopID(forum)
	if err != nil || topID == nil || topID.GetValue() != 123 {
		t.Fatalf("forum topic = %#v, %v; want 123", topID, err)
	}

	malformed := &botAPIRequest{values: map[string]json.RawMessage{
		"message_thread_id": json.RawMessage(`"bad"`),
	}}
	if _, err := botAPIMessageTopID(malformed); err == nil || err.Error() != "message_thread_id is invalid" {
		t.Fatalf("malformed forum topic error = %v, want validation error", err)
	}

	for _, test := range []struct {
		name       string
		raw        string
		wantErr    error
		wantStatus int
	}{
		{name: "representable direct topic", raw: `2147483647`, wantErr: errBotAPIDirectTopicUnsupported, wantStatus: http.StatusNotImplemented},
		{name: "overflow direct topic", raw: `2147483648`, wantErr: errBotAPIDirectTopicOverflow, wantStatus: http.StatusNotImplemented},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := &botAPIRequest{values: map[string]json.RawMessage{
				"direct_messages_topic_id": json.RawMessage(test.raw),
			}}
			got, err := botAPIMessageTopID(request)
			if got != nil || err == nil || err != test.wantErr {
				t.Fatalf("direct topic = %#v, %v; want nil and %v", got, err, test.wantErr)
			}
			status, code, description := botAPIMessageTopIDError(err)
			if status != test.wantStatus || code != 501 || !strings.Contains(description, "direct_messages_topic_id") {
				t.Fatalf("direct topic response = (%d, %d, %q), want explicit 501", status, code, description)
			}
		})
	}
}
