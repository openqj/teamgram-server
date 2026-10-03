package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type resetNotifyUserClientStub struct {
	userclient.UserClient
	result *mtproto.Bool
	err    error
}

func (s *resetNotifyUserClientStub) UserResetNotifySettings(context.Context, *userpb.TLUserResetNotifySettings) (*mtproto.Bool, error) {
	return s.result, s.err
}

func TestAccountResetNotifySettingsPropagatesUserError(t *testing.T) {
	wantErr := errors.New("user notify settings unavailable")
	c := New(context.Background(), &svc.ServiceContext{
		Dao: &dao.Dao{UserClient: &resetNotifyUserClientStub{err: wantErr}},
	})
	c.MD = &metadata.RpcMetadata{UserId: 42}

	got, err := c.AccountResetNotifySettings(&mtproto.TLAccountResetNotifySettings{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("AccountResetNotifySettings() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("AccountResetNotifySettings() result = %v, want nil on error", got)
	}
}

func TestAccountResetNotifySettingsPreservesBackendResult(t *testing.T) {
	c := New(context.Background(), &svc.ServiceContext{
		Dao: &dao.Dao{UserClient: &resetNotifyUserClientStub{result: mtproto.BoolFalse}},
	})
	c.MD = &metadata.RpcMetadata{UserId: 42}

	got, err := c.AccountResetNotifySettings(&mtproto.TLAccountResetNotifySettings{})
	if err != nil {
		t.Fatalf("AccountResetNotifySettings() error = %v, want nil", err)
	}
	if got != mtproto.BoolFalse {
		t.Fatalf("AccountResetNotifySettings() result = %v, want BoolFalse", got)
	}
}
