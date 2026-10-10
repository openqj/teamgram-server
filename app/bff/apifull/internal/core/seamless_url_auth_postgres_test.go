package core

import (
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestURLAuthMethodsUsePostgresAcrossCoreCalls(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = persist.ResetURLAuths(782000000 + int64(os.Getpid()))
	})

	userID := int64(782000000 + os.Getpid())
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	const rawURL = "https://example.invalid/core-url-auth"
	if _, err := core.MessagesRequestUrlAuth(&mtproto.TLMessagesRequestUrlAuth{
		Url: wrapperspb.String(rawURL),
	}); err != nil {
		t.Fatal(err)
	}
	accepted, err := core.MessagesAcceptUrlAuth(&mtproto.TLMessagesAcceptUrlAuth{
		Url:       wrapperspb.String(rawURL),
		MatchCode: wrapperspb.String("core-match"),
		Peer:      mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9055}).To_InputPeer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := accepted.GetUrl_FLAGSTRING().GetValue(); got != rawURL {
		t.Fatalf("accepted URL=%q want %q", got, rawURL)
	}
	matched, err := core.MessagesCheckUrlAuthMatchCode(&mtproto.TLMessagesCheckUrlAuthMatchCode{
		Url: rawURL, MatchCode: "core-match",
	})
	if err != nil || matched != mtproto.BoolTrue {
		t.Fatalf("match result=%v err=%v", matched, err)
	}
	listed, err := core.AccountGetWebAuthorizations(&mtproto.TLAccountGetWebAuthorizations{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetAuthorizations()) != 1 || listed.GetAuthorizations()[0].GetDomain() != "example.invalid" {
		t.Fatalf("web authorizations=%v", listed.GetAuthorizations())
	}
	bob := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID + 1}}
	if reset, resetErr := bob.AccountResetWebAuthorization(&mtproto.TLAccountResetWebAuthorization{Hash: listed.GetAuthorizations()[0].GetHash()}); resetErr != nil || reset != mtproto.BoolTrue {
		t.Fatalf("cross-user reset result=%v err=%v", reset, resetErr)
	}
	listed, err = core.AccountGetWebAuthorizations(&mtproto.TLAccountGetWebAuthorizations{})
	if err != nil || len(listed.GetAuthorizations()) != 1 {
		t.Fatalf("cross-user reset removed authorization: items=%v err=%v", listed.GetAuthorizations(), err)
	}
	if _, err = core.MessagesDeclineUrlAuth(&mtproto.TLMessagesDeclineUrlAuth{Url: rawURL}); err != nil {
		t.Fatal(err)
	}
	listed, err = core.AccountGetWebAuthorizations(&mtproto.TLAccountGetWebAuthorizations{})
	if err != nil || len(listed.GetAuthorizations()) != 0 {
		t.Fatalf("declined authorizations=%v err=%v", listed.GetAuthorizations(), err)
	}
}
