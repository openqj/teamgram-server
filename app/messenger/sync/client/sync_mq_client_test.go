package sync_client

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"google.golang.org/protobuf/proto"
)

type testProducer func(context.Context, string, string, []byte) (int32, int64, error)

func (f testProducer) SendMessageV2(ctx context.Context, method, key string, data []byte) (int32, int64, error) {
	return f(ctx, method, key, data)
}

func TestSyncMqClientPreservesUpdateContract(t *testing.T) {
	request := &sync.TLSyncUpdatesNotMe{UserId: 9007199254740993, PermAuthKeyId: 9007199254740995}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fail bool
	client := NewSyncMqClient(testProducer(func(got context.Context, method, key string, data []byte) (int32, int64, error) {
		if got != ctx || method != string(proto.MessageName(request)) || key != "9007199254740993" {
			t.Fatal("Sync MQ context, method, or recipient key changed")
		}
		decoded := new(sync.TLSyncUpdatesNotMe)
		if err := json.Unmarshal(data, decoded); err != nil || !proto.Equal(decoded, request) {
			t.Fatalf("Sync MQ update payload changed: %v", err)
		}
		if fail {
			return -1, -1, context.Canceled
		}
		return 0, 1, nil
	}))
	if result, err := client.SyncUpdatesNotMe(ctx, request); err != nil || result == nil {
		t.Fatalf("Sync MQ acknowledged publication = %v, %v", result, err)
	}
	fail = true
	if result, err := client.SyncUpdatesNotMe(ctx, request); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("Sync MQ failed publication = %v, %v", result, err)
	}
}
