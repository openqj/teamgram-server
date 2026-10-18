package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/teamgram/teamgram-server/pkg/twofa"
	"github.com/zeromicro/go-zero/core/logx"
)

type recoverPasswordIdentityUserClient struct {
	userclient.UserClient
	user *mtproto.ImmutableUser
}

func (c *recoverPasswordIdentityUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.user, nil
}

type recoverPasswordChallengeStore struct {
	record verification.Challenge
}

func (s *recoverPasswordChallengeStore) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}

func (s *recoverPasswordChallengeStore) Put(_ context.Context, _ string, record verification.Challenge, _ time.Duration) error {
	s.record = record
	return nil
}

func (s *recoverPasswordChallengeStore) Verify(context.Context, string, string, string, int64, int, bool) (*verification.Challenge, error) {
	return &s.record, nil
}

func (s *recoverPasswordChallengeStore) Delete(context.Context, string) error { return nil }

type recoverPasswordDeliveryProvider struct{}

func (recoverPasswordDeliveryProvider) Ready() error { return nil }

func (recoverPasswordDeliveryProvider) Deliver(context.Context, verification.Delivery) error {
	return nil
}

func TestAuthRecoverPasswordRejectsResolvedUserIdentityMismatch(t *testing.T) {
	const userID = int64(712349)
	const code = "123456"

	passwordStore := &memoryPasswordRecoveryStore{values: make(map[string]string)}
	encoded, err := json.Marshal(twofa.PasswordState{Email: "recovery@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	passwordStore.values[acctPasswordKey(userID)] = string(encoded)
	oldStore := persist.Default
	persist.Default = passwordStore
	defer func() { persist.Default = oldStore }()

	challengeStore := &recoverPasswordChallengeStore{}
	challengeService := verification.NewChallengeService(challengeStore, verification.ChallengeSettings{
		TTL: time.Minute, RateLimit: 3, RateWindow: time.Minute, MaxAttempts: 3,
		Secret: "recover-password-test-secret", RequireSecret: false,
	}, nil, recoverPasswordDeliveryProvider{})
	challenge, err := challengeService.Issue(context.Background(), verification.IssueRequest{
		Channel: verification.ChannelEmail, Purpose: challengePurposePasswordRecovery,
		Subject: "recovery@example.test", Scope: verification.ScopeID(userID),
		ChallengeID: verification.PurposeID(challengePurposePasswordRecovery, verification.ScopeID(userID)), Code: code,
	})
	if err != nil {
		t.Fatal(err)
	}
	if challenge == nil {
		t.Fatal("challenge issue returned nil")
	}
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx: ctx, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: &recoverPasswordIdentityUserClient{
			user: &mtproto.ImmutableUser{User: &mtproto.UserData{Id: userID + 1}},
		}}, Challenges: challengeService},
	}
	result, err := c.AuthRecoverPassword(&mtproto.TLAuthRecoverPassword{Code: code})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("recover password = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}
