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
)

type globalPrivacyUserClientStub struct {
	userclient.UserClient
	getResult    *mtproto.GlobalPrivacySettings
	getErr       error
	getRequest   *userpb.TLUserGetGlobalPrivacySettings
	setResult    *mtproto.Bool
	setErr       error
	setRequest   *userpb.TLUserSetGlobalPrivacySettings
	setCallCount int
}

func (s *globalPrivacyUserClientStub) UserGetGlobalPrivacySettings(_ context.Context, in *userpb.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	s.getRequest = in
	return s.getResult, s.getErr
}

func (s *globalPrivacyUserClientStub) UserSetGlobalPrivacySettings(_ context.Context, in *userpb.TLUserSetGlobalPrivacySettings) (*mtproto.Bool, error) {
	s.setCallCount++
	s.setRequest = in
	return s.setResult, s.setErr
}

func newGlobalPrivacyTestCore(stub *globalPrivacyUserClientStub) *PrivacySettingsCore {
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}})
	c.MD = &metadata.RpcMetadata{UserId: 229001}
	return c
}

func testGlobalPrivacySettings() *mtproto.GlobalPrivacySettings {
	return mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: true,
		KeepArchivedUnmuted:                          true,
		KeepArchivedFolders:                          false,
		HideReadMarks:                                true,
		NewNoncontactPeersRequirePremium:             true,
	}).To_GlobalPrivacySettings()
}

func TestAccountGlobalPrivacySettingsSetAndGet(t *testing.T) {
	settings := testGlobalPrivacySettings()
	stub := &globalPrivacyUserClientStub{setResult: mtproto.BoolTrue}
	core := newGlobalPrivacyTestCore(stub)

	got, err := core.AccountSetGlobalPrivacySettings(&mtproto.TLAccountSetGlobalPrivacySettings{Settings: settings})
	if err != nil {
		t.Fatalf("AccountSetGlobalPrivacySettings() error = %v", err)
	}
	if got != settings {
		t.Fatalf("AccountSetGlobalPrivacySettings() returned %p, want input %p", got, settings)
	}
	if stub.setCallCount != 1 || stub.setRequest == nil {
		t.Fatalf("set call count/request = %d/%v, want one request", stub.setCallCount, stub.setRequest)
	}
	if stub.setRequest.GetUserId() != 229001 || stub.setRequest.GetSettings() != settings {
		t.Fatalf("set request = %+v, want user 229001 and supplied settings", stub.setRequest)
	}

	stub.getResult = settings
	got, err = core.AccountGetGlobalPrivacySettings(&mtproto.TLAccountGetGlobalPrivacySettings{})
	if err != nil {
		t.Fatalf("AccountGetGlobalPrivacySettings() error = %v", err)
	}
	if got != settings {
		t.Fatalf("AccountGetGlobalPrivacySettings() returned %p, want stored %p", got, settings)
	}
	if stub.getRequest == nil || stub.getRequest.GetUserId() != 229001 {
		t.Fatalf("get request = %+v, want user 229001", stub.getRequest)
	}
}

func TestAccountSetGlobalPrivacySettingsPropagatesWriteFailure(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	stub := &globalPrivacyUserClientStub{setErr: wantErr}
	core := newGlobalPrivacyTestCore(stub)

	got, err := core.AccountSetGlobalPrivacySettings(&mtproto.TLAccountSetGlobalPrivacySettings{Settings: testGlobalPrivacySettings()})
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("AccountSetGlobalPrivacySettings() = (%v, %v), want nil and %v", got, err, wantErr)
	}
}

func TestAccountSetGlobalPrivacySettingsRequiresWriteAcknowledgement(t *testing.T) {
	for name, result := range map[string]*mtproto.Bool{
		"nil":   nil,
		"false": mtproto.BoolFalse,
	} {
		t.Run(name, func(t *testing.T) {
			stub := &globalPrivacyUserClientStub{setResult: result}
			core := newGlobalPrivacyTestCore(stub)

			got, err := core.AccountSetGlobalPrivacySettings(&mtproto.TLAccountSetGlobalPrivacySettings{Settings: testGlobalPrivacySettings()})
			if got != nil || err == nil {
				t.Fatalf("AccountSetGlobalPrivacySettings() = (%v, %v), want nil result and acknowledgement error", got, err)
			}
		})
	}
}

func TestAccountSetGlobalPrivacySettingsRejectsMissingSettings(t *testing.T) {
	stub := &globalPrivacyUserClientStub{setResult: mtproto.BoolTrue}
	core := newGlobalPrivacyTestCore(stub)

	got, err := core.AccountSetGlobalPrivacySettings(&mtproto.TLAccountSetGlobalPrivacySettings{})
	if got != nil || err == nil {
		t.Fatalf("AccountSetGlobalPrivacySettings() = (%v, %v), want missing-settings error", got, err)
	}
	if stub.setCallCount != 0 {
		t.Fatalf("user service called %d times for missing settings", stub.setCallCount)
	}
}

func TestAccountGetGlobalPrivacySettingsPropagatesReadFailure(t *testing.T) {
	wantErr := errors.New("user service unavailable")
	stub := &globalPrivacyUserClientStub{getErr: wantErr}
	core := newGlobalPrivacyTestCore(stub)

	got, err := core.AccountGetGlobalPrivacySettings(&mtproto.TLAccountGetGlobalPrivacySettings{})
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("AccountGetGlobalPrivacySettings() = (%v, %v), want nil and %v", got, err, wantErr)
	}
}

func TestAccountGetGlobalPrivacySettingsRejectsNilResponse(t *testing.T) {
	stub := &globalPrivacyUserClientStub{}
	core := newGlobalPrivacyTestCore(stub)

	got, err := core.AccountGetGlobalPrivacySettings(&mtproto.TLAccountGetGlobalPrivacySettings{})
	if got != nil || err == nil {
		t.Fatalf("AccountGetGlobalPrivacySettings() = (%v, %v), want nil-response error", got, err)
	}
}
