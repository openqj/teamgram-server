package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
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
