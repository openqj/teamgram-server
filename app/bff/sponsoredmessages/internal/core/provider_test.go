package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/sponsoredmessages/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/sponsoredmessages/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const sponsoredProviderTestKey = "sponsored-provider-test-signing-key-0123456789"

func TestMessagesGetSponsoredMessagesUsesVerifiedProvider(t *testing.T) {
	var requestIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if r.Header.Get("Idempotency-Key") != request.RequestID {
			t.Fatalf("idempotency key = %q, request id = %q", r.Header.Get("Idempotency-Key"), request.RequestID)
		}
		requestIDs = append(requestIDs, request.RequestID)
		result := map[string]any{"predicate_name": "messages_sponsoredMessages", "constructor": 0, "messages": []any{}, "chats": []any{}, "users": []any{}}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(map[string]any{"request_id": request.RequestID, "operation": request.Operation, "verified": true, "user_id": request.UserID, "result": json.RawMessage(encoded)})
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, []byte(sponsoredProviderTestKey))
		_, _ = mac.Write(body)
		w.Header().Set("X-Teamgram-Sponsored-Provider-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c := New(context.Background(), &svc.ServiceContext{Config: config.Config{SponsoredProviderEndpoint: server.URL, SponsoredProviderSigningKey: sponsoredProviderTestKey}})
	c.MD = &metadata.RpcMetadata{UserId: time.Now().UnixNano()}
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	got, err := c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{Peer: peer})
	if err != nil || got == nil || got.GetPredicateName() == "" {
		t.Fatalf("MessagesGetSponsoredMessages = (%v, %v)", got, err)
	}
	got, err = c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{Peer: peer})
	if err != nil || got == nil || got.GetPredicateName() == "" {
		t.Fatalf("second MessagesGetSponsoredMessages = (%v, %v)", got, err)
	}
	if len(requestIDs) != 2 || requestIDs[0] == "" || requestIDs[0] != requestIDs[1] {
		t.Fatalf("request IDs = %#v, want stable retry key", requestIDs)
	}
}

func TestMessagesGetSponsoredMessagesRejectsMessageSelectorForInventory(t *testing.T) {
	c := New(context.Background(), &svc.ServiceContext{})
	c.MD = &metadata.RpcMetadata{UserId: time.Now().UnixNano()}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	got, err := c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{
		Peer:  peer,
		MsgId: wrapperspb.Int32(1),
	})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesGetSponsoredMessages() = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}

func TestSecureSponsoredProviderURLRequiresFixtureOptInForHTTP(t *testing.T) {
	fixtureURL, err := url.Parse("http://sponsored-provider-fixture:23195")
	if err != nil {
		t.Fatal(err)
	}
	if secureSponsoredProviderURL(fixtureURL, false) {
		t.Fatal("fixture hostname should not allow HTTP in production mode")
	}
	if !secureSponsoredProviderURL(fixtureURL, true) {
		t.Fatal("fixture hostname should allow HTTP in provider-fixture mode")
	}
	loopbackURL, err := url.Parse("http://127.0.0.1:23195")
	if err != nil {
		t.Fatal(err)
	}
	if !secureSponsoredProviderURL(loopbackURL, false) {
		t.Fatal("loopback HTTP should remain available for local tests")
	}
}
