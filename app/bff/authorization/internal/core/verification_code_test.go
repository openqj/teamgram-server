package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	verification "github.com/teamgram/teamgram-server/pkg/code"
)

type phoneChallengeStore struct {
	mu      sync.Mutex
	records map[string]verification.Challenge
}

func (s *phoneChallengeStore) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}

func (s *phoneChallengeStore) Put(_ context.Context, key string, challenge verification.Challenge, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]verification.Challenge)
	}
	s.records[key] = challenge
	return nil
}

func (s *phoneChallengeStore) Verify(_ context.Context, key, id, _ string, now int64, _ int, consume bool) (*verification.Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	if !ok {
		return nil, verification.ErrChallengeNotFound
	}
	if record.ExpiresAt <= now {
		delete(s.records, key)
		return nil, verification.ErrChallengeExpired
	}
	if record.ID != id {
		return nil, verification.ErrChallengeInvalid
	}
	if consume {
		delete(s.records, key)
	}
	return &record, nil
}

func (s *phoneChallengeStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
	return nil
}

type phoneChallengeProvider struct{}

func (phoneChallengeProvider) Ready() error { return nil }

func (phoneChallengeProvider) Deliver(context.Context, verification.Delivery) error { return nil }

func TestConsumePhoneChallengeBindsSubjectBeforeConsume(t *testing.T) {
	store := &phoneChallengeStore{}
	service := verification.NewChallengeService(store, verification.ChallengeSettings{
		TTL: 2 * time.Minute, RateLimit: 10, RateWindow: time.Minute, MaxAttempts: 3,
	}, phoneChallengeProvider{}, phoneChallengeProvider{})
	c := &AuthorizationCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 91},
		svcCtx: &svc.ServiceContext{Challenges: service},
	}
	const (
		phone = "+15551234567"
		id    = "login-challenge"
		code  = "12345"
	)
	if _, err := service.Issue(context.Background(), verification.IssueRequest{
		Channel: verification.ChannelSMS, Purpose: challengePurposeAuthLogin,
		Subject: phone, Scope: c.challengeScope(), ChallengeID: id, Code: code,
	}); err != nil {
		t.Fatal(err)
	}

	if err := c.consumePhoneChallenge("+15557654321", id, code, challengePurposeAuthLogin); !errors.Is(err, mtproto.ErrPhoneCodeInvalid) {
		t.Fatalf("wrong subject error = %v, want PHONE_CODE_INVALID", err)
	}
	if err := c.consumePhoneChallenge(phone, id, code, challengePurposeAuthLogin); err != nil {
		t.Fatalf("correct subject consume = %v", err)
	}
}

func TestConsumeEmailLoginChallengeUsesPurposeBoundID(t *testing.T) {
	store := &phoneChallengeStore{}
	service := verification.NewChallengeService(store, verification.ChallengeSettings{
		TTL: 2 * time.Minute, RateLimit: 10, RateWindow: time.Minute, MaxAttempts: 3,
	}, phoneChallengeProvider{}, phoneChallengeProvider{})
	c := &AuthorizationCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 92},
		svcCtx: &svc.ServiceContext{Challenges: service},
	}
	const (
		phone = "+15551234567"
		hash  = "phone-code-hash"
		code  = "123456"
	)
	challengeID := verification.PurposeID(mtproto.Predicate_emailVerifyPurposeLoginSetup, phone, hash)
	if _, err := service.Issue(context.Background(), verification.IssueRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposeVerifyEmail,
		Subject: "login@example.com", Scope: c.challengeScope(), ChallengeID: challengeID, Code: code,
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.consumeEmailLoginChallenge(phone, hash, code); err != nil {
		t.Fatalf("consume email login challenge = %v", err)
	}
	if err := c.consumeEmailLoginChallenge(phone, hash, code); !errors.Is(err, mtproto.ErrEmailVerifyExpired) {
		t.Fatalf("replay error = %v, want EMAIL_VERIFY_EXPIRED", err)
	}
}
