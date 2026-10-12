package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const userChannelProfilesProviderTestSigningKey = "user-channel-profiles-provider-key-at-least-32-bytes"

func TestUsersSuggestBirthdayUsesVerifiedProvider(t *testing.T) {
	const callerID int64 = 8801300
	const targetID int64 = 8801301
	recipient := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID, AccessHash: 123}).To_InputUser()
	birthday := mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3, Year: wrapperspb.Int32(1990)}).To_Birthday()
	var request birthdaySuggestionProviderRequest
	var idempotencyKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read provider request: %v", err)
		}
		if !verifyUserChannelProfilesProviderSignature(userChannelProfilesProviderTestSigningKey, r.Header.Get("X-Teamgram-UserChannelProfiles-Provider-Request-Signature"), body) {
			t.Fatal("provider request signature is invalid")
		}
		if err = json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		idempotencyKeys = append(idempotencyKeys, r.Header.Get("Idempotency-Key"))
		result := mtproto.MakeTLUpdates(&mtproto.Updates{Date: 100, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{}, Updates: []*mtproto.Update{}}).To_Updates()
		responseBody, _ := json.Marshal(birthdaySuggestionProviderResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			CallerID: callerID, TargetID: targetID, Result: result,
		})
		w.Header().Set("X-Teamgram-UserChannelProfiles-Provider-Signature", userChannelProfilesProviderSignature(userChannelProfilesProviderTestSigningKey, responseBody))
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()
	c := &UserChannelProfilesCore{
		ctx: context.Background(), Logger: logx.WithContext(context.Background()),
		MD: &metadata.RpcMetadata{UserId: callerID},
		svcCtx: &svc.ServiceContext{Config: config.Config{
			UserChannelProfilesProviderEndpoint:   server.URL,
			UserChannelProfilesProviderKey:        "provider-key",
			UserChannelProfilesProviderSigningKey: userChannelProfilesProviderTestSigningKey,
		}},
	}
	got, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{Id: recipient, Birthday: birthday})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("suggest birthday = (%#v, %v), want updates", got, err)
	}
	if request.Operation != "users.suggestBirthday" || request.CallerID != callerID || request.TargetID != targetID || request.Recipient == nil || request.Recipient.GetUserId() != targetID || request.Birthday == nil || request.Birthday.GetDay() != 12 || request.Birthday.GetMonth() != 3 || request.Birthday.GetYear().GetValue() != 1990 {
		t.Fatalf("provider request = %+v", request)
	}
	if _, err = c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{Id: recipient, Birthday: birthday}); err != nil {
		t.Fatalf("repeat suggest birthday = %v", err)
	}
	if len(idempotencyKeys) != 2 || len(idempotencyKeys[0]) != 64 || idempotencyKeys[0] != idempotencyKeys[1] {
		t.Fatalf("idempotency keys = %#v, want same 64-character key", idempotencyKeys)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("updates did not encode: %v", err)
	}
}

func TestUsersSuggestBirthdayRejectsUntrustedProviderResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request birthdaySuggestionProviderRequest
		_ = json.Unmarshal(body, &request)
		responseBody, _ := json.Marshal(birthdaySuggestionProviderResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			CallerID: request.CallerID, TargetID: request.TargetID, Result: &mtproto.Updates{PredicateName: "unknown"},
		})
		w.Header().Set("X-Teamgram-UserChannelProfiles-Provider-Signature", userChannelProfilesProviderSignature(userChannelProfilesProviderTestSigningKey, responseBody))
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()
	c := &UserChannelProfilesCore{
		ctx: context.Background(), Logger: logx.WithContext(context.Background()),
		MD: &metadata.RpcMetadata{UserId: 8801302},
		svcCtx: &svc.ServiceContext{Config: config.Config{
			UserChannelProfilesProviderEndpoint:   server.URL,
			UserChannelProfilesProviderSigningKey: userChannelProfilesProviderTestSigningKey,
		}},
	}
	got, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 8801303, AccessHash: 123}).To_InputUser(),
		Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3}).To_Birthday(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("suggest birthday = (%#v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}
