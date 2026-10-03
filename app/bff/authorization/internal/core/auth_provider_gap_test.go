package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func newProviderGapCore() *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 91},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
	}
}

func validAuthAPI() (int32, string) {
	return 100, "0123456789abcdef0123456789abcdef"
}

func TestAuthorizationProviderGapsFailClosed(t *testing.T) {
	apiID, apiHash := validAuthAPI()
	c := newProviderGapCore()

	if result, err := c.AuthImportBotAuthorization(&mtproto.TLAuthImportBotAuthorization{
		ApiId: apiID, ApiHash: apiHash, BotAuthToken: "bot-token",
	}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("bot import = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
	if result, err := c.AuthImportWebTokenAuthorization(&mtproto.TLAuthImportWebTokenAuthorization{
		ApiId: apiID, ApiHash: apiHash, WebAuthToken: "web-token",
	}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("web-token import = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
	if result, err := c.AuthDropTempAuthKeys(&mtproto.TLAuthDropTempAuthKeys{}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("drop temporary keys = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
	if result, err := c.AuthReportMissingCode(&mtproto.TLAuthReportMissingCode{
		PhoneNumber: "+14155552671", PhoneCodeHash: "hash",
	}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("report missing code = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
	if result, err := c.AuthCheckPaidAuth(&mtproto.TLAuthCheckPaidAuth{
		PhoneNumber: "+14155552671", PhoneCodeHash: "hash", FormId: 1,
	}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("check paid auth = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}

func TestAuthorizationProviderGapInputErrors(t *testing.T) {
	c := newProviderGapCore()
	if result, err := c.AuthDropTempAuthKeys(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil drop request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}
	if result, err := c.AuthRequestFirebaseSms(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil Firebase request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}
	if result, err := c.AuthReportMissingCode(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil missing-code request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}
	if result, err := c.AuthCheckPaidAuth(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil paid-auth request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}
	if result, err := c.AuthResetLoginEmail(nil); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil reset-email request = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", result, err)
	}
}

func TestBindTempAuthKeyRejectsMalformedPayloadBeforeProvider(t *testing.T) {
	c := newProviderGapCore()
	c.MD.AuthId = 92
	c.MD.PermAuthKeyId = 91

	if result, err := c.AuthBindTempAuthKey(nil); result != nil || !errors.Is(err, mtproto.ErrEncryptedMessageInvalid) {
		t.Fatalf("nil bind request = (%v, %v), want (nil, ENCRYPTED_MESSAGE_INVALID)", result, err)
	}
	if result, err := c.AuthBindTempAuthKey(&mtproto.TLAuthBindTempAuthKey{PermAuthKeyId: 91, EncryptedMessage: make([]byte, 55)}); result != nil || !errors.Is(err, mtproto.ErrEncryptedMessageInvalid) {
		t.Fatalf("short bind payload = (%v, %v), want (nil, ENCRYPTED_MESSAGE_INVALID)", result, err)
	}
	if result, err := c.AuthBindTempAuthKey(&mtproto.TLAuthBindTempAuthKey{PermAuthKeyId: 91, EncryptedMessage: make([]byte, 56)}); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("unconfigured bind provider = (%v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}

func TestResetLoginEmailFailsClosedWithoutChallengeProvider(t *testing.T) {
	ctx := context.Background()
	c := &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 91},
		svcCtx: &svc.ServiceContext{},
	}
	result, err := c.AuthResetLoginEmail(&mtproto.TLAuthResetLoginEmail{
		PhoneNumber: "+14155552671", PhoneCodeHash: "hash",
	})
	if result != nil || !errors.Is(err, mtproto.ErrSmsCodeCreateFailed) {
		t.Fatalf("reset login email = (%v, %v), want (nil, SMS_CODE_CREATE_FAILED)", result, err)
	}
}
