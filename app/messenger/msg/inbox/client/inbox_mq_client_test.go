package inboxclient

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type testProducer func(context.Context, string, string, []byte) (int32, int64, error)

func (f testProducer) SendMessageV2(ctx context.Context, method, key string, data []byte) (int32, int64, error) {
	return f(ctx, method, key, data)
}

func TestInboxMqClientPreservesDeliveryContract(t *testing.T) {
	request := &inbox.TLInboxSendUserMessageToInboxV2{UserId: 9007199254740993, FromId: 101, PeerId: 202, ClientReqMsgId: wrapperspb.Int64(9007199254740995)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fail bool
	client := NewInboxMqClient(testProducer(func(got context.Context, method, key string, data []byte) (int32, int64, error) {
		if got != ctx || method != string(proto.MessageName(request)) || key != "9007199254740993" {
			t.Fatal("Inbox MQ context, method, or recipient key changed")
		}
		decoded := new(inbox.TLInboxSendUserMessageToInboxV2)
		if err := json.Unmarshal(data, decoded); err != nil || !proto.Equal(decoded, request) {
			t.Fatalf("Inbox MQ delivery payload changed: %v", err)
		}
		if fail {
			return -1, -1, context.DeadlineExceeded
		}
		return 0, 1, nil
	}))
	if result, err := client.InboxSendUserMessageToInboxV2(ctx, request); err != nil || result == nil {
		t.Fatalf("Inbox MQ acknowledged publication = %v, %v", result, err)
	}
	fail = true
	if result, err := client.InboxSendUserMessageToInboxV2(ctx, request); !errors.Is(err, context.DeadlineExceeded) || result != nil {
		t.Fatalf("Inbox MQ failed publication = %v, %v", result, err)
	}
}
