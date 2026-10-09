package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/dao"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/sess"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/svc"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
)

func TestSessionPushHandlersReturnActorFailure(t *testing.T) {
	manager := sess.NewMainAuthWrapperManager(&dao.Dao{})
	wrapper := manager.AllocMainAuthWrapper(11, func(authID int64) *sess.MainAuthWrapper {
		return sess.NewMainAuthWrapper(authID, 101, mtproto.AuthStateNormal, nil, 0, manager)
	})
	t.Cleanup(wrapper.Stop)
	core := New(context.Background(), &svc.ServiceContext{MainAuthMgr: manager})
	update := mtproto.MakeTLUpdatesTooLong(nil).To_Updates()
	for _, push := range []func() (*mtproto.Bool, error){
		func() (*mtproto.Bool, error) {
			return core.SessionPushUpdatesData(&session.TLSessionPushUpdatesData{PermAuthKeyId: 11, Updates: update})
		},
		func() (*mtproto.Bool, error) {
			return core.SessionPushSessionUpdatesData(&session.TLSessionPushSessionUpdatesData{PermAuthKeyId: 11, AuthKeyId: 11, SessionId: 22, Updates: update})
		},
		func() (*mtproto.Bool, error) {
			return core.SessionPushRpcResultData(&session.TLSessionPushRpcResultData{PermAuthKeyId: 11, AuthKeyId: 11, SessionId: 22, RpcResultData: []byte("result")})
		},
	} {
		if reply, err := push(); reply != nil || !errors.Is(err, sess.ErrPushNotDelivered) {
			t.Fatalf("undelivered push confirmed: reply=%v err=%v", reply, err)
		}
	}
}

func TestSessionPushHandlersRejectEmptyRequest(t *testing.T) {
	core := New(context.Background(), nil)
	for _, push := range []func() (*mtproto.Bool, error){
		func() (*mtproto.Bool, error) { return core.SessionPushUpdatesData(nil) },
		func() (*mtproto.Bool, error) { return core.SessionPushSessionUpdatesData(nil) },
		func() (*mtproto.Bool, error) { return core.SessionPushRpcResultData(nil) },
	} {
		if reply, err := push(); reply != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
			t.Fatalf("invalid push request accepted: reply=%v err=%v", reply, err)
		}
	}
}
