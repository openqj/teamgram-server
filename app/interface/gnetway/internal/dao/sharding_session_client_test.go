package dao

import (
	"testing"

	sessionclient "github.com/teamgram/teamgram-server/app/interface/session/client"
	"github.com/zeromicro/go-zero/core/hash"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestShardingSessionClientRetainsOnlyNodeAfterTransientFailures(t *testing.T) {
	const node = "127.0.0.1:20120"

	client := &ShardingSessionClient{
		dispatcher:   hash.NewConsistentHash(),
		sessions:     map[string]sessionclient.SessionClient{node: nil},
		failCounters: make(map[string]int),
	}
	client.dispatcher.Add(node)

	for range maxNodeFailures {
		err := client.InvokeByKey("auth-key", func(sessionclient.SessionClient) error {
			return status.Error(codes.DeadlineExceeded, "temporary outage")
		})
		if status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("InvokeByKey() error code = %s, want %s", status.Code(err), codes.DeadlineExceeded)
		}
	}

	if _, ok := client.dispatcher.Get("auth-key"); !ok {
		t.Fatal("dispatcher lost its only session node")
	}
	if _, ok := client.sessions[node]; !ok {
		t.Fatal("sessions lost its only session node")
	}
	if got := client.failCounters[node]; got != 0 {
		t.Fatalf("failure counter = %d, want 0", got)
	}
}
