package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/twofa"
	"github.com/zeromicro/go-zero/core/logx"
)

type memoryPasswordRecoveryStore struct {
	values map[string]string
	writes int
}

func (s *memoryPasswordRecoveryStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *memoryPasswordRecoveryStore) Set(key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	s.writes++
	return nil
}

type memoryRecoveryChallengeStore struct {
	records []verification.Challenge
}

func (s *memoryRecoveryChallengeStore) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}

func (s *memoryRecoveryChallengeStore) Put(_ context.Context, _ string, challenge verification.Challenge, _ time.Duration) error {
	s.records = append(s.records, challenge)
	return nil
}

func (s *memoryRecoveryChallengeStore) Verify(context.Context, string, string, string, int64, int, bool) (*verification.Challenge, error) {
	return nil, verification.ErrChallengeNotFound
}

func (s *memoryRecoveryChallengeStore) Delete(context.Context, string) error { return nil }

type recordingRecoveryProvider struct {
	deliveries []verification.Delivery
}

func (*recordingRecoveryProvider) Ready() error { return nil }

func (p *recordingRecoveryProvider) Deliver(_ context.Context, delivery verification.Delivery) error {
	p.deliveries = append(p.deliveries, delivery)
	return nil
}

func recoveryChallenges(store verification.ChallengeStore, email verification.DeliveryProvider) *verification.ChallengeService {
	return verification.NewChallengeService(store, verification.ChallengeSettings{
		TTL: time.Minute, RateLimit: 3, RateWindow: time.Minute, MaxAttempts: 3,
	}, nil, email)
}

func TestAuthRequestPasswordRecoveryFailsClosedWithoutMailDelivery(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      twofa.PasswordState
		wantWrites int
	}{
		{
			name: "does not create code",
			state: twofa.PasswordState{
				Email: "recovery@example.test",
			},
		},
		{
			name: "revokes legacy local code",
			state: twofa.PasswordState{
				Email:          "recovery@example.test",
				RecoveryCode:   "12345",
				RecoveryExpire: 9999999999,
			},
			wantWrites: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			passwordStore := &memoryPasswordRecoveryStore{values: make(map[string]string)}
			encoded, err := json.Marshal(tc.state)
			if err != nil {
				t.Fatal(err)
			}
			const userID = int64(712345)
			passwordStore.values[acctPasswordKey(userID)] = string(encoded)

			oldStore := persist.Default
			persist.Default = passwordStore
			defer func() { persist.Default = oldStore }()

			ctx := context.Background()
			challengeStore := &memoryRecoveryChallengeStore{}
			c := &AuthorizationCore{
				ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: userID},
				svcCtx: &svc.ServiceContext{Challenges: recoveryChallenges(challengeStore, nil)},
			}
			response, err := c.AuthRequestPasswordRecovery(&mtproto.TLAuthRequestPasswordRecovery{})
			if response != nil || err != mtproto.ErrSendCodeUnavailable {
				t.Fatalf("request recovery = (%v, %v), want (nil, SEND_CODE_UNAVAILABLE)", response, err)
			}
			if passwordStore.writes != tc.wantWrites {
				t.Fatalf("writes = %d, want %d", passwordStore.writes, tc.wantWrites)
			}
			if len(challengeStore.records) != 0 {
				t.Fatal("provider-unavailable request persisted a challenge")
			}

			var saved twofa.PasswordState
			if err = json.Unmarshal([]byte(passwordStore.values[acctPasswordKey(userID)]), &saved); err != nil {
				t.Fatal(err)
			}
			if saved.RecoveryCode != "" || saved.RecoveryExpire != 0 {
				t.Fatalf("recovery code remains usable after request failure: code=%q expiry=%d", saved.RecoveryCode, saved.RecoveryExpire)
			}
		})
	}
}

func TestAuthRequestPasswordRecoveryDeliversSharedChallenge(t *testing.T) {
	const userID = int64(712346)
	passwordStore := &memoryPasswordRecoveryStore{values: make(map[string]string)}
	encoded, err := json.Marshal(twofa.PasswordState{Email: "recovery@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	passwordStore.values[acctPasswordKey(userID)] = string(encoded)
	oldStore := persist.Default
	persist.Default = passwordStore
	defer func() { persist.Default = oldStore }()

	challengeStore := &memoryRecoveryChallengeStore{}
	provider := &recordingRecoveryProvider{}
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Challenges: recoveryChallenges(challengeStore, provider)},
	}
	response, err := c.AuthRequestPasswordRecovery(&mtproto.TLAuthRequestPasswordRecovery{})
	if err != nil || response == nil {
		t.Fatalf("request recovery = (%v, %v)", response, err)
	}
	if len(provider.deliveries) != 1 || len(challengeStore.records) != 1 {
		t.Fatalf("deliveries = %d, records = %d", len(provider.deliveries), len(challengeStore.records))
	}
	delivery := provider.deliveries[0]
	record := challengeStore.records[0]
	if delivery.Destination != "recovery@example.test" || len(delivery.Code) != 6 {
		t.Fatalf("delivery = %+v", delivery)
	}
	if record.CodeDigest == "" || record.CodeDigest == delivery.Code {
		t.Fatalf("challenge persisted plaintext or empty digest: %+v", record)
	}
}

func TestLegacyLocalRecoveryCodeCannotAuthorizeRecovery(t *testing.T) {
	const userID = int64(712347)
	passwordStore := &memoryPasswordRecoveryStore{values: make(map[string]string)}
	encoded, err := json.Marshal(twofa.PasswordState{
		Email:          "recovery@example.test",
		RecoveryCode:   "12345",
		RecoveryExpire: 9999999999,
	})
	if err != nil {
		t.Fatal(err)
	}
	passwordStore.values[acctPasswordKey(userID)] = string(encoded)
	oldStore := persist.Default
	persist.Default = passwordStore
	defer func() { persist.Default = oldStore }()

	ctx := context.Background()
	c := &AuthorizationCore{
		ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Challenges: recoveryChallenges(&memoryRecoveryChallengeStore{}, nil)},
	}
	result, err := c.AuthCheckRecoveryPassword(&mtproto.TLAuthCheckRecoveryPassword{Code: "12345"})
	if result != nil || err != mtproto.ErrPasswordRecoveryExpired {
		t.Fatalf("legacy local recovery code = (%v, %v), want (nil, PASSWORD_RECOVERY_EXPIRED)", result, err)
	}
}

func TestPasswordRecoveryFailsClosedWithoutChallengeService(t *testing.T) {
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: 712348},
		svcCtx: &svc.ServiceContext{},
	}
	if result, err := c.AuthRequestPasswordRecovery(&mtproto.TLAuthRequestPasswordRecovery{}); result != nil || err != mtproto.ErrSendCodeUnavailable {
		t.Fatalf("request = (%v, %v), want (nil, SEND_CODE_UNAVAILABLE)", result, err)
	}
	if result, err := c.AuthCheckRecoveryPassword(&mtproto.TLAuthCheckRecoveryPassword{Code: "123456"}); result != nil || err != mtproto.ErrPasswordRecoveryExpired {
		t.Fatalf("check = (%v, %v), want (nil, PASSWORD_RECOVERY_EXPIRED)", result, err)
	}
	if result, err := c.AuthRecoverPassword(&mtproto.TLAuthRecoverPassword{Code: "123456"}); result != nil || err != mtproto.ErrPasswordRecoveryExpired {
		t.Fatalf("recover = (%v, %v), want (nil, PASSWORD_RECOVERY_EXPIRED)", result, err)
	}
}
