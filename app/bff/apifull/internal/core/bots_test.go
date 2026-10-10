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
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type botCommandsUserClient struct {
	user_client.UserClient
	commands []*mtproto.BotCommand
}

func (c *botCommandsUserClient) UserSetBotCommands(_ context.Context, in *userpb.TLUserSetBotCommands) (*mtproto.Bool, error) {
	c.commands = in.GetCommands()
	return mtproto.BoolTrue, nil
}

func (c *botCommandsUserClient) UserGetBotInfo(_ context.Context, _ *userpb.TLUserGetBotInfo) (*mtproto.BotInfo, error) {
	return mtproto.MakeTLBotInfo(&mtproto.BotInfo{Commands: c.commands}).To_BotInfo(), nil
}

func TestBotCommandsAndMenuButtonRoundTrip(t *testing.T) {
	userClient := &botCommandsUserClient{}
	c := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: userClient}},
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	scope := mtproto.MakeTLBotCommandScopeDefault(&mtproto.BotCommandScope{}).To_BotCommandScope()
	if _, err := c.BotsSetBotCommands(&mtproto.TLBotsSetBotCommands{
		Scope:    scope,
		Commands: []*mtproto.BotCommand{{Command: "start", Description: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.BotsGetBotCommands(&mtproto.TLBotsGetBotCommands{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDatas()) != 1 || got.Datas[0].GetCommand() != "start" || got.Datas[0].GetDescription() != "hi" {
		t.Fatalf("commands = %#v", got.GetDatas())
	}
	if _, err := c.BotsResetBotCommands(&mtproto.TLBotsResetBotCommands{Scope: scope}); err != nil {
		t.Fatal(err)
	}
	got, err = c.BotsGetBotCommands(&mtproto.TLBotsGetBotCommands{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetDatas() == nil || len(got.GetDatas()) != 0 {
		t.Fatalf("commands after reset = %#v, want empty vector", got)
	}

	user := &mtproto.InputUser{UserId: 7}
	button := mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: "Open", Url: "https://example.com"}).To_BotMenuButton()
	if _, err := c.BotsSetBotMenuButton(&mtproto.TLBotsSetBotMenuButton{UserId: user, Button: button}); err != nil {
		t.Fatal(err)
	}
	gotButton, err := c.BotsGetBotMenuButton(&mtproto.TLBotsGetBotMenuButton{UserId: user})
	if err != nil {
		t.Fatal(err)
	}
	if gotButton.GetText() != "Open" || gotButton.GetUrl() != "https://example.com" {
		t.Fatalf("menu button = %#v", gotButton)
	}
}

func TestBotWritesNilError(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	fns := []struct {
		name string
		call func() error
	}{
		{"reset commands", func() error { _, err := c.BotsResetBotCommands(nil); return err }},
		{"set bot info", func() error { _, err := c.BotsSetBotInfo(nil); return err }},
		{"create bot", func() error { _, err := c.BotsCreateBot(nil); return err }},
		{"export token", func() error { _, err := c.BotsExportBotToken(nil); return err }},
		{"edit access", func() error { _, err := c.BotsEditAccessSettings(nil); return err }},
		{"set join results", func() error { _, err := c.BotsSetJoinChatResults(nil); return err }},
		{"help updates", func() error { _, err := c.HelpSetBotUpdatesStatus(nil); return err }},
		{"custom request", func() error { _, err := c.BotsSendCustomRequest(nil); return err }},
		{"webhook answer", func() error { _, err := c.BotsAnswerWebhookJSONQuery(nil); return err }},
		{"broadcast rights", func() error { _, err := c.BotsSetBotBroadcastDefaultAdminRights(nil); return err }},
		{"group rights", func() error { _, err := c.BotsSetBotGroupDefaultAdminRights(nil); return err }},
		{"malformed group rights", func() error {
			_, err := c.BotsSetBotGroupDefaultAdminRights(&mtproto.TLBotsSetBotGroupDefaultAdminRights{
				AdminRights: &mtproto.ChatAdminRights{InviteUsers: true},
			})
			return err
		}},
		{"mismatched group rights", func() error {
			_, err := c.BotsSetBotGroupDefaultAdminRights(&mtproto.TLBotsSetBotGroupDefaultAdminRights{
				AdminRights: &mtproto.ChatAdminRights{PredicateName: "invalid", Constructor: mtproto.TLConstructor_CRC32_chatAdminRights},
			})
			return err
		}},
		{"attach menu", func() error { _, err := c.MessagesToggleBotInAttachMenu(nil); return err }},
		{"custom verification", func() error { _, err := c.BotsSetCustomVerification(nil); return err }},
	}
	for _, fn := range fns {
		err := fn.call()
		if fn.name == "reset commands" || fn.name == "create bot" || fn.name == "export token" {
			if !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
				t.Fatalf("%s: got %v, want INPUT_CONSTRUCTOR_INVALID", fn.name, err)
			}
		} else if fn.name == "set bot info" {
			if !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("%s: got %v, want INPUT_REQUEST_INVALID", fn.name, err)
			}
		} else if fn.name == "broadcast rights" || fn.name == "group rights" || fn.name == "malformed group rights" || fn.name == "mismatched group rights" {
			if !errors.Is(err, mtproto.ErrInputConstructorInvalid) {
				t.Fatalf("%s: got %v, want INPUT_CONSTRUCTOR_INVALID", fn.name, err)
			}
		} else if fn.name == "edit access" || fn.name == "set join results" || fn.name == "custom verification" || fn.name == "help updates" || fn.name == "custom request" || fn.name == "webhook answer" {
			if !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("%s: got %v, want METHOD_NOT_IMPL", fn.name, err)
			}
		} else if fn.name == "attach menu" {
			if !errors.Is(err, mtproto.ErrInputUserDeactivated) {
				t.Fatalf("%s: got %v, want INPUT_USER_DEACTIVATED", fn.name, err)
			}
		} else if err != nil {
			t.Fatalf("%s: got %v", fn.name, err)
		}
	}
}
