package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/zeromicro/go-zero/core/logx"
)

type invalidateSignInCodesStoreStub struct {
	data       *model.PhoneCodeTransaction
	getErr     error
	deleteErr  error
	deletedKey []string
}

func (s *invalidateSignInCodesStoreStub) GetCachePhoneCode(_ context.Context, _ int64, _ string) (*model.PhoneCodeTransaction, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.data, nil
}

func (s *invalidateSignInCodesStoreStub) DeleteCachePhoneCode(_ context.Context, _ int64, key string) error {
	s.deletedKey = append(s.deletedKey, key)
	return s.deleteErr
}

type invalidateSignInCodesUserStub struct {
	userclient.UserClient
	err error
}

func (s *invalidateSignInCodesUserStub) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return nil, s.err
}

func TestDeleteMatchingSignInCodeRemovesAliases(t *testing.T) {
	store := &invalidateSignInCodesStoreStub{data: &model.PhoneCodeTransaction{
		PhoneNumber:   "+15551234567",
		PhoneCode:     "123456",
		PhoneCodeHash: "hash",
	}}
	if err := deleteMatchingSignInCode(context.Background(), store, 42, "hash", map[string]struct{}{"hash": {}}); err != nil {
		t.Fatalf("deleteMatchingSignInCode() error = %v", err)
	}
	if got, want := store.deletedKey, []string{"hash", "+15551234567"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("deleted keys = %v, want %v", got, want)
	}
}

func TestDeleteMatchingSignInCodePropagatesStoreErrors(t *testing.T) {
	getErr := errors.New("cache unavailable")
	store := &invalidateSignInCodesStoreStub{getErr: getErr}
	if err := deleteMatchingSignInCode(context.Background(), store, 42, "hash", map[string]struct{}{"hash": {}}); !errors.Is(err, getErr) {
		t.Fatalf("read error = %v, want %v", err, getErr)
	}

	deleteErr := errors.New("cache delete failed")
	store = &invalidateSignInCodesStoreStub{
		data:      &model.PhoneCodeTransaction{PhoneCodeHash: "hash"},
		deleteErr: deleteErr,
	}
	if err := deleteMatchingSignInCode(context.Background(), store, 42, "hash", map[string]struct{}{"hash": {}}); !errors.Is(err, deleteErr) {
		t.Fatalf("delete error = %v, want %v", err, deleteErr)
	}
}

type invalidateSignInCodesChallengeStoreStub struct {
	deleted   []string
	deleteErr error
}

func (s *invalidateSignInCodesChallengeStoreStub) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}

func (s *invalidateSignInCodesChallengeStoreStub) Put(context.Context, string, verification.Challenge, time.Duration) error {
	return nil
}

func (s *invalidateSignInCodesChallengeStoreStub) Verify(context.Context, string, string, string, int64, int, bool) (*verification.Challenge, error) {
	return nil, verification.ErrChallengeNotFound
}

func (s *invalidateSignInCodesChallengeStoreStub) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.deleteErr
}

func TestRevokeMatchingSignInCodeChallengesRevokesSMSAndApp(t *testing.T) {
	cache := &invalidateSignInCodesStoreStub{data: &model.PhoneCodeTransaction{
		PhoneCode:     "123456",
		PhoneCodeHash: "challenge-hash",
		PhoneNumber:   "+15551234567",
	}}
	challengeStore := &invalidateSignInCodesChallengeStoreStub{}
	challenges := verification.NewChallengeService(challengeStore, verification.ChallengeSettings{}, nil, nil)
	if err := revokeMatchingSignInCodeChallenges(context.Background(), cache, challenges, 42, "42", []string{"123456"}, map[string]struct{}{"123456": {}}); err != nil {
		t.Fatalf("revokeMatchingSignInCodeChallenges() error = %v", err)
	}
	if got := len(challengeStore.deleted); got != 2 {
		t.Fatalf("revoked challenge keys = %d, want SMS and App keys", got)
	}
}

func TestMatchingSignInCodeChallengesChecksSharedNumericCode(t *testing.T) {
	const (
		scope = "42"
		phone = "+15551234567"
		hash  = "challenge-hash"
		code  = "123456"
	)
	cache := &invalidateSignInCodesStoreStub{data: &model.PhoneCodeTransaction{
		PhoneCodeHash: hash,
		PhoneNumber:   phone,
	}}
	challengeStore := &phoneChallengeStore{}
	challenges := verification.NewChallengeService(challengeStore, verification.ChallengeSettings{
		TTL: time.Minute, RateLimit: 3, RateWindow: time.Minute, MaxAttempts: 3,
		Secret: "invalidate-sign-in-test-secret", RequireSecret: false,
	}, phoneChallengeProvider{}, phoneChallengeProvider{})
	if _, err := challenges.Issue(context.Background(), verification.IssueRequest{
		Channel: verification.ChannelSMS, Purpose: challengePurposeAuthLogin,
		Subject: phone, Scope: scope, ChallengeID: hash, Code: code,
	}); err != nil {
		t.Fatalf("issue shared challenge: %v", err)
	}
	matched, err := matchingSignInCodeChallenges(context.Background(), cache, challenges, 42, scope, []string{phone}, map[string]struct{}{code: {}})
	if err != nil || len(matched) != 1 || matched[0].data.PhoneCodeHash != hash {
		t.Fatalf("numeric code match = (%v, %+v), want one %q transaction", err, matched, hash)
	}
	matched, err = matchingSignInCodeChallenges(context.Background(), cache, challenges, 42, scope, []string{phone}, map[string]struct{}{"000000": {}})
	if err != nil || len(matched) != 0 {
		t.Fatalf("wrong numeric code match = (%v, %+v), want no matches", err, matched)
	}
}

func TestRevokeMatchingSignInCodeChallengesPropagatesProviderError(t *testing.T) {
	wantErr := errors.New("challenge store unavailable")
	cache := &invalidateSignInCodesStoreStub{data: &model.PhoneCodeTransaction{
		PhoneCode:     "123456",
		PhoneCodeHash: "challenge-hash",
	}}
	challengeStore := &invalidateSignInCodesChallengeStoreStub{deleteErr: wantErr}
	challenges := verification.NewChallengeService(challengeStore, verification.ChallengeSettings{}, nil, nil)
	if err := revokeMatchingSignInCodeChallenges(context.Background(), cache, challenges, 42, "42", []string{"123456"}, map[string]struct{}{"123456": {}}); !errors.Is(err, wantErr) {
		t.Fatalf("revokeMatchingSignInCodeChallenges() error = %v, want %v", err, wantErr)
	}
}

func TestAccountInvalidateSignInCodesRejectsInvalidDependencies(t *testing.T) {
	var nilCore *AuthorizationCore
	if result, err := nilCore.AccountInvalidateSignInCodes(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil core = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}

	ctx := context.Background()
	c := &AuthorizationCore{ctx: ctx, MD: &metadata.RpcMetadata{PermAuthKeyId: 42}, svcCtx: &svc.ServiceContext{}, Logger: logx.WithContext(ctx)}
	if result, err := c.AccountInvalidateSignInCodes(&mtproto.TLAccountInvalidateSignInCodes{Codes: []string{"hash"}}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing dao = (%v, %v), want METHOD_NOT_IMPL", result, err)
	}

	c = &AuthorizationCore{ctx: ctx, MD: &metadata.RpcMetadata{PermAuthKeyId: 42}, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}, Logger: logx.WithContext(ctx)}
	if result, err := c.AccountInvalidateSignInCodes(&mtproto.TLAccountInvalidateSignInCodes{Codes: []string{"hash"}}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing challenge provider = (%v, %v), want METHOD_NOT_IMPL", result, err)
	}
}

func TestAccountInvalidateSignInCodesPropagatesUserProviderError(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx: ctx,
		MD:  &metadata.RpcMetadata{PermAuthKeyId: 42, UserId: 7},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: &invalidateSignInCodesUserStub{err: wantErr},
		}},
		Logger: logx.WithContext(ctx),
	}
	result, err := c.AccountInvalidateSignInCodes(&mtproto.TLAccountInvalidateSignInCodes{Codes: []string{"hash"}})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("provider error = (%v, %v), want nil and %v", result, err, wantErr)
	}
}

func TestAccountInvalidateSignInCodesRejectsNilUserProviderResult(t *testing.T) {
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx: ctx,
		MD:  &metadata.RpcMetadata{PermAuthKeyId: 42, UserId: 7},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: &invalidateSignInCodesUserStub{},
		}},
		Logger: logx.WithContext(ctx),
	}

	result, err := c.AccountInvalidateSignInCodes(&mtproto.TLAccountInvalidateSignInCodes{Codes: []string{"hash"}})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil user provider result = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}
