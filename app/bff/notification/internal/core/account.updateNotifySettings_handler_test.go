package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/notification/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type updateNotifySettingsKey struct {
	userID   int64
	peerType int32
	peerID   int64
}

type updateNotifyUserClientStub struct {
	userclient.UserClient
	users     *userpb.Vector_ImmutableUser
	usersErr  error
	setResult *mtproto.Bool
	setErr    error
	settings  map[updateNotifySettingsKey]*mtproto.PeerNotifySettings
	setCalls  int
}

func (s *updateNotifyUserClientStub) UserGetMutableUsers(_ context.Context, _ *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, s.usersErr
}

func (s *updateNotifyUserClientStub) UserSetNotifySettings(_ context.Context, in *userpb.TLUserSetNotifySettings) (*mtproto.Bool, error) {
	s.setCalls++
	if s.setErr != nil {
		return nil, s.setErr
	}
	if s.setResult != nil && mtproto.FromBool(s.setResult) {
		if s.settings == nil {
			s.settings = make(map[updateNotifySettingsKey]*mtproto.PeerNotifySettings)
		}
		s.settings[updateNotifySettingsKey{in.UserId, in.PeerType, in.PeerId}] = in.Settings
	}
	return s.setResult, nil
}

func (s *updateNotifyUserClientStub) UserGetNotifySettings(_ context.Context, in *userpb.TLUserGetNotifySettings) (*mtproto.PeerNotifySettings, error) {
	return s.settings[updateNotifySettingsKey{in.UserId, in.PeerType, in.PeerId}], nil
}

type updateNotifySyncClientStub struct {
	syncclient.SyncClient
	request *syncpb.TLSyncUpdatesNotMe
}

func (s *updateNotifySyncClientStub) SyncUpdatesNotMe(_ context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.request = in
	return mtproto.EmptyVoid, nil
}

func TestAccountUpdateNotifySettingsPersistsAndReadsUserPeer(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	userClient := &updateNotifyUserClientStub{
		users:     updateNotifyTestUsers(updateNotifyTestImmutableUser(selfID, false), updateNotifyTestImmutableUser(peerID, false)),
		setResult: mtproto.BoolTrue,
	}
	syncClient := &updateNotifySyncClientStub{}
	core := newUpdateNotifyTestCore(selfID, userClient, syncClient)

	result, err := core.AccountUpdateNotifySettings(updateNotifyTestRequest(peerID))
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("AccountUpdateNotifySettings() = (%v, %v), want BoolTrue", result, err)
	}
	if userClient.setCalls != 1 || syncClient.request == nil {
		t.Fatalf("set calls = %d, sync request = %v; want one persisted write and sync", userClient.setCalls, syncClient.request)
	}

	settings, err := core.AccountGetNotifySettings(&mtproto.TLAccountGetNotifySettings{Peer: updateNotifyTestPeer(peerID)})
	if err != nil {
		t.Fatalf("AccountGetNotifySettings() error = %v", err)
	}
	if settings == nil || settings.GetShowPreviews() == nil || mtproto.FromBool(settings.GetShowPreviews()) {
		t.Fatalf("show previews = %v, want false", settings.GetShowPreviews())
	}
	if settings.GetSilent() == nil || !mtproto.FromBool(settings.GetSilent()) {
		t.Fatalf("silent = %v, want true", settings.GetSilent())
	}
	if settings.GetMuteUntil() == nil || settings.GetMuteUntil().GetValue() != 12345 {
		t.Fatalf("mute until = %v, want 12345", settings.GetMuteUntil())
	}
	if settings.GetSound() == nil || settings.GetSound().GetValue() != "chime" {
		t.Fatalf("sound = %v, want chime", settings.GetSound())
	}
}

func TestAccountUpdateNotifySettingsRejectsInvalidUserPeer(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)

	tests := []struct {
		name  string
		users *userpb.Vector_ImmutableUser
		want  error
	}{
		{name: "nil user response", want: mtproto.ErrPeerIdInvalid},
		{name: "missing peer", users: updateNotifyTestUsers(updateNotifyTestImmutableUser(selfID, false)), want: mtproto.ErrPeerIdInvalid},
		{name: "deleted peer", users: updateNotifyTestUsers(updateNotifyTestImmutableUser(selfID, false), updateNotifyTestImmutableUser(peerID, true)), want: mtproto.ErrInputUserDeactivated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userClient := &updateNotifyUserClientStub{users: tt.users, setResult: mtproto.BoolTrue}
			syncClient := &updateNotifySyncClientStub{}
			core := newUpdateNotifyTestCore(selfID, userClient, syncClient)

			result, err := core.AccountUpdateNotifySettings(updateNotifyTestRequest(peerID))
			if result != nil || !errors.Is(err, tt.want) {
				t.Fatalf("AccountUpdateNotifySettings() = (%v, %v), want nil and %v", result, err, tt.want)
			}
			if userClient.setCalls != 0 || syncClient.request != nil {
				t.Fatalf("set calls = %d, sync request = %v; want no side effects", userClient.setCalls, syncClient.request)
			}
		})
	}
}

func TestAccountUpdateNotifySettingsRequiresWriteConfirmation(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)

	for _, result := range []*mtproto.Bool{nil, mtproto.BoolFalse} {
		userClient := &updateNotifyUserClientStub{
			users:     updateNotifyTestUsers(updateNotifyTestImmutableUser(selfID, false), updateNotifyTestImmutableUser(peerID, false)),
			setResult: result,
		}
		syncClient := &updateNotifySyncClientStub{}
		core := newUpdateNotifyTestCore(selfID, userClient, syncClient)

		got, err := core.AccountUpdateNotifySettings(updateNotifyTestRequest(peerID))
		if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
			t.Fatalf("AccountUpdateNotifySettings() = (%v, %v), want nil and INTERNAL_SERVER_ERROR", got, err)
		}
		if userClient.setCalls != 1 || len(userClient.settings) != 0 || syncClient.request != nil {
			t.Fatalf("set calls = %d, settings = %v, sync request = %v; want rejected write without sync", userClient.setCalls, userClient.settings, syncClient.request)
		}
	}
}

func newUpdateNotifyTestCore(selfID int64, userClient *updateNotifyUserClientStub, syncClient *updateNotifySyncClientStub) *NotificationCore {
	ctx := context.Background()
	return &NotificationCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient, SyncClient: syncClient}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: selfID},
	}
}

func updateNotifyTestRequest(peerID int64) *mtproto.TLAccountUpdateNotifySettings {
	return &mtproto.TLAccountUpdateNotifySettings{
		Peer:     updateNotifyTestPeer(peerID),
		Settings: updateNotifyTestSettings(),
	}
}

func updateNotifyTestPeer(peerID int64) *mtproto.InputNotifyPeer {
	return mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peerID}).To_InputPeer(),
	}).To_InputNotifyPeer()
}

func updateNotifyTestSettings() *mtproto.InputPeerNotifySettings {
	return mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
		ShowPreviews:     mtproto.BoolFalse,
		Silent:           mtproto.BoolTrue,
		MuteUntil:        &wrapperspb.Int32Value{Value: 12345},
		Sound_FLAGSTRING: &wrapperspb.StringValue{Value: "chime"},
	}).To_InputPeerNotifySettings()
}

func updateNotifyTestUsers(users ...*mtproto.ImmutableUser) *userpb.Vector_ImmutableUser {
	return &userpb.Vector_ImmutableUser{Datas: users}
}

func updateNotifyTestImmutableUser(id int64, deleted bool) *mtproto.ImmutableUser {
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: id, FirstName: "User", Deleted: deleted},
	}).To_ImmutableUser()
}
