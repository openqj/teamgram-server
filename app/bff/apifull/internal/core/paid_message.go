// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)

package core

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

type paidMessagesRevenueProviderResponse struct {
	Verified     bool  `json:"verified"`
	CallerUserID int64 `json:"caller_user_id"`
	UserID       int64 `json:"user_id"`
	StarsAmount  int64 `json:"stars_amount"`
}

// RPCPaidMessageServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) paidMessagesUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) AccountGetPaidMessagesRevenue(in *mtproto.TLAccountGetPaidMessagesRevenue) (*mtproto.Account_PaidMessagesRevenue, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, c.paidMessagesUnavailable()
	}
	target := in.GetUserId()
	targetPeer := mtproto.FromInputUser(callerID, target)
	if targetPeer == nil || targetPeer.PeerId <= 0 ||
		(targetPeer.PeerType != mtproto.PEER_SELF && targetPeer.PeerType != mtproto.PEER_USER) {
		return nil, mtproto.ErrUserIdInvalid
	}
	if parent := in.GetParentPeer(); parent != nil && !validRevenuePeer(callerID, parent) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	targetID := targetPeer.PeerId
	var response paidMessagesRevenueProviderResponse
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation    string             `json:"operation"`
		CallerUserID int64              `json:"caller_user_id"`
		UserID       int64              `json:"user_id"`
		User         *mtproto.InputUser `json:"user"`
		ParentPeer   *mtproto.InputPeer `json:"parent_peer,omitempty"`
	}{"get_paid_messages_revenue", callerID, targetID, target, in.GetParentPeer()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.CallerUserID != callerID || response.UserID != targetID || response.StarsAmount < 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return mtproto.MakeTLAccountPaidMessagesRevenue(&mtproto.Account_PaidMessagesRevenue{
		StarsAmount: response.StarsAmount,
	}).To_Account_PaidMessagesRevenue(), nil
}

func (c *ApiFullCore) AccountToggleNoPaidMessagesException(in *mtproto.TLAccountToggleNoPaidMessagesException) (*mtproto.Bool, error) {
	_ = in
	return nil, c.paidMessagesUnavailable()
}

func (c *ApiFullCore) ChannelsUpdatePaidMessagesPrice(in *mtproto.TLChannelsUpdatePaidMessagesPrice) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	// Keep the method fail-closed for installations that have not configured
	// the canonical PostgreSQL channel store.
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetChannel() == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if in.GetChannel().GetChannelId() <= 0 || in.GetChannel().GetAccessHash() == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	stars := in.GetSendPaidMessagesStars()
	if stars < 0 {
		return nil, mtproto.ErrCurrencyTotalAmountInvalid
	}
	channel, err := domain.UpdatePaidMessagesPrice(userID, in.GetChannel().GetChannelId(), in.GetChannel().GetAccessHash(), stars, in.GetBroadcastMessagesAllowed())
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing), errors.Is(err, domain.ErrInvalidChannelAccessHash):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(channel, channel.Creator == userID)}), nil
}

func (c *ApiFullCore) AccountAddNoPaidMessagesException(in *mtproto.TLAccountAddNoPaidMessagesException) (*mtproto.Bool, error) {
	_ = in
	return nil, c.paidMessagesUnavailable()
}
