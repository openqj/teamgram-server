package mq

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"google.golang.org/protobuf/proto"
)

func TestHandleMessageDispatchesPushVariants(t *testing.T) {
	cases := []struct {
		name   string
		method string
		value  string
	}{
		{
			name:   "push updates if not",
			method: string(proto.MessageName((*sync.TLSyncPushUpdatesIfNot)(nil))),
			value:  `{"user_id":1,"updates":{}}`,
		},
		{
			name:   "push bot updates",
			method: string(proto.MessageName((*sync.TLSyncPushBotUpdates)(nil))),
			value:  `{"user_id":1,"updates":{}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handleMessage(context.Background(), nil, tc.method, "1", []byte(tc.value))
			if err == nil || strings.Contains(err.Error(), "invalid Kafka method") {
				t.Fatalf("message was not handled or its error was swallowed: %v", err)
			}
		})
	}
}

func TestHandleMessageRejectsMalformedRequests(t *testing.T) {
	for _, request := range []proto.Message{
		&sync.TLSyncUpdatesMe{}, &sync.TLSyncUpdatesNotMe{}, &sync.TLSyncPushUpdates{},
		&sync.TLSyncPushUpdatesIfNot{}, &sync.TLSyncPushBotUpdates{}, &sync.TLSyncPushRpcResult{}, &sync.TLSyncBroadcastUpdates{},
	} {
		t.Run(string(proto.MessageName(request)), func(t *testing.T) {
			for _, value := range []string{"null", "{}", "{"} {
				if err := handleMessage(context.Background(), nil, string(proto.MessageName(request)), "1", []byte(value)); err == nil {
					t.Fatalf("invalid request %q returned success", value)
				}
			}
		})
	}
}
