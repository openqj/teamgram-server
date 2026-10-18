package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestPaidReactionPrivacyFallsBackToLocalDefaultWhenProviderMissesOperation(t *testing.T) {
	const userID int64 = 991993
	store := &localPrivacyStore{values: map[string]string{}}
	previous := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(previous) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request businessProviderEnvelope
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode provider request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.RequestID == "" || request.Operation != "messages.getPaidReactionPrivacy" || request.UserID != userID {
			t.Errorf("provider request = %+v", request)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := json.Marshal(businessProviderEnvelope{
			RequestID: request.RequestID,
			Operation: request.Operation,
			UserID:    request.UserID,
			Error:     "operation not provisioned",
		})
		if err != nil {
			t.Errorf("encode provider response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Teamgram-Business-Provider-Signature", businessProviderSignature(businessProviderGapTestKey, body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	core := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{
			BusinessProviderEndpoint:   server.URL,
			BusinessProviderSigningKey: businessProviderGapTestKey,
		}},
		MD: &metadata.RpcMetadata{UserId: userID},
	}
	updates, err := core.MessagesGetPaidReactionPrivacy(&mtproto.TLMessagesGetPaidReactionPrivacy{})
	if err != nil || updates == nil || len(updates.GetUpdates()) != 1 {
		t.Fatalf("provider miss fallback = (%v, %v), want one local update", updates, err)
	}
	item := updates.GetUpdates()[0]
	if item.GetPredicateName() != mtproto.Predicate_updatePaidReactionPrivacy ||
		item.GetPrivate_PAIDREACTIONPRIVACY() == nil ||
		item.GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName() != mtproto.Predicate_paidReactionPrivacyDefault ||
		item.GetPrivate_BOOL() != mtproto.BoolFalse {
		t.Fatalf("fallback update = %v, want default privacy and BoolFalse", item)
	}
}
