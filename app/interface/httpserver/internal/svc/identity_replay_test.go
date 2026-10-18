package svc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestNewProofReplayStoreRequiresProtectedRedisNode(t *testing.T) {
	if _, err := NewProofReplayStore(nil); err == nil {
		t.Fatal("expected an empty replay configuration to be rejected")
	}
	if _, err := NewProofReplayStore(kv.KvConf{{RedisConf: redis.RedisConf{Host: ""}, Weight: 100}}); err == nil {
		t.Fatal("expected an empty Redis host to be rejected")
	}
}

func TestProofReplayStoreClaimIsAtomicAgainstRedis(t *testing.T) {
	host := os.Getenv("TEAMGRAM_IDENTITY_REPLAY_REDIS_HOST")
	if host == "" {
		t.Skip("set TEAMGRAM_IDENTITY_REPLAY_REDIS_HOST to run the Redis integration check")
	}
	store, err := NewProofReplayStore(kv.KvConf{{RedisConf: redis.RedisConf{Host: host}, Weight: 100}})
	if err != nil {
		t.Fatal(err)
	}
	authKeyID := time.Now().UnixNano()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, err := store.Claim(ctx, authKeyID, 1, 2*time.Second)
	if err != nil || !first {
		t.Fatalf("first proof claim = %v, %v; want true", first, err)
	}
	second, err := store.Claim(ctx, authKeyID, 1, 2*time.Second)
	if err != nil || second {
		t.Fatalf("replayed proof claim = %v, %v; want false", second, err)
	}
}
