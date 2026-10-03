package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type hidePeerSettingsUserClientStub struct {
	userclient.UserClient
	reply   *mtproto.Bool
	err     error
	request *userpb.TLUserDeletePeerSettings
}

type hidePeerSettingsSyncClientStub struct {
	syncclient.SyncClient
	request *syncpb.TLSyncUpdatesNotMe
	err     error
}

func (s *hidePeerSettingsSyncClientStub) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.request = in
	return mtproto.EmptyVoid, s.err
}

func (s *hidePeerSettingsUserClientStub) UserDeletePeerSettings(_ context.Context, in *userpb.TLUserDeletePeerSettings) (*mtproto.Bool, error) {
	s.request = in
	return s.reply, s.err
}

func newHidePeerSettingsTestCore(user userclient.UserClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{UserClient: user},
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func newHidePeerSettingsSyncTestCore(user userclient.UserClient, syncStub syncclient.SyncClient) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Dao: &dao.Dao{UserClient: user, SyncClient: syncStub},
		},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 9001},
	}
}

func TestMessagesHidePeerSettingsBarPropagatesUserServiceFailures(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer()
	in := &mtproto.TLMessagesHidePeerSettingsBar{Peer: peer}

	t.Run("service error", func(t *testing.T) {
		wantErr := errors.New("user service unavailable")
		user := &hidePeerSettingsUserClientStub{err: wantErr}
		got, err := newHidePeerSettingsTestCore(user).MessagesHidePeerSettingsBar(in)
		if !errors.Is(err, wantErr) || got != nil {
			t.Fatalf("MessagesHidePeerSettingsBar() = (%v, %v), want propagated error", got, err)
		}
		if user.request == nil || user.request.UserId != 42 || user.request.PeerType != mtproto.PEER_USER || user.request.PeerId != 84 {
			t.Fatalf("UserDeletePeerSettings request = %+v, want user 42 / peer user 84", user.request)
		}
	})

	t.Run("nil service reply", func(t *testing.T) {
		got, err := newHidePeerSettingsTestCore(&hidePeerSettingsUserClientStub{}).MessagesHidePeerSettingsBar(in)
		if err != mtproto.ErrInternalServerError || got != nil {
			t.Fatalf("MessagesHidePeerSettingsBar() = (%v, %v), want INTERNAL_SERVER_ERROR for nil reply", got, err)
		}
	})

	t.Run("success", func(t *testing.T) {
		user := &hidePeerSettingsUserClientStub{reply: mtproto.BoolFalse}
		got, err := newHidePeerSettingsTestCore(user).MessagesHidePeerSettingsBar(in)
		if err != nil || got != mtproto.BoolFalse {
			t.Fatalf("MessagesHidePeerSettingsBar() = (%v, %v), want service reply passthrough", got, err)
		}
	})
}

func TestMessagesHidePeerSettingsBarSyncsClearedSettings(t *testing.T) {
	syncStub := &hidePeerSettingsSyncClientStub{}
	user := &hidePeerSettingsUserClientStub{reply: mtproto.BoolTrue}
	got, err := newHidePeerSettingsSyncTestCore(user, syncStub).MessagesHidePeerSettingsBar(&mtproto.TLMessagesHidePeerSettingsBar{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: 840}).To_InputPeer(),
	})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("MessagesHidePeerSettingsBar() = (%v, %v), want true", got, err)
	}
	if syncStub.request == nil || syncStub.request.GetUserId() != 42 || syncStub.request.GetPermAuthKeyId() != 9001 {
		t.Fatalf("SyncUpdatesNotMe request = %v, want current user and auth key", syncStub.request)
	}
	updates := syncStub.request.GetUpdates().GetUpdates()
	if len(updates) != 1 || updates[0].GetPredicateName() != mtproto.Predicate_updatePeerSettings {
		t.Fatalf("synced updates = %v, want one updatePeerSettings", updates)
	}
	if updates[0].GetPeer_PEER().GetUserId() != 84 || updates[0].GetSettings() == nil {
		t.Fatalf("updatePeerSettings = %v, want user 84 and empty settings", updates[0])
	}
}

func TestMessagesHidePeerSettingsBarPropagatesSyncFailure(t *testing.T) {
	syncErr := errors.New("sync unavailable")
	syncStub := &hidePeerSettingsSyncClientStub{err: syncErr}
	user := &hidePeerSettingsUserClientStub{reply: mtproto.BoolTrue}
	got, err := newHidePeerSettingsSyncTestCore(user, syncStub).MessagesHidePeerSettingsBar(&mtproto.TLMessagesHidePeerSettingsBar{
		Peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 77}).To_InputPeer(),
	})
	if !errors.Is(err, syncErr) || got != nil {
		t.Fatalf("MessagesHidePeerSettingsBar() = (%v, %v), want propagated sync error", got, err)
	}
}
