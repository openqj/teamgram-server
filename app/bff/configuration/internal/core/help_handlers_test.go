package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	configuration "github.com/teamgram/teamgram-server/app/bff/configuration/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/svc"
)

func TestHelpHandlersUseDeploymentHelpConfig(t *testing.T) {
	previous := sharedpersist.Default
	sharedpersist.Default = &testHelpStore{values: map[string]string{
		helpConfigKey: `{"inviteText":"Join us","supportPhoneNumber":"+100","supportUserId":42,"supportUserAccessHash":99,"supportUserUsername":"support","appUpdateVersion":"2.0","appUpdateText":"Update now","appUpdateUrl":"https://example.test/update"}`,
	}}
	t.Cleanup(func() { sharedpersist.Default = previous })
	c := &ConfigurationCore{svcCtx: &svc.ServiceContext{Config: configuration.Config{Help: configuration.HelpConfig{InviteText: "config fallback"}}}}

	invite, err := c.HelpGetInviteText(&mtproto.TLHelpGetInviteText{})
	if err != nil || invite.GetMessage() != "Join us" {
		t.Fatalf("invite = %q, err=%v", invite.GetMessage(), err)
	}
	support, err := c.HelpGetSupport(&mtproto.TLHelpGetSupport{})
	if err != nil || support.GetPhoneNumber() != "+100" || support.GetUser().GetId() != 42 || support.GetUser().GetUsername().GetValue() != "support" {
		t.Fatalf("support = %+v, err=%v", support, err)
	}
	update, err := c.HelpGetAppUpdate(&mtproto.TLHelpGetAppUpdate{Source: "test"})
	if err != nil || update.GetVersion() != "2.0" || update.GetUrl().GetValue() != "https://example.test/update" {
		t.Fatalf("update = %+v, err=%v", update, err)
	}
}

func TestHelpGetAppUpdateReturnsNoUpdateWhenUnset(t *testing.T) {
	previous := sharedpersist.Default
	sharedpersist.Default = &testHelpStore{values: map[string]string{}}
	t.Cleanup(func() { sharedpersist.Default = previous })
	got, err := (&ConfigurationCore{}).HelpGetAppUpdate(&mtproto.TLHelpGetAppUpdate{})
	if err != nil || got.GetPredicateName() != mtproto.Predicate_help_noAppUpdate {
		t.Fatalf("result = %+v, err=%v", got, err)
	}
}

func TestHelpGetSupportNameUsesDeploymentHelpConfig(t *testing.T) {
	previous := sharedpersist.Default
	sharedpersist.Default = &testHelpStore{values: map[string]string{
		helpConfigKey: `{"supportUserUsername":"support_bot"}`,
	}}
	t.Cleanup(func() { sharedpersist.Default = previous })

	got, err := (&ConfigurationCore{}).HelpGetSupportName(&mtproto.TLHelpGetSupportName{})
	if err != nil {
		t.Fatalf("HelpGetSupportName() error = %v", err)
	}
	if got == nil || got.GetPredicateName() != mtproto.Predicate_help_supportName || got.GetName() != "support_bot" {
		t.Fatalf("HelpGetSupportName() = %+v", got)
	}
}

type testHelpStore struct{ values map[string]string }

func (s *testHelpStore) Get(key string) (string, error) { return s.values[key], nil }
func (s *testHelpStore) Set(key, value string) error    { s.values[key] = value; return nil }
