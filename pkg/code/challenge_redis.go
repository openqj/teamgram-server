package code

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

type challengeKV interface {
	EvalCtx(context.Context, string, string, ...any) (any, error)
	SetexCtx(context.Context, string, string, int) error
	DelCtx(context.Context, ...string) (int, error)
}

type RedisChallengeStore struct {
	kv challengeKV
}

func NewRedisChallengeStore(kv challengeKV, secret ...string) *RedisChallengeStore {
	return &RedisChallengeStore{kv: kv}
}

const rateLimitScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
if count > tonumber(ARGV[2]) then
  return {0, ttl}
end
return {1, ttl}
`

func (s *RedisChallengeStore) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if s == nil || s.kv == nil {
		return false, 0, ErrProviderUnavailable
	}
	if limit <= 0 {
		limit = 1
	}
	seconds := int(math.Ceil(window.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	result, err := s.kv.EvalCtx(ctx, rateLimitScript, key, seconds, limit)
	if err != nil {
		return false, 0, err
	}
	values, err := redisArray(result)
	if err != nil || len(values) < 2 {
		return false, 0, fmt.Errorf("invalid rate-limit response: %v", result)
	}
	allowed, err := redisInt(values[0])
	if err != nil {
		return false, 0, err
	}
	ttl, err := redisInt(values[1])
	if err != nil {
		return false, 0, err
	}
	if ttl < 0 {
		ttl = int64(seconds)
	}
	return allowed == 1, time.Duration(ttl) * time.Second, nil
}

func (s *RedisChallengeStore) Put(ctx context.Context, key string, challenge Challenge, ttl time.Duration) error {
	if s == nil || s.kv == nil {
		return ErrProviderUnavailable
	}
	b, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	seconds := int(math.Ceil(ttl.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return s.kv.SetexCtx(ctx, key, string(b), seconds)
}

const verifyChallengeScript = `
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {0, ''}
end
local value = cjson.decode(raw)
if tonumber(value.expires_at) <= tonumber(ARGV[2]) then
  redis.call('DEL', KEYS[1])
  return {-1, ''}
end
if value.id ~= ARGV[1] then
  return {-2, ''}
end
-- Go performs the constant-time SHA-256 comparison after this atomic read.
-- The script tracks failures and removes exhausted challenges before returning.
if ARGV[3] ~= value.code_digest then
  value.attempts = (tonumber(value.attempts) or 0) + 1
  if value.attempts >= tonumber(ARGV[4]) then
    redis.call('DEL', KEYS[1])
  else
    redis.call('SET', KEYS[1], cjson.encode(value), 'KEEPTTL')
  end
  return {-2, ''}
end
if ARGV[5] == '1' then
  redis.call('DEL', KEYS[1])
end
return {1, raw}
`

func (s *RedisChallengeStore) Verify(ctx context.Context, key, id, digest string, now int64, maxAttempts int, consume bool) (*Challenge, error) {
	if s == nil || s.kv == nil {
		return nil, ErrChallengeNotFound
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	consumeArg := 0
	if consume {
		consumeArg = 1
	}
	result, err := s.kv.EvalCtx(ctx, verifyChallengeScript, key, id, now, digest, maxAttempts, consumeArg)
	if err != nil {
		return nil, err
	}
	values, err := redisArray(result)
	if err != nil || len(values) < 2 {
		return nil, fmt.Errorf("invalid challenge response: %v", result)
	}
	status, err := redisInt(values[0])
	if err != nil {
		return nil, err
	}
	switch status {
	case 1:
		raw, err := redisString(values[1])
		if err != nil {
			return nil, err
		}
		var challenge Challenge
		if err = json.Unmarshal([]byte(raw), &challenge); err != nil {
			return nil, err
		}
		return &challenge, nil
	case -1:
		return nil, ErrChallengeExpired
	case -2:
		return nil, ErrChallengeInvalid
	default:
		return nil, ErrChallengeNotFound
	}
}

func (s *RedisChallengeStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.kv == nil {
		return nil
	}
	_, err := s.kv.DelCtx(ctx, key)
	return err
}

func redisArray(value any) ([]any, error) {
	switch v := value.(type) {
	case []any:
		return v, nil
	default:
		return nil, fmt.Errorf("expected redis array, got %T", value)
	}
}

func redisInt(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case uint64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("expected redis integer, got %T", value)
	}
}

func redisString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("expected redis string, got %T", value)
	}
}
