package core

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

func TestHelpDismissSuggestionPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open PostgreSQL store: %v", err)
	}
	uid := time.Now().UnixNano()
	key := dismissSuggestionKey(uid)
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid + 1}).To_InputPeer()
	c := &ConfigurationCore{MD: &metadata.RpcMetadata{UserId: uid}}
	req := &mtproto.TLHelpDismissSuggestion{Peer: peer, Suggestion: "suggestion-a"}
	if got, err := c.HelpDismissSuggestion(req); err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatalf("first dismiss = (%v, %v)", got, err)
	}
	if got, err := c.HelpDismissSuggestion(req); err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_boolTrue {
		t.Fatalf("retry dismiss = (%v, %v)", got, err)
	}
	raw, err := persist.Default.Get(key)
	if err != nil {
		t.Fatalf("read persisted dismissals: %v", err)
	}
	var entries map[string][]string
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("decode persisted dismissals: %v", err)
	}
	if got := entries["user:"+strconv.FormatInt(uid+1, 10)]; len(got) != 1 || got[0] != "suggestion-a" {
		t.Fatalf("persisted dismissals = %#v, want one idempotent entry", entries)
	}
}

func TestHelpDismissSuggestionRejectsInvalidRequests(t *testing.T) {
	if got, err := (&ConfigurationCore{}).HelpDismissSuggestion(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated request = (%v, %v)", got, err)
	}
	c := &ConfigurationCore{MD: &metadata.RpcMetadata{UserId: 12}}
	if got, err := c.HelpDismissSuggestion(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%v, %v)", got, err)
	}
	if got, err := c.HelpDismissSuggestion(&mtproto.TLHelpDismissSuggestion{Suggestion: "x"}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("missing peer = (%v, %v)", got, err)
	}
	if got, err := c.HelpDismissSuggestion(&mtproto.TLHelpDismissSuggestion{
		Peer:       mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer(),
		Suggestion: "x",
	}); got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("invalid peer = (%v, %v)", got, err)
	}
}
