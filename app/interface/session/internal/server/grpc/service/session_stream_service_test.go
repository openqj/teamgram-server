package service

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
)

func TestSessionStreamRequestKeyUsesPermanentAuthKey(t *testing.T) {
	tests := []struct {
		name string
		req  *session.SessionStreamRequest
		want string
	}{
		{
			name: "send data",
			req: &session.SessionStreamRequest{
				Payload: &session.SessionStreamRequest_SendData{SendData: &session.TLSessionSendDataToSession{
					Data: &session.SessionClientData{PermAuthKeyId: 101},
				}},
			},
			want: "perm:101",
		},
		{
			name: "session updates",
			req: &session.SessionStreamRequest{
				Payload: &session.SessionStreamRequest_PushSessionUpdates{PushSessionUpdates: &session.TLSessionPushSessionUpdatesData{
					PermAuthKeyId: 202,
					AuthKeyId:     303,
					SessionId:     404,
				}},
			},
			want: "perm:202",
		},
		{
			name: "query auth key",
			req: &session.SessionStreamRequest{
				Payload: &session.SessionStreamRequest_QueryAuthKey{QueryAuthKey: &session.TLSessionQueryAuthKey{AuthKeyId: 505}},
			},
			want: "auth:505",
		},
		{
			name: "set auth key",
			req: &session.SessionStreamRequest{
				Payload: &session.SessionStreamRequest_SetAuthKey{SetAuthKey: &session.TLSessionSetAuthKey{
					AuthKey: &mtproto.AuthKeyInfo{AuthKeyId: 606, PermAuthKeyId: 607},
				}},
			},
			want: "perm:607",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sessionStreamRequestKey(test.req); got != test.want {
				t.Fatalf("key=%q, want %q", got, test.want)
			}
		})
	}
}

func TestSessionStreamRequestKeyIsolatesMissingKeys(t *testing.T) {
	first := &session.SessionStreamRequest{RequestId: "one"}
	second := &session.SessionStreamRequest{RequestId: "two"}
	if got := sessionStreamRequestKey(first); got != "request:one" {
		t.Fatalf("request id key=%q", got)
	}
	if got := sessionStreamRequestKey(second); got != "request:two" {
		t.Fatalf("request id key=%q", got)
	}
	withoutID := sessionStreamRequestKey(&session.SessionStreamRequest{})
	if !strings.HasPrefix(withoutID, "request:0x") {
		t.Fatalf("missing-key fallback=%q", withoutID)
	}
}
