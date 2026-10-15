package mq

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
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

func TestSafeCommitBatchPolicyIsLimitedToReceiptBackedSyncPushUpdates(t *testing.T) {
	method := string(proto.MessageName((*sync.TLSyncPushUpdates)(nil)))
	if !safeToBatchCommit("Sync-T", method) {
		t.Fatal("receipt-backed Sync-T updates should allow bounded offset batching")
	}
	ifNotMethod := string(proto.MessageName((*sync.TLSyncPushUpdatesIfNot)(nil)))
	if !safeToBatchCommit("Sync-T", ifNotMethod) {
		t.Fatal("receipt-backed Sync-T pushUpdatesIfNot should allow bounded offset batching")
	}
	for _, item := range [][2]string{{"BotFather-T", method}, {"Sync-T", "TLSyncPushRpcResult"},
		{"Inbox-T", method}, {"BotFather-T", ifNotMethod}, {"Sync-T", "unknown"}} {
		if safeToBatchCommit(item[0], item[1]) {
			t.Fatalf("unexpected batching policy for topic=%q method=%q", item[0], item[1])
		}
	}
}

func TestPermanentAuthKeyFailureIsConsumed(t *testing.T) {
	if !errors.Is(mtproto.ErrAuthKeyUnregistered, mtproto.ErrAuthKeyUnregistered) {
		t.Fatal("auth key error must be comparable with errors.Is")
	}
	if !isPermanentSessionError(mtproto.ErrAuthKeyUnregistered) {
		t.Fatal("AUTH_KEY_UNREGISTERED must be treated as permanent")
	}
	if isPermanentSessionError(mtproto.ErrInternalServerError) {
		t.Fatal("transient/internal errors must remain retryable")
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
