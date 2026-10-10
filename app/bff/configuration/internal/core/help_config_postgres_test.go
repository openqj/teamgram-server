package core

import (
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

func TestHelpHandlersReadPostgresConfiguration(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := sharedpersist.Default
	if err := sharedpersist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	previousRaw, err := sharedpersist.Default.Get(helpConfigKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sharedpersist.Default.Set(helpConfigKey, previousRaw)
		_ = sharedpersist.ClosePostgres()
		sharedpersist.Default = previous
	})
	if err := sharedpersist.Default.Set(helpConfigKey, `{"inviteText":"PostgreSQL invite","appUpdateVersion":"9.9"}`); err != nil {
		t.Fatal(err)
	}
	c := &ConfigurationCore{}
	invite, err := c.HelpGetInviteText(&mtproto.TLHelpGetInviteText{})
	if err != nil || invite.GetMessage() != "PostgreSQL invite" {
		t.Fatalf("invite = %q, err=%v", invite.GetMessage(), err)
	}
	update, err := c.HelpGetAppUpdate(&mtproto.TLHelpGetAppUpdate{})
	if err != nil || update.GetVersion() != "9.9" {
		t.Fatalf("update = %q, err=%v", update.GetVersion(), err)
	}
}
