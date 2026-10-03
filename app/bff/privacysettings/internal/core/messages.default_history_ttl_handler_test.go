package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type defaultHistoryTTLUserClientStub struct {
	userclient.UserClient
	setErr     error
	getResult  *mtproto.DefaultHistoryTTL
	getErr     error
	setRequest *userpb.TLUserSetDefaultHistoryTTL
}

func (s *defaultHistoryTTLUserClientStub) UserSetDefaultHistoryTTL(_ context.Context, in *userpb.TLUserSetDefaultHistoryTTL) (*mtproto.Bool, error) {
	s.setRequest = in
	if s.setErr != nil {
		return nil, s.setErr
	}
	return mtproto.BoolTrue, nil
}

func (s *defaultHistoryTTLUserClientStub) UserGetDefaultHistoryTTL(context.Context, *userpb.TLUserGetDefaultHistoryTTL) (*mtproto.DefaultHistoryTTL, error) {
	return s.getResult, s.getErr
}

func newDefaultHistoryTTLTestCore(stub *defaultHistoryTTLUserClientStub) *PrivacySettingsCore {
	return &PrivacySettingsCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}},
		MD:     &metadata.RpcMetadata{UserId: 229001},
		Logger: logx.WithContext(context.Background()),
	}
}

func TestMessagesSetDefaultHistoryTTLPropagatesWriteFailure(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	core := newDefaultHistoryTTLTestCore(&defaultHistoryTTLUserClientStub{setErr: wantErr})
	result, err := core.MessagesSetDefaultHistoryTTL(&mtproto.TLMessagesSetDefaultHistoryTTL{Period: 3600})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("set default history TTL = (%v, %v), want nil and %v", result, err, wantErr)
	}
}

func TestMessagesSetDefaultHistoryTTLRejectsInvalidPeriod(t *testing.T) {
	core := newDefaultHistoryTTLTestCore(&defaultHistoryTTLUserClientStub{})
	result, err := core.MessagesSetDefaultHistoryTTL(&mtproto.TLMessagesSetDefaultHistoryTTL{Period: 366*86400 + 1})
	if result != nil || err != mtproto.ErrTtlPeriodInvalid {
		t.Fatalf("invalid period = (%v, %v), want TTL_PERIOD_INVALID", result, err)
	}
}

func TestMessagesGetDefaultHistoryTTLRejectsNilProviderResponse(t *testing.T) {
	core := newDefaultHistoryTTLTestCore(&defaultHistoryTTLUserClientStub{})
	result, err := core.MessagesGetDefaultHistoryTTL(&mtproto.TLMessagesGetDefaultHistoryTTL{})
	if result != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("nil provider response = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}
