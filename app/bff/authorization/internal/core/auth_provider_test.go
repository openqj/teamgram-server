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
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

const authProviderTestSigningKey = "test-only-signing-key-with-at-least-32-bytes"

type authProviderTestUserClient struct {
	userclient.UserClient
	byTokenUser *mtproto.ImmutableUser
	byIDUser    *mtproto.ImmutableUser
	tokenReq    *userpb.TLUserGetImmutableUserByToken
	idReq       *userpb.TLUserGetImmutableUser
}

func (s *authProviderTestUserClient) UserGetImmutableUserByToken(_ context.Context, in *userpb.TLUserGetImmutableUserByToken) (*mtproto.ImmutableUser, error) {
	s.tokenReq = in
	return s.byTokenUser, nil
}

func (s *authProviderTestUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	s.idReq = in
	return s.byIDUser, nil
}

type authProviderTestAuthsessionClient struct {
	authsessionclient.AuthsessionClient
	bindReq *authsession.TLAuthsessionBindAuthKeyUser
	bindErr error
}

func (s *authProviderTestAuthsessionClient) AuthsessionBindAuthKeyUser(_ context.Context, in *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	s.bindReq = in
	if s.bindErr != nil {
		return nil, s.bindErr
	}
	return &mtproto.Int64{V: 1}, nil
}

func newAuthProviderTestServer(t *testing.T, signingKey string, response func(authProviderRequest) authProviderResponse, signResponse bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("provider method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-provider-key" {
			t.Errorf("provider authorization = %q, want configured bearer key", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read provider request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !verifyAuthProviderSignature(signingKey, r.Header.Get("X-Teamgram-Auth-Provider-Request-Signature"), body) {
			t.Errorf("provider request signature is invalid")
		}
		var request authProviderRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode provider request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(request.RequestID) != 64 {
			t.Errorf("request id length = %d, want 64 hex chars", len(request.RequestID))
		}
		responseBody, err := json.Marshal(response(request))
		if err != nil {
			t.Errorf("encode provider response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if signResponse {
			w.Header().Set("X-Teamgram-Auth-Provider-Signature", authProviderSignature(signingKey, responseBody))
		} else {
			w.Header().Set("X-Teamgram-Auth-Provider-Signature", "sha256=00")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	}))
}

func newAuthProviderTestCore(server *httptest.Server, users *authProviderTestUserClient, sessions *authProviderTestAuthsessionClient) *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			Config: config.Config{
				AuthProviderEndpoint:       server.URL,
				AuthProviderKey:            "test-provider-key",
				AuthProviderSigningKey:     authProviderTestSigningKey,
				AuthProviderTimeoutSeconds: 5,
			},
			Dao: &dao.Dao{UserClient: users, AuthsessionClient: sessions},
		},
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 9101},
		Logger: logx.WithContext(ctx),
	}
}

func authProviderTestUser(id int64, bot bool) *mtproto.ImmutableUser {
	var botData *mtproto.BotData
	if bot {
		botData = &mtproto.BotData{}
	}
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, Bot: botData}}
}

func TestAuthImportBotAuthorizationVerifiesAndBindsResolvedBot(t *testing.T) {
	apiID, apiHash := validAuthAPI()
	const userID = int64(501)
	server := newAuthProviderTestServer(t, authProviderTestSigningKey, func(request authProviderRequest) authProviderResponse {
		if request.Operation != "verify_bot_authorization" || request.APIID != apiID || request.APIHash != apiHash || request.BotAuthToken != "bot-token" || request.WebAuthToken != "" {
			t.Errorf("provider request = %+v", request)
		}
		return authProviderResponse{RequestID: request.RequestID, Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: userID}
	}, true)
	defer server.Close()

	users := &authProviderTestUserClient{byTokenUser: authProviderTestUser(userID, true)}
	sessions := &authProviderTestAuthsessionClient{}
	core := newAuthProviderTestCore(server, users, sessions)
	result, err := core.AuthImportBotAuthorization(&mtproto.TLAuthImportBotAuthorization{
		ApiId: apiID, ApiHash: apiHash, BotAuthToken: "bot-token",
	})
	if err != nil || result == nil {
		t.Fatalf("bot import = (%v, %v), want authorization", result, err)
	}
	if users.tokenReq == nil || users.tokenReq.GetToken() != "bot-token" {
		t.Fatalf("token lookup = %v, want supplied bot token", users.tokenReq)
	}
	if sessions.bindReq == nil || sessions.bindReq.GetAuthKeyId() != 9101 || sessions.bindReq.GetUserId() != userID {
		t.Fatalf("auth key bind = %v, want auth key 9101 and user %d", sessions.bindReq, userID)
	}
}

func TestAuthImportBotAuthorizationRejectsProviderDatabaseIdentityMismatch(t *testing.T) {
	apiID, apiHash := validAuthAPI()
	server := newAuthProviderTestServer(t, authProviderTestSigningKey, func(request authProviderRequest) authProviderResponse {
		return authProviderResponse{RequestID: request.RequestID, Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: 502}
	}, true)
	defer server.Close()

	users := &authProviderTestUserClient{byTokenUser: authProviderTestUser(501, true)}
	sessions := &authProviderTestAuthsessionClient{}
	core := newAuthProviderTestCore(server, users, sessions)
	result, err := core.AuthImportBotAuthorization(&mtproto.TLAuthImportBotAuthorization{
		ApiId: apiID, ApiHash: apiHash, BotAuthToken: "bot-token",
	})
	if result != nil || !errors.Is(err, mtproto.ErrAccessTokenInvalid) {
		t.Fatalf("bot import = (%v, %v), want ACCESS_TOKEN_INVALID", result, err)
	}
	if sessions.bindReq != nil {
		t.Fatalf("mismatched bot identity was bound: %v", sessions.bindReq)
	}
}

func TestAuthImportWebTokenAuthorizationVerifiesAndBindsResolvedUser(t *testing.T) {
	apiID, apiHash := validAuthAPI()
	const userID = int64(601)
	server := newAuthProviderTestServer(t, authProviderTestSigningKey, func(request authProviderRequest) authProviderResponse {
		if request.Operation != "verify_web_token_authorization" || request.APIID != apiID || request.APIHash != apiHash || request.WebAuthToken != "web-token" || request.BotAuthToken != "" {
			t.Errorf("provider request = %+v", request)
		}
		return authProviderResponse{RequestID: request.RequestID, Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: userID}
	}, true)
	defer server.Close()

	users := &authProviderTestUserClient{byIDUser: authProviderTestUser(userID, false)}
	sessions := &authProviderTestAuthsessionClient{}
	core := newAuthProviderTestCore(server, users, sessions)
	result, err := core.AuthImportWebTokenAuthorization(&mtproto.TLAuthImportWebTokenAuthorization{
		ApiId: apiID, ApiHash: apiHash, WebAuthToken: "web-token",
	})
	if err != nil || result == nil {
		t.Fatalf("web-token import = (%v, %v), want authorization", result, err)
	}
	if users.idReq == nil || users.idReq.GetId() != userID {
		t.Fatalf("user lookup = %v, want provider user %d", users.idReq, userID)
	}
	if sessions.bindReq == nil || sessions.bindReq.GetAuthKeyId() != 9101 || sessions.bindReq.GetUserId() != userID {
		t.Fatalf("auth key bind = %v, want auth key 9101 and user %d", sessions.bindReq, userID)
	}
}

func TestAuthImportWebTokenAuthorizationRejectsResolvedUserIdentityMismatch(t *testing.T) {
	apiID, apiHash := validAuthAPI()
	server := newAuthProviderTestServer(t, authProviderTestSigningKey, func(request authProviderRequest) authProviderResponse {
		return authProviderResponse{RequestID: request.RequestID, Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: 602}
	}, true)
	defer server.Close()

	users := &authProviderTestUserClient{byIDUser: authProviderTestUser(601, false)}
	sessions := &authProviderTestAuthsessionClient{}
	core := newAuthProviderTestCore(server, users, sessions)
	result, err := core.AuthImportWebTokenAuthorization(&mtproto.TLAuthImportWebTokenAuthorization{
		ApiId: apiID, ApiHash: apiHash, WebAuthToken: "web-token",
	})
	if result != nil || !errors.Is(err, mtproto.ErrAccessTokenInvalid) {
		t.Fatalf("web-token import = (%v, %v), want ACCESS_TOKEN_INVALID", result, err)
	}
	if sessions.bindReq != nil {
		t.Fatalf("mismatched web-token identity was bound: %v", sessions.bindReq)
	}
}

func TestAuthProviderRejectsInvalidResponseSignature(t *testing.T) {
	server := newAuthProviderTestServer(t, authProviderTestSigningKey, func(request authProviderRequest) authProviderResponse {
		return authProviderResponse{RequestID: request.RequestID, Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: 700}
	}, false)
	defer server.Close()

	core := newAuthProviderTestCore(server, &authProviderTestUserClient{}, &authProviderTestAuthsessionClient{})
	if result, err := core.authProvider(context.Background(), authProviderRequest{Operation: "verify_web_token_authorization", APIID: 100}); result.UserID != 0 || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("provider result = (%+v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}

func TestAuthProviderRejectsSignedReplayResponse(t *testing.T) {
	server := newAuthProviderTestServer(t, authProviderTestSigningKey, func(request authProviderRequest) authProviderResponse {
		return authProviderResponse{RequestID: "previous-request", Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: 700}
	}, true)
	defer server.Close()

	core := newAuthProviderTestCore(server, &authProviderTestUserClient{}, &authProviderTestAuthsessionClient{})
	if result, err := core.authProvider(context.Background(), authProviderRequest{Operation: "verify_web_token_authorization", APIID: 100}); result.UserID != 0 || !errors.Is(err, mtproto.ErrAccessTokenInvalid) {
		t.Fatalf("provider result = (%+v, %v), want ACCESS_TOKEN_INVALID", result, err)
	}
}
