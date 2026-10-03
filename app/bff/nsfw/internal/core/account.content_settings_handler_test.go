package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/nsfw/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/nsfw/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type contentSettingsUserClientStub struct {
	userclient.UserClient
	setResult *mtproto.Bool
	setErr    error
	getResult *mtproto.Account_ContentSettings
	getErr    error
}

func (s *contentSettingsUserClientStub) UserSetContentSettings(context.Context, *userpb.TLUserSetContentSettings) (*mtproto.Bool, error) {
	return s.setResult, s.setErr
}

func (s *contentSettingsUserClientStub) UserGetContentSettings(context.Context, *userpb.TLUserGetContentSettings) (*mtproto.Account_ContentSettings, error) {
	return s.getResult, s.getErr
}

func newContentSettingsTestCore(stub *contentSettingsUserClientStub) *NsfwCore {
	ctx := context.Background()
	return &NsfwCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 229001},
	}
}

func TestAccountSetContentSettingsRequiresWriteAcknowledgement(t *testing.T) {
	core := newContentSettingsTestCore(&contentSettingsUserClientStub{setResult: mtproto.BoolFalse})
	result, err := core.AccountSetContentSettings(&mtproto.TLAccountSetContentSettings{})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("false write acknowledgement = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}

func TestAccountGetContentSettingsRejectsNilProviderResponse(t *testing.T) {
	core := newContentSettingsTestCore(&contentSettingsUserClientStub{})
	result, err := core.AccountGetContentSettings(&mtproto.TLAccountGetContentSettings{})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil read response = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}
