package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	verification "github.com/teamgram/teamgram-server/pkg/code"
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

type missingCodeReporterStub struct {
	readyErr   error
	deliverErr error
	delivery   verification.Delivery
}

func (s *missingCodeReporterStub) Ready() error { return s.readyErr }

func (s *missingCodeReporterStub) Deliver(_ context.Context, delivery verification.Delivery) error {
	s.delivery = delivery
	return s.deliverErr
}

func validAuthAPI() (int32, string) {
	return 100, "0123456789abcdef0123456789abcdef"
}

type tempAuthKeyDropClient struct {
	authsessionclient.AuthsessionClient
	request *authsession.TLAuthsessionDropTempAuthKeys
}

func (s *tempAuthKeyDropClient) AuthsessionDropTempAuthKeys(_ context.Context, request *authsession.TLAuthsessionDropTempAuthKeys) (*mtproto.Bool, error) {
	s.request = request
	return mtproto.BoolTrue, nil
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

func TestAuthCheckPasswordRejectsMissingRuntimeContext(t *testing.T) {
	var nilCore *AuthorizationCore
	if result, err := nilCore.AuthCheckPassword(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("nil check-password core = (%v, %v), want AUTH_KEY_UNREGISTERED", result, err)
	}
	c := newProviderGapCore()
	c.MD.UserId = 91
	if result, err := c.AuthCheckPassword(&mtproto.TLAuthCheckPassword{}); result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing check-password runtime = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}

func TestAuthDropTempAuthKeysUsesAuthsessionProvider(t *testing.T) {
	client := &tempAuthKeyDropClient{}
	c := newProviderGapCore()
	c.svcCtx.Dao.AuthsessionClient = client
	except := []int64{101, 202}

	result, err := c.AuthDropTempAuthKeys(&mtproto.TLAuthDropTempAuthKeys{ExceptAuthKeys: except})
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("drop temporary keys = (%v, %v), want BoolTrue", result, err)
	}
	if client.request == nil || len(client.request.GetExceptAuthKeys()) != len(except) || client.request.GetExceptAuthKeys()[0] != except[0] || client.request.GetExceptAuthKeys()[1] != except[1] {
		t.Fatalf("authsession drop request = %v, want except IDs %v", client.request, except)
	}
}

func TestAuthReportMissingCodeUsesConfiguredProvider(t *testing.T) {
	reporter := &missingCodeReporterStub{}
	c := newProviderGapCore()
	c.svcCtx.MissingCodeReporter = reporter
	c.ctx = context.Background()

	result, err := c.AuthReportMissingCode(&mtproto.TLAuthReportMissingCode{
		PhoneNumber: "+1 415 555 2671", PhoneCodeHash: "challenge", Mnc: "310",
	})
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("report missing code = (%v, %v), want BoolTrue", result, err)
	}
	if reporter.delivery.Channel != verification.ChannelSMS || reporter.delivery.Destination != "14155552671" || reporter.delivery.ChallengeID != "challenge" || reporter.delivery.Purpose != "auth.reportMissingCode" || reporter.delivery.MNC != "310" {
		t.Fatalf("delivery = %+v", reporter.delivery)
	}
}

func TestAuthReportMissingCodePropagatesProviderFailure(t *testing.T) {
	reporter := &missingCodeReporterStub{deliverErr: errors.New("report unavailable")}
	c := newProviderGapCore()
	c.svcCtx.MissingCodeReporter = reporter
	c.ctx = context.Background()

	result, err := c.AuthReportMissingCode(&mtproto.TLAuthReportMissingCode{
		PhoneNumber: "+14155552671", PhoneCodeHash: "challenge",
	})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("report missing code = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
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
