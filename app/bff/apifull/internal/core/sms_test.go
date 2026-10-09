package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestSmsjobsJoinUserId(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if err := persist.Default.Set("sms:1:joined", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.Default.Set("sms:1:joined", "") })
	if got, err := c.SmsjobsJoin(&mtproto.TLSmsjobsJoin{}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("join=(%#v, %v), want postgres BOOL_TRUE", got, err)
	}
}

func TestSmsJoinPostgresDoesNotUseLegacyStore(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 17}}
	if err := persist.Default.Set("sms:17:joined", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.Default.Set("sms:17:joined", "") })
	if got, err := c.SmsjobsJoin(&mtproto.TLSmsjobsJoin{}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("join=(%#v, %v), want postgres BOOL_TRUE", got, err)
	}
	got, err := persist.Default.Get("sms:17:joined")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("unsupported SMS job wrote state: %q", got)
	}
}
