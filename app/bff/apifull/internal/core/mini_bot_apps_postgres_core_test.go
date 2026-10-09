// Copyright 2026 Teamgram Authors
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

package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type miniBotAppsUserClient struct {
	user_client.UserClient
	users    map[int64]*mtproto.ImmutableUser
	botInfos map[int64]*userpb.BotInfoData
}

func (c *miniBotAppsUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if user := c.users[in.GetId()]; user != nil {
		return user, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

func (c *miniBotAppsUserClient) UserGetBotInfoV2(_ context.Context, in *userpb.TLUserGetBotInfoV2) (*userpb.BotInfoData, error) {
	if info := c.botInfos[in.GetBotId()]; info != nil {
		return info, nil
	}
	return nil, mtproto.ErrBotInvalid
}

func miniBotInput(id, accessHash int64) *mtproto.InputUser {
	return &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: id, AccessHash: accessHash}
}

func miniBotProfile(id, accessHash int64, bot bool) *mtproto.ImmutableUser {
	profile := &mtproto.UserData{Id: id, AccessHash: accessHash}
	if bot {
		profile.Bot = &mtproto.BotData{}
	}
	return &mtproto.ImmutableUser{User: profile}
}

func TestMiniBotAccessHashValidation(t *testing.T) {
	const (
		botID      = int64(990001)
		accessHash = int64(771122)
	)
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
	}
	if got, err := c.verifyMiniBot(miniBotInput(botID, accessHash)); err != nil || got != botID {
		t.Fatalf("valid bot: got=%d err=%v", got, err)
	}
	if _, err := c.verifyMiniBot(miniBotInput(botID, accessHash+1)); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("wrong access hash: got %v, want USER_ID_INVALID", err)
	}
	client.users[botID] = miniBotProfile(botID, accessHash, false)
	if _, err := c.verifyMiniBot(miniBotInput(botID, accessHash)); !errors.Is(err, mtproto.ErrBotInvalid) {
		t.Fatalf("non-bot profile: got %v, want BOT_INVALID", err)
	}
}

func openMiniBotAppsPostgres(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if !persist.PostgresEnabled() {
		if err := persist.OpenPostgresReadOnly(dsn); err != nil {
			t.Fatalf("open PostgreSQL schema: %v", err)
		}
	}
}

func TestMiniBotWebViewAndPermissionHandlersPostgres(t *testing.T) {
	openMiniBotAppsPostgres(t)
	uid := time.Now().UnixNano()
	botID := uid + 1
	accessHash := uid + 2
	client := &miniBotAppsUserClient{
		users: map[int64]*mtproto.ImmutableUser{
			botID: miniBotProfile(botID, accessHash, true),
		},
		botInfos: map[int64]*userpb.BotInfoData{
			botID: {MainAppUrl: wrapperspb.String("https://bot.example.test/main")},
		},
	}
	if err := persist.SetMiniBotPermission(uid, botID, false); err != nil {
		t.Fatalf("reset permission: %v", err)
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: uid},
	}
	bot := miniBotInput(botID, accessHash)
	result, err := c.MessagesRequestWebView(&mtproto.TLMessagesRequestWebView{
		Bot: bot, Url: wrapperspb.String("https://bot.example.test/app"), Platform: "ios", Fullscreen: true,
	})
	if err != nil {
		t.Fatalf("request webview: %v", err)
	}
	if result == nil || result.GetUrl() != "https://bot.example.test/app" || result.GetQueryId_FLAGINT64() == nil || result.GetQueryId_FLAGINT64().GetValue() <= 0 {
		t.Fatalf("webview result = %#v", result)
	}
	if _, err = c.MessagesProlongWebView(&mtproto.TLMessagesProlongWebView{QueryId: result.GetQueryId_FLAGINT64().GetValue()}); err != nil {
		t.Fatalf("prolong webview: %v", err)
	}
	allowed, err := c.BotsCanSendMessage(&mtproto.TLBotsCanSendMessage{Bot: bot})
	if err != nil || allowed != mtproto.BoolFalse {
		t.Fatalf("initial canSendMessage: got=%v err=%v", allowed, err)
	}
	if _, err = c.BotsAllowSendMessage(&mtproto.TLBotsAllowSendMessage{Bot: bot}); err != nil {
		t.Fatalf("allowSendMessage: %v", err)
	}
	allowed, err = c.BotsCanSendMessage(&mtproto.TLBotsCanSendMessage{Bot: bot})
	if err != nil || allowed != mtproto.BoolTrue {
		t.Fatalf("updated canSendMessage: got=%v err=%v", allowed, err)
	}

	button := mtproto.MakeTLKeyboardButtonWebView(&mtproto.KeyboardButton{
		Text: "Open", Url: "https://bot.example.test/button",
	}).GetData2()
	requested, err := c.BotsRequestWebViewButton(&mtproto.TLBotsRequestWebViewButton{
		UserId: bot, Button: button,
	})
	if err != nil {
		t.Fatalf("request webview button: %v", err)
	}
	gotButton, err := c.BotsGetRequestedWebViewButton(&mtproto.TLBotsGetRequestedWebViewButton{
		Bot: bot, WebappReqId: requested.GetWebappReqId(),
	})
	if err != nil {
		t.Fatalf("get requested webview button: %v", err)
	}
	if gotButton == nil || gotButton.GetText() != "Open" || gotButton.GetUrl() != "https://bot.example.test/button" {
		t.Fatalf("requested button = %#v", gotButton)
	}

	mainResult, err := c.MessagesRequestMainWebView(&mtproto.TLMessagesRequestMainWebView{Bot: bot, Platform: "android"})
	if err != nil {
		t.Fatalf("request main webview: %v", err)
	}
	if mainResult == nil || mainResult.GetUrl() != "https://bot.example.test/main" {
		t.Fatalf("main webview result = %#v", mainResult)
	}
}
