package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/config"
	accountdao "github.com/teamgram/teamgram-server/app/bff/account/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/account/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestBotConnectionProviderFixtureHostRequiresExplicitMode(t *testing.T) {
	parsed, err := url.Parse("http://identity-provider-fixture.test:23192")
	if err != nil {
		t.Fatal(err)
	}
	if secureBotConnectionProviderURL(parsed, false) {
		t.Fatal("fixture host accepted without explicit fixture mode")
	}
	if !secureBotConnectionProviderURL(parsed, true) {
		t.Fatal("fixture host rejected in explicit fixture mode")
	}
}

type confirmBotUserClient struct {
	userclient.UserClient
	profile *mtproto.ImmutableUser
}

func (c *confirmBotUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.profile, nil
}

func TestAccountConfirmBotConnectionFailsClosedWithoutProvider(t *testing.T) {
	c := &AccountCore{MD: &metadata.RpcMetadata{UserId: 42}}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 4300}).To_InputUser()
	got, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{BotId: bot})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("confirm bot connection = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}

func TestAccountConfirmBotConnectionUsesVerifiedProvider(t *testing.T) {
	const signingKey = "account-bot-provider-signing-key-0123456789"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var request struct {
			RequestID string `json:"request_id"`
			Operation string `json:"operation"`
			UserID    int64  `json:"user_id"`
		}
		if err = json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		response, _ := json.Marshal(map[string]any{"request_id": request.RequestID, "operation": request.Operation, "verified": true, "user_id": request.UserID, "result": map[string]any{"confirmed": true}})
		mac := hmac.New(sha256.New, []byte(signingKey))
		_, _ = mac.Write(response)
		w.Header().Set("X-Teamgram-Bot-Connection-Provider-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		_, _ = w.Write(response)
	}))
	defer server.Close()
	users := &confirmBotUserClient{profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: 43, AccessHash: 4300, Bot: &mtproto.BotData{}},
	}).To_ImmutableUser()}
	c := &AccountCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{
		Config: config.Config{BotConnectionProviderEndpoint: server.URL, BotConnectionProviderSigningKey: signingKey},
		Dao:    &accountdao.Dao{UserClient: users},
	}}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 4300}).To_InputUser()
	got, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{BotId: bot})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("confirm bot connection = (%v, %v)", got, err)
	}
}

func TestAccountConfirmBotConnectionRejectsNonBotOrStaleAccessHash(t *testing.T) {
	const signingKey = "account-bot-provider-signing-key-0123456789"
	c := &AccountCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{
		Config: config.Config{BotConnectionProviderEndpoint: "https://provider.example", BotConnectionProviderSigningKey: signingKey},
		Dao: &accountdao.Dao{UserClient: &confirmBotUserClient{profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: 43, AccessHash: 4300},
		}).To_ImmutableUser()}},
	}}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 4300}).To_InputUser()
	if got, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{BotId: bot}); got != nil || !errors.Is(err, mtproto.ErrBotInvalid) {
		t.Fatalf("non-bot confirmation = (%v, %v), want BOT_INVALID", got, err)
	}

	c.svcCtx.Dao.UserClient = &confirmBotUserClient{profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: 43, AccessHash: 4301, Bot: &mtproto.BotData{}},
	}).To_ImmutableUser()}
	if got, err := c.AccountConfirmBotConnection(&mtproto.TLAccountConfirmBotConnection{BotId: bot}); got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("stale access hash confirmation = (%v, %v), want USER_ID_INVALID", got, err)
	}
}
