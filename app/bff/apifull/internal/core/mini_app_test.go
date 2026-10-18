// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"github.com/teamgram/teamgram-server/app/botapi"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMiniAppWebViewScopeKeyUsesStartParamValue(t *testing.T) {
	got := miniAppWebViewScopeKey(
		"bot:900000000001:short:demo",
		10,
		mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		wrapperspb.String("start"),
		"web",
	)
	const want = "bot:900000000001:short:demo:peer:1:10:start:start:platform:web"
	if got != want {
		t.Fatalf("scope key = %q, want %q", got, want)
	}
}

func TestMiniAppWebViewScopeKeyWithOptionsIsStableAndIsolated(t *testing.T) {
	appKey := "bot:900000000001:short:demo"
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	base := miniAppWebViewScopeKey(appKey, 10, peer, nil, "web")
	if got := miniAppWebViewScopeKeyWithOptions(appKey, 10, peer, nil, "web", false, false, false, nil); got != base {
		t.Fatalf("default options changed legacy scope key: got %q want %q", got, base)
	}
	if got := miniAppWebViewScopeKeyWithOptions(appKey, 10, peer, nil, "web", true, false, false, nil); got == base {
		t.Fatal("write_allowed option reused the default scope key")
	}
	theme := &mtproto.DataJSON{Data: `{"bg_color":"#fff"}`}
	first := miniAppWebViewScopeKeyWithOptions(appKey, 10, peer, nil, "web", false, false, true, theme)
	second := miniAppWebViewScopeKeyWithOptions(appKey, 10, peer, nil, "web", false, false, true, theme)
	if first == base || first != second {
		t.Fatalf("theme/fullscreen scope key is not deterministic and isolated: first=%q second=%q base=%q", first, second, base)
	}
}

func TestMiniAppRequestWebView(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesRequestWebView(&mtproto.TLMessagesRequestWebView{}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("request webview: got=%v err=%v, want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := c.MessagesRequestWebView(&mtproto.TLMessagesRequestWebView{
		Bot: mtproto.MakeTLInputUserSelf(nil).To_InputUser(), Url: wrapperspb.String("https://bot.example.test/app"),
	}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("request webview without peer: got=%v err=%v, want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := c.MessagesRequestMainWebView(&mtproto.TLMessagesRequestMainWebView{
		Bot: mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("request main webview without peer: got=%v err=%v, want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := c.MessagesGetBotApp(&mtproto.TLMessagesGetBotApp{}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("get bot app: got=%v err=%v, want INPUT_REQUEST_INVALID", got, err)
	}
}

func TestMiniAppURL(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 14}}
	if got, err := c.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("send webview data: got=%v err=%v, want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := c.MessagesProlongWebView(&mtproto.TLMessagesProlongWebView{}); got != nil || !errors.Is(err, mtproto.ErrQueryIdEmpty) {
		t.Fatalf("prolong webview: got=%v err=%v, want QUERY_ID_EMPTY", got, err)
	}
}

func TestSignedMiniAppURLUsesBotTokenWithoutExposingIt(t *testing.T) {
	const (
		callerID = int64(712)
		botID    = int64(900000000001)
		botToken = "123456:miniapp-test-token"
	)
	client := &miniBotAppsUserClient{botInfos: map[int64]*userpb.BotInfoData{
		botID: {Token: botToken},
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
	}

	got, err := c.signedMiniAppURL(callerID, botID, 99, "https://mini.example/app?start=demo#fragment")
	if err != nil {
		t.Fatalf("signedMiniAppURL: %v", err)
	}
	if strings.Contains(got, botToken) {
		t.Fatalf("signed URL exposed bot token: %q", got)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	hashValues := miniAppHashValues(t, parsed)
	if parsed.Query().Get("tgWebAppData") != "" || parsed.Query().Get("tgWebAppStartParam") != "" {
		t.Fatalf("Telegram fields leaked into URL query: %q", parsed.RawQuery)
	}
	values, err := botapi.ValidateWebAppInitData(hashValues.Get("tgWebAppData"), botToken, time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("signed init data did not validate: %v", err)
	}
	if values.Get("query_id") != "99" || values.Get("user") != `{"id":712}` {
		t.Fatalf("signed init data = %q", values.Encode())
	}
	if parsed.Query().Get("start") != "demo" || !strings.HasPrefix(parsed.EscapedFragment(), "fragment?") {
		t.Fatalf("URL lost existing query or fragment: %q", got)
	}
	withStart, err := c.signedMiniAppURLWithStartParam(callerID, botID, 99, "https://mini.example/app", "launch")
	if err != nil {
		t.Fatalf("signedMiniAppURLWithStartParam: %v", err)
	}
	withStartURL := mustParseURL(t, withStart)
	if strings.HasPrefix(withStartURL.EscapedFragment(), "?") {
		t.Fatalf("signed Web App URL has non-canonical query-only hash: %q", withStartURL.EscapedFragment())
	}
	if got := miniAppHashValues(t, withStartURL).Get("tgWebAppStartParam"); got != "launch" {
		t.Fatalf("tgWebAppStartParam = %q, want launch", got)
	}
}

func TestSignedMiniAppURLReplacesProviderInitData(t *testing.T) {
	const (
		callerID = int64(712)
		botID    = int64(900000000001)
		botToken = "123456:miniapp-test-token"
	)
	client := &miniBotAppsUserClient{botInfos: map[int64]*userpb.BotInfoData{
		botID: {Token: botToken},
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
	}
	got, err := c.signedMiniAppURL(callerID, botID, 101, "https://mini.example/app?tgWebAppData=forged&tgWebAppStartParam=forged&start=demo#tgWebAppData=forged&tgWebAppStartParam=forged")
	if err != nil {
		t.Fatalf("signedMiniAppURL: %v", err)
	}
	parsed := mustParseURL(t, got)
	if parsed.Query().Get("tgWebAppData") != "" || parsed.Query().Get("tgWebAppStartParam") != "" {
		t.Fatalf("provider init data was forwarded: %q", got)
	}
	hashValues := miniAppHashValues(t, parsed)
	if hashValues.Get("tgWebAppStartParam") != "" || hashValues.Get("tgWebAppData") == "forged" {
		t.Fatalf("provider Telegram fields were forwarded: %q", parsed.EscapedFragment())
	}
	values, err := botapi.ValidateWebAppInitData(hashValues.Get("tgWebAppData"), botToken, time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("replacement init data did not validate: %v", err)
	}
	if values.Get("query_id") != "101" || values.Get("user") != `{"id":712}` {
		t.Fatalf("replacement init data = %q", values.Encode())
	}
}

func TestSignedMiniAppURLRequiresBotToken(t *testing.T) {
	const botID = int64(900000000001)
	client := &miniBotAppsUserClient{botInfos: map[int64]*userpb.BotInfoData{
		botID: {},
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
	}
	if got, err := c.signedMiniAppURL(712, botID, 101, "https://mini.example/app"); got != "" || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing bot token result = %q, err=%v, want METHOD_NOT_IMPL", got, err)
	}
}

func TestResolveMiniWebViewURLUsesBotMenuWhenURLIsOmitted(t *testing.T) {
	const (
		callerID = int64(712)
		botID    = int64(900000000001)
		botHash  = int64(404)
		menuURL  = "https://mini.example/menu"
	)
	client := &miniBotAppsUserClient{
		users: map[int64]*mtproto.ImmutableUser{
			botID: miniBotProfile(botID, botHash, true),
		},
		botInfos: map[int64]*userpb.BotInfoData{
			botID: {
				BotInfo: mtproto.MakeTLBotInfo(&mtproto.BotInfo{
					MenuButton: mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{
						Text: "Open",
						Url:  menuURL,
					}).To_BotMenuButton(),
				}).To_BotInfo(),
			},
		},
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	resolved, err := c.resolveMiniWebViewURL(miniBotInput(botID, botHash), nil)
	if err != nil {
		t.Fatalf("resolve omitted URL: %v", err)
	}
	if resolved != menuURL {
		t.Fatalf("resolved URL = %q, want %q", resolved, menuURL)
	}
	if resolved, err = c.resolveMiniWebViewURL(miniBotInput(botID, botHash), wrapperspb.String("https://mini.example/explicit")); err != nil || resolved != "https://mini.example/explicit" {
		t.Fatalf("resolve explicit URL = %q, err=%v", resolved, err)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return parsed
}

func miniAppHashValues(t *testing.T, parsed *url.URL) url.Values {
	t.Helper()
	fragment := parsed.EscapedFragment()
	query := fragment
	if question := strings.IndexByte(fragment, '?'); question >= 0 {
		query = fragment[question+1:]
	} else if !strings.Contains(fragment, "=") {
		query = ""
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parse Mini App hash %q: %v", fragment, err)
	}
	return values
}
