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

package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type premiumAccessUserClient struct {
	userclient.UserClient
	profiles map[int64]*mtproto.ImmutableUser
}

func (c *premiumAccessUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if profile := c.profiles[in.GetId()]; profile != nil {
		return profile, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

func premiumAccessProfile(id int64, premium bool) *mtproto.ImmutableUser {
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: id, Premium: premium},
	}).To_ImmutableUser()
}

func premiumAccessCore(userID int64, premium bool) *ApiFullCore {
	ctx := context.Background()
	return &ApiFullCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: &premiumAccessUserClient{
			profiles: map[int64]*mtproto.ImmutableUser{userID: premiumAccessProfile(userID, premium)},
		}}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID, PermAuthKeyId: 1},
	}
}

func TestBusinessPremiumGateRejectsNonPremiumUser(t *testing.T) {
	c := premiumAccessCore(6, false)
	inputUser := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7}).To_InputUser()
	inputPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 8}).To_InputPeer()
	cases := []struct {
		name string
		call func() error
	}{
		{"account.updateConnectedBot", func() error {
			_, err := c.AccountUpdateConnectedBot(&mtproto.TLAccountUpdateConnectedBot{Bot: inputUser})
			return err
		}},
		{"account.getConnectedBots", func() error {
			_, err := c.AccountGetConnectedBots(&mtproto.TLAccountGetConnectedBots{})
			return err
		}},
		{"account.toggleConnectedBotPaused", func() error {
			_, err := c.AccountToggleConnectedBotPaused(&mtproto.TLAccountToggleConnectedBotPaused{Peer: inputPeer})
			return err
		}},
		{"account.disablePeerConnectedBot", func() error {
			_, err := c.AccountDisablePeerConnectedBot(&mtproto.TLAccountDisablePeerConnectedBot{Peer: inputPeer})
			return err
		}},
		{"messages.getQuickReplies", func() error {
			_, err := c.MessagesGetQuickReplies(&mtproto.TLMessagesGetQuickReplies{})
			return err
		}},
		{"messages.reorderQuickReplies", func() error {
			_, err := c.MessagesReorderQuickReplies(&mtproto.TLMessagesReorderQuickReplies{Order: []int32{1}})
			return err
		}},
		{"messages.checkQuickReplyShortcut", func() error {
			_, err := c.MessagesCheckQuickReplyShortcut(&mtproto.TLMessagesCheckQuickReplyShortcut{Shortcut: "probe"})
			return err
		}},
		{"messages.editQuickReplyShortcut", func() error {
			_, err := c.MessagesEditQuickReplyShortcut(&mtproto.TLMessagesEditQuickReplyShortcut{ShortcutId: 1, Shortcut: "probe"})
			return err
		}},
		{"messages.deleteQuickReplyShortcut", func() error {
			_, err := c.MessagesDeleteQuickReplyShortcut(&mtproto.TLMessagesDeleteQuickReplyShortcut{ShortcutId: 1})
			return err
		}},
		{"messages.getQuickReplyMessages", func() error {
			_, err := c.MessagesGetQuickReplyMessages(&mtproto.TLMessagesGetQuickReplyMessages{ShortcutId: 1})
			return err
		}},
		{"messages.sendQuickReplyMessages", func() error {
			_, err := c.MessagesSendQuickReplyMessages(&mtproto.TLMessagesSendQuickReplyMessages{
				Peer:       inputPeer,
				ShortcutId: 1,
				Id:         []int32{1},
				RandomId:   []int64{1},
			})
			return err
		}},
		{"messages.deleteQuickReplyMessages", func() error {
			_, err := c.MessagesDeleteQuickReplyMessages(&mtproto.TLMessagesDeleteQuickReplyMessages{ShortcutId: 1, Id: []int32{1}})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, mtproto.ErrPremiumAccountRequired) {
				t.Fatalf("error = %v, want PREMIUM_ACCOUNT_REQUIRED", err)
			}
		})
	}
}
