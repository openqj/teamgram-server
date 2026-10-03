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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCAccentColorsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) AccountUpdateColor(in *mtproto.TLAccountUpdateColor) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}

	var color int32
	var backgroundEmojiID int64
	if peerColor := in.GetColor_FLAGPEERCOLOR(); peerColor != nil {
		predicate := peerColor.GetPredicateName()
		constructor := peerColor.GetConstructor()
		switch {
		case predicate == mtproto.Predicate_peerColorCollectible || predicate == mtproto.Predicate_inputPeerColorCollectible || constructor == mtproto.TLConstructor_CRC32_peerColorCollectible || constructor == mtproto.TLConstructor_CRC32_inputPeerColorCollectible:
			return nil, mtproto.ErrMethodNotImpl
		case predicate == mtproto.Predicate_peerColor || constructor == mtproto.TLConstructor_CRC32_peerColor:
		default:
			return nil, mtproto.ErrInputConstructorInvalid
		}
		if peerColor.GetColor() != nil {
			color = peerColor.GetColor().GetValue()
		}
		if peerColor.GetBackgroundEmojiId_FLAGINT64() != nil {
			backgroundEmojiID = peerColor.GetBackgroundEmojiId_FLAGINT64().GetValue()
		}
	} else {
		if in.GetColor_FLAGINT32() != nil {
			color = in.GetColor_FLAGINT32().GetValue()
		}
		if in.GetBackgroundEmojiId() != nil {
			backgroundEmojiID = in.GetBackgroundEmojiId().GetValue()
		}
	}

	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	updated, err := d.UserClient.UserSetColor(ctx, &userpb.TLUserSetColor{
		UserId:            uid,
		ForProfile:        in.GetForProfile(),
		Color:             color,
		BackgroundEmojiId: backgroundEmojiID,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || !mtproto.FromBool(updated) {
		return nil, mtproto.ErrInternalServerError
	}
	return updated, nil
}

func (c *ApiFullCore) AccountGetDefaultBackgroundEmojis(in *mtproto.TLAccountGetDefaultBackgroundEmojis) (*mtproto.EmojiList, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

type builtinPeerColor struct {
	id    int32
	color int32
}

// Keep the catalog aligned with the seven fallback colors used by the web client.
var builtinPeerColors = []builtinPeerColor{
	{id: 0, color: 0xD45246},
	{id: 1, color: 0xF68136},
	{id: 2, color: 0x6C61DF},
	{id: 3, color: 0x46BA43},
	{id: 4, color: 0x5CAFFA},
	{id: 5, color: 0x408ACF},
	{id: 6, color: 0xD95574},
}

func peerColorSet(color int32) *mtproto.Help_PeerColorSet {
	return mtproto.MakeTLHelpPeerColorSet(&mtproto.Help_PeerColorSet{
		Colors: []int32{color},
	}).To_Help_PeerColorSet()
}

func peerProfileColorSet(color int32) *mtproto.Help_PeerColorSet {
	return mtproto.MakeTLHelpPeerColorProfileSet(&mtproto.Help_PeerColorSet{
		PaletteColors: []int32{color},
		BgColors:      []int32{color},
		StoryColors:   []int32{color},
	}).To_Help_PeerColorSet()
}

func peerColorOption(color builtinPeerColor, profile bool) *mtproto.Help_PeerColorOption {
	set := peerColorSet(color.color)
	if profile {
		set = peerProfileColorSet(color.color)
	}

	return mtproto.MakeTLHelpPeerColorOption(&mtproto.Help_PeerColorOption{
		ColorId: color.id,
		Colors:  set,
	}).To_Help_PeerColorOption()
}

func accentPeerColors(profile bool) []*mtproto.Help_PeerColorOption {
	out := make([]*mtproto.Help_PeerColorOption, 0, len(builtinPeerColors))
	for _, color := range builtinPeerColors {
		out = append(out, peerColorOption(color, profile))
	}
	return out
}

func (c *ApiFullCore) HelpGetPeerColors(in *mtproto.TLHelpGetPeerColors) (*mtproto.Help_PeerColors, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLHelpPeerColors(&mtproto.Help_PeerColors{
		Colors: accentPeerColors(false),
	}).To_Help_PeerColors(), nil
}

func (c *ApiFullCore) HelpGetPeerProfileColors(in *mtproto.TLHelpGetPeerProfileColors) (*mtproto.Help_PeerColors, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return mtproto.MakeTLHelpPeerColors(&mtproto.Help_PeerColors{
		Colors: accentPeerColors(true),
	}).To_Help_PeerColors(), nil
}

func (c *ApiFullCore) ChannelsUpdateColor(in *mtproto.TLChannelsUpdateColor) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetChannel().GetChannelId() == 0 || in.GetChannel().GetAccessHash() == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	channelID := in.GetChannel().GetChannelId()
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok || ch.AccessHash != in.GetChannel().GetAccessHash() {
		return nil, mtproto.ErrChannelInvalid
	}
	var color *int32
	if in.GetColor_FLAGINT32() != nil {
		value := in.GetColor_FLAGINT32().GetValue()
		color = &value
	}
	var backgroundEmojiID *int64
	if in.GetBackgroundEmojiId() != nil {
		value := in.GetBackgroundEmojiId().GetValue()
		backgroundEmojiID = &value
	}
	if err = domain.UpdateChannelColor(uid, channelID, in.GetForProfile(), color, backgroundEmojiID); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	ch, ok, err = domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(ch, ch.Creator == uid)}), nil
}
