package svc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/stores/kv"
)

var ErrIdentityReplayStoreUnavailable = errors.New("Teamgram Identity proof replay store is unavailable")

// ProofReplayStore atomically claims a short-lived proof. Implementations are
// shared across Teamgram instances so a proof cannot be exchanged twice after
// a load balancer retry or failover.
type ProofReplayStore interface {
	Claim(context.Context, int64, int64, time.Duration) (bool, error)
}

type redisProofReplayStore struct {
	store kv.Store
}

func NewProofReplayStore(conf kv.KvConf) (ProofReplayStore, error) {
	if len(conf) == 0 {
		return nil, errors.New("Identity replay KV is required")
	}
	for index := range conf {
		if conf[index].Type == "" {
			conf[index].Type = "node"
		}
	}
	for _, node := range conf {
		if node.Host == "" || node.Weight <= 0 {
			return nil, errors.New("Identity replay KV contains an invalid Redis node")
		}
		if err := node.RedisConf.Validate(); err != nil {
			return nil, fmt.Errorf("Identity replay KV Redis node is invalid: %w", err)
		}
	}
	return &redisProofReplayStore{store: kv.NewStore(conf)}, nil
}

func (s *redisProofReplayStore) Claim(ctx context.Context, authKeyID, pingID int64, ttl time.Duration) (bool, error) {
	if s == nil || s.store == nil {
		return false, ErrIdentityReplayStoreUnavailable
	}
	if authKeyID <= 0 || pingID <= 0 || ttl <= 0 {
		return false, errors.New("invalid Identity proof replay claim")
	}
	if ttl > 5*time.Minute {
		ttl = 5 * time.Minute
	}
	seconds := int(ttl / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	key := fmt.Sprintf("identity:aimemory:proof:%d:%d", authKeyID, pingID)
	claimed, err := s.store.SetnxExCtx(ctx, key, "1", seconds)
	if err != nil {
		return false, fmt.Errorf("claim Identity proof replay key: %w", err)
	}
	return claimed, nil
}
