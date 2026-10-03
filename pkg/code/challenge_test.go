package code

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	zeroredis "github.com/zeromicro/go-zero/core/stores/redis"
)

type recordingProvider struct {
	mu         sync.Mutex
	deliveries []Delivery
	readyErr   error
	deliverErr error
}

type singleRedisKV struct {
	client *zeroredis.Redis
}

func (s singleRedisKV) EvalCtx(ctx context.Context, script, key string, args ...any) (any, error) {
	return s.client.EvalCtx(ctx, script, []string{key}, args...)
}

func (s singleRedisKV) SetexCtx(ctx context.Context, key, value string, seconds int) error {
	return s.client.SetexCtx(ctx, key, value, seconds)
}

func (s singleRedisKV) DelCtx(ctx context.Context, keys ...string) (int, error) {
	total := 0
	for _, key := range keys {
		removed, err := s.client.DelCtx(ctx, key)
		if err != nil {
			return total, err
		}
		total += removed
	}
	return total, nil
}

func (p *recordingProvider) Ready() error { return p.readyErr }

func (p *recordingProvider) Deliver(_ context.Context, delivery Delivery) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deliveries = append(p.deliveries, delivery)
	return p.deliverErr
}

func (p *recordingProvider) last() Delivery {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.deliveries[len(p.deliveries)-1]
}

func newRedisChallengeTest(t *testing.T, settings ChallengeSettings, sms, email DeliveryProvider) (*ChallengeService, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := zeroredis.New(server.Addr())
	store := NewRedisChallengeStore(singleRedisKV{client: client})
	return NewChallengeService(store, settings, sms, email), server
}

func TestChallengeProviderUnavailableDoesNotIssue(t *testing.T) {
	service, server := newRedisChallengeTest(t, ChallengeSettings{
		TTL: time.Minute, RateLimit: 2, RateWindow: time.Minute, MaxAttempts: 3,
	}, unavailableProvider{}, unavailableProvider{})

	_, err := service.Issue(context.Background(), IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000001",
		Scope: "auth-key", ChallengeID: "unavailable", CodeLength: 5,
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("Issue() error = %v, want provider unavailable", err)
	}
	if server.Exists(challengeKey(ChannelSMS, "login", "auth-key", "unavailable")) {
		t.Fatal("provider-unavailable issue persisted a challenge")
	}
}

func TestChallengeStubDeliveryTTLRateLimitAndSingleConsumption(t *testing.T) {
	provider := &recordingProvider{}
	settings := ChallengeSettings{
		TTL: 2 * time.Minute, RateLimit: 2, RateWindow: 10 * time.Minute,
		MaxAttempts: 3, Secret: "test-secret",
	}
	service, server := newRedisChallengeTest(t, settings, provider, unavailableProvider{})
	ctx := context.Background()

	first, err := service.Issue(ctx, IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000002",
		Scope: "auth-key", ChallengeID: "first", CodeLength: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery := provider.last()
	if delivery.Code != first.Code || len(first.Code) != 6 || delivery.Destination != "+15550000002" {
		t.Fatalf("delivery = %+v, issued = %+v", delivery, first)
	}
	key := challengeKey(ChannelSMS, "login", "auth-key", "first")
	if ttl := server.TTL(key); ttl <= 0 || ttl > settings.TTL {
		t.Fatalf("challenge TTL = %v", ttl)
	}
	raw, err := server.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if contains(raw, first.Code) {
		t.Fatal("stored challenge contains the plaintext code")
	}

	_, err = service.Consume(ctx, VerifyRequest{
		Channel: ChannelSMS, Purpose: "login", Scope: "auth-key",
		ChallengeID: "first", Code: "000000",
	})
	if !errors.Is(err, ErrChallengeInvalid) {
		t.Fatalf("wrong code error = %v", err)
	}
	if _, err = service.Consume(ctx, VerifyRequest{
		Channel: ChannelSMS, Purpose: "login", Scope: "auth-key",
		ChallengeID: "first", Code: first.Code,
	}); err != nil {
		t.Fatalf("correct code: %v", err)
	}
	if _, err = service.Consume(ctx, VerifyRequest{
		Channel: ChannelSMS, Purpose: "login", Scope: "auth-key",
		ChallengeID: "first", Code: first.Code,
	}); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("replay error = %v", err)
	}

	if _, err = service.Issue(ctx, IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000002",
		Scope: "auth-key", ChallengeID: "second", CodeLength: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Issue(ctx, IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000002",
		Scope: "other-auth-key", ChallengeID: "third", CodeLength: 5,
	}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("third issue error = %v, want rate limited", err)
	}
}

func TestChallengeDeliveryFailureRemovesChallenge(t *testing.T) {
	provider := &recordingProvider{deliverErr: ErrDeliveryFailed}
	service, server := newRedisChallengeTest(t, ChallengeSettings{
		TTL: time.Minute, RateLimit: 2, RateWindow: time.Minute, MaxAttempts: 3,
	}, provider, unavailableProvider{})

	_, err := service.Issue(context.Background(), IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000003",
		Scope: "auth-key", ChallengeID: "delivery-failure", CodeLength: 5,
	})
	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("Issue() error = %v", err)
	}
	if server.Exists(challengeKey(ChannelSMS, "login", "auth-key", "delivery-failure")) {
		t.Fatal("failed delivery left a usable challenge")
	}
}

func TestChallengeConcurrentConsumeSucceedsOnce(t *testing.T) {
	provider := &recordingProvider{}
	service, _ := newRedisChallengeTest(t, ChallengeSettings{
		TTL: time.Minute, RateLimit: 2, RateWindow: time.Minute, MaxAttempts: 3,
	}, provider, unavailableProvider{})
	ctx := context.Background()
	issued, err := service.Issue(ctx, IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000004",
		Scope: "auth-key", ChallengeID: "concurrent", CodeLength: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 12
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, consumeErr := service.Consume(ctx, VerifyRequest{
				Channel: ChannelSMS, Purpose: "login", Scope: "auth-key",
				ChallengeID: "concurrent", Code: issued.Code,
			})
			results <- consumeErr
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result == nil {
			successes++
		} else if !errors.Is(result, ErrChallengeNotFound) {
			t.Fatalf("unexpected consume error: %v", result)
		}
	}
	if successes != 1 {
		t.Fatalf("successful consumes = %d, want 1", successes)
	}
}

func TestChallengeExpiresWithRedisTTL(t *testing.T) {
	provider := &recordingProvider{}
	service, server := newRedisChallengeTest(t, ChallengeSettings{
		TTL: 30 * time.Second, RateLimit: 2, RateWindow: time.Minute, MaxAttempts: 3,
	}, provider, unavailableProvider{})
	ctx := context.Background()
	issued, err := service.Issue(ctx, IssueRequest{
		Channel: ChannelSMS, Purpose: "login", Subject: "+15550000005",
		Scope: "auth-key", ChallengeID: "expiry", CodeLength: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	server.FastForward(31 * time.Second)
	if _, err = service.Consume(ctx, VerifyRequest{
		Channel: ChannelSMS, Purpose: "login", Scope: "auth-key",
		ChallengeID: "expiry", Code: issued.Code,
	}); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("expired consume error = %v", err)
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
