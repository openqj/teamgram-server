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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type paidMessagesRevenueProviderResponse struct {
	Verified     bool  `json:"verified"`
	CallerUserID int64 `json:"caller_user_id"`
	UserID       int64 `json:"user_id"`
	StarsAmount  int64 `json:"stars_amount"`
}

type paidMessagesExceptionProviderResponse struct {
	Verified     bool  `json:"verified"`
	CallerUserID int64 `json:"caller_user_id"`
	UserID       int64 `json:"user_id"`
	Result       *bool `json:"result"`
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
	// For a private chat, flags=0 is the user-facing shorthand for adding the
	// target to the caller's no-paid-messages privacy allowlist. That state is
	// authoritative in the User PostgreSQL service, so it does not require a
	// payment provider. Refunds and require_payment have external/monoforum
	// semantics and remain fail-closed when no parent peer is supplied.
	if in.GetParentPeer() == nil {
		if in.GetRefundCharged() || in.GetRequirePayment() {
			return nil, mtproto.ErrPaymentUnsupported
		}
		targetID, err = c.resolveNoPaidMessagesTarget(callerID, target)
		if err != nil {
			return nil, err
		}
		return c.persistNoPaidMessagesException(callerID, targetID)
	}
	var response paidMessagesExceptionProviderResponse
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation      string             `json:"operation"`
		CallerUserID   int64              `json:"caller_user_id"`
		UserID         int64              `json:"user_id"`
		User           *mtproto.InputUser `json:"user"`
		ParentPeer     *mtproto.InputPeer `json:"parent_peer,omitempty"`
		RefundCharged  bool               `json:"refund_charged"`
		RequirePayment bool               `json:"require_payment"`
	}{"toggle_no_paid_messages_exception", callerID, targetID, target, in.GetParentPeer(), in.GetRefundCharged(), in.GetRequirePayment()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.CallerUserID != callerID || response.UserID != targetID || response.Result == nil {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	if *response.Result {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (c *ApiFullCore) resolveNoPaidMessagesTarget(callerID int64, target *mtproto.InputUser) (int64, error) {
	if target == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	targetID := target.GetUserId()
	switch target.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		return callerID, nil
	case mtproto.Predicate_inputUser:
		if targetID <= 0 || target.GetAccessHash() == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.requestContext(), &userpb.TLUserGetImmutableUser{Id: targetID})
		if err != nil {
			return 0, err
		}
		if profile == nil || profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetAccessHash() != target.GetAccessHash() {
			return 0, mtproto.ErrUserIdInvalid
		}
		return targetID, nil
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}

func (c *ApiFullCore) persistNoPaidMessagesException(callerID, targetID int64) (*mtproto.Bool, error) {
	privacy, err := c.svcCtx.Dao.UserGetPrivacy(c.requestContext(), &userpb.TLUserGetPrivacy{
		UserId:  callerID,
		KeyType: mtproto.NO_PAID_MESSAGES,
	})
	if err != nil {
		return nil, err
	}
	if privacy == nil {
		return nil, mtproto.ErrInternalServerError
	}
	rules := make([]*mtproto.PrivacyRule, 0, len(privacy.GetDatas()))
	for _, rule := range privacy.GetDatas() {
		if rule == nil {
			return nil, mtproto.ErrPrivacyValueInvalid
		}
		// UserGetPrivacy may return objects backed by a shared RPC/cache value;
		// clone both repeated fields before changing the allowlist.
		rules = append(rules, &mtproto.PrivacyRule{
			PredicateName: rule.GetPredicateName(),
			Constructor:   rule.GetConstructor(),
			Users:         append([]int64(nil), rule.GetUsers()...),
			Chats:         append([]int64(nil), rule.GetChats()...),
		})
	}
	allowIndex := -1
	alreadyAllowed := false
	for i, rule := range rules {
		switch rule.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowUsers:
			if allowIndex < 0 {
				allowIndex = i
			}
			if containsPrivacyUser(rule.GetUsers(), targetID) {
				alreadyAllowed = true
			}
		case mtproto.Predicate_privacyValueDisallowUsers:
			// An explicit exception must override a stale deny entry.
			rule.Users = removePrivacyUser(rule.GetUsers(), targetID)
		}
	}
	if allowIndex < 0 {
		rules = append(rules, mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{targetID}}).To_PrivacyRule())
	} else if !alreadyAllowed {
		rules[allowIndex].Users = append(rules[allowIndex].Users, targetID)
	}
	if len(rules) == 0 {
		return nil, mtproto.ErrPrivacyValueInvalid
	}
	saved, err := c.svcCtx.Dao.UserSetPrivacy(c.requestContext(), &userpb.TLUserSetPrivacy{
		UserId:  callerID,
		KeyType: mtproto.NO_PAID_MESSAGES,
		Rules:   rules,
	})
	if err != nil {
		return nil, err
	}
	if saved == nil || saved.GetPredicateName() != mtproto.Predicate_boolTrue {
		return nil, mtproto.ErrInternalServerError
	}
	if c.svcCtx.Dao.SyncClient != nil {
		permAuthKeyID := int64(0)
		if c.MD != nil {
			permAuthKeyID = c.MD.GetPermAuthKeyId()
		}
		updates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePrivacy(&mtproto.Update{
			Key:   mtproto.ToPrivacyKey(mtproto.NO_PAID_MESSAGES),
			Rules: rules,
		}).To_Update())
		if _, syncErr := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.requestContext(), &syncpb.TLSyncUpdatesNotMe{
			UserId:        callerID,
			PermAuthKeyId: permAuthKeyID,
			Updates:       updates,
		}); syncErr != nil {
			return nil, fmt.Errorf("account.toggleNoPaidMessagesException: privacy rules were saved but sync update failed: %w", syncErr)
		}
	}
	return mtproto.BoolTrue, nil
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
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, c.paidMessagesUnavailable()
	}
	if in.GetUserId() == nil {
		return nil, mtproto.ErrInputUserDeactivated
	}
	// Resolve and authenticate the target before changing the caller's privacy
	// rules. InputUserSelf is the only valid zero-hash form; regular users must
	// carry the current access hash from the authoritative User service.
	targetID, err := c.resolveNoPaidMessagesTarget(callerID, in.GetUserId())
	if err != nil {
		return nil, err
	}
	// Refunds are an authoritative Stars/payment mutation. This local privacy
	// path cannot settle or reverse a charge, so never report success while
	// silently dropping the requested refund side effect. Validate the target
	// first so malformed Layer 229 input keeps its canonical error precedence.
	if in.GetRefundCharged() {
		return nil, mtproto.ErrPaymentUnsupported
	}

	return c.persistNoPaidMessagesException(callerID, targetID)
}

func containsPrivacyUser(users []int64, targetID int64) bool {
	for _, id := range users {
		if id == targetID {
			return true
		}
	}
	return false
}

func removePrivacyUser(users []int64, targetID int64) []int64 {
	filtered := users[:0]
	for _, id := range users {
		if id != targetID {
			filtered = append(filtered, id)
		}
	}
	return filtered
}
