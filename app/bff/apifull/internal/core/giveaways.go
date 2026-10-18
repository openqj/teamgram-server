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
	"encoding/json"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

type giveawayInfoProviderResponse struct {
	Verified bool                           `json:"verified"`
	UserID   int64                          `json:"user_id"`
	Info     *mtproto.Payments_GiveawayInfo `json:"info"`
}

func normalizeGiveawayInfo(info *mtproto.Payments_GiveawayInfo) bool {
	if info == nil {
		return false
	}
	if info.GetPredicateName() == "" {
		// Provider JSON does not carry the TL union marker. Infer the result
		// constructor from fields that only payments.giveawayInfoResults has.
		results := info.GetWinner() || info.GetRefunded() || info.GetGiftCodeSlug() != nil ||
			info.GetStarsPrize() != nil || info.GetFinishDate() != 0 || info.GetWinnersCount() != 0 ||
			info.GetActivatedCount_FLAGINT32() != nil || info.GetActivatedCount_INT32() != 0
		if results {
			mtproto.MakeTLPaymentsGiveawayInfoResults(info).To_Payments_GiveawayInfo()
		} else {
			mtproto.MakeTLPaymentsGiveawayInfo(info).To_Payments_GiveawayInfo()
		}
	}
	if info.GetStartDate() < 0 || info.GetFinishDate() < 0 || info.GetWinnersCount() < 0 || info.GetActivatedCount_INT32() < 0 {
		return false
	}
	if stars := info.GetStarsPrize(); stars != nil && stars.GetValue() < 0 {
		return false
	}
	if joined := info.GetJoinedTooEarlyDate(); joined != nil && joined.GetValue() < 0 {
		return false
	}
	if activated := info.GetActivatedCount_FLAGINT32(); activated != nil && activated.GetValue() < 0 {
		return false
	}
	if country := info.GetDisallowedCountry(); country != nil && strings.TrimSpace(country.GetValue()) == "" {
		return false
	}
	if slug := info.GetGiftCodeSlug(); slug != nil && strings.TrimSpace(slug.GetValue()) == "" {
		return false
	}
	switch info.GetPredicateName() {
	case mtproto.Predicate_payments_giveawayInfo:
		return !info.GetWinner() && !info.GetRefunded() && info.GetGiftCodeSlug() == nil && info.GetStarsPrize() == nil &&
			info.GetFinishDate() == 0 && info.GetWinnersCount() == 0 && info.GetActivatedCount_FLAGINT32() == nil && info.GetActivatedCount_INT32() == 0
	case mtproto.Predicate_payments_giveawayInfoResults:
		return !info.GetParticipating() && !info.GetPreparingResults() && info.GetJoinedTooEarlyDate() == nil &&
			info.GetAdminDisallowedChatId() == nil && info.GetDisallowedCountry() == nil
	default:
		return false
	}
}

// RPCGiveawaysServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) PaymentsGetPremiumGiftCodeOptions(in *mtproto.TLPaymentsGetPremiumGiftCodeOptions) (*mtproto.Vector_PremiumGiftCodeOption, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetBoostPeer() != nil && !validRevenuePeer(uid, in.GetBoostPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var response struct {
		Verified bool                             `json:"verified"`
		UserID   int64                            `json:"user_id"`
		Options  []*mtproto.PremiumGiftCodeOption `json:"options"`
	}
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string             `json:"operation"`
		UserID    int64              `json:"user_id"`
		BoostPeer *mtproto.InputPeer `json:"boost_peer,omitempty"`
	}{"get_premium_gift_code_options", uid, in.GetBoostPeer()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || len(response.Options) == 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	for _, option := range response.Options {
		if option == nil || option.GetUsers() <= 0 || option.GetMonths() <= 0 || option.GetMonths() > 36 || option.GetAmount() < 0 || strings.TrimSpace(option.GetCurrency()) == "" {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if product := option.GetStoreProduct(); product != nil && strings.TrimSpace(product.GetValue()) == "" {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if option.GetStoreQuantity() != nil && option.GetStoreQuantity().GetValue() <= 0 {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if option.GetPredicateName() == "" {
			mtproto.MakeTLPremiumGiftCodeOption(option).To_PremiumGiftCodeOption()
		}
		if option.GetPredicateName() != mtproto.Predicate_premiumGiftCodeOption {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
	}
	return &mtproto.Vector_PremiumGiftCodeOption{Datas: response.Options}, nil
}

func (c *ApiFullCore) PaymentsGetGiveawayInfo(in *mtproto.TLPaymentsGetGiveawayInfo) (*mtproto.Payments_GiveawayInfo, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateThreadRequest(uid, in.GetPeer(), in.GetMsgId()); err != nil {
		return nil, err
	}
	var response giveawayInfoProviderResponse
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string             `json:"operation"`
		UserID    int64              `json:"user_id"`
		Peer      *mtproto.InputPeer `json:"peer"`
		MsgID     int32              `json:"msg_id"`
	}{"get_giveaway_info", uid, in.GetPeer(), in.GetMsgId()}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || !normalizeGiveawayInfo(response.Info) {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	return response.Info, nil
}

func (c *ApiFullCore) PaymentsLaunchPrepaidGiveaway(in *mtproto.TLPaymentsLaunchPrepaidGiveaway) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || !validRevenuePeer(uid, in.GetPeer()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetGiveawayId() <= 0 || in.GetPurpose() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	purpose := in.GetPurpose()
	if purpose.GetPredicateName() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	switch purpose.GetPredicateName() {
	case mtproto.Predicate_inputStorePaymentPremiumGiveaway, mtproto.Predicate_inputStorePaymentStarsGiveaway:
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}
	if purpose.GetAmount() < 0 || purpose.GetStars() < 0 || purpose.GetUntilDate() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var raw json.RawMessage
	if err = c.callBusinessProvider("payments.launchPrepaidGiveaway", map[string]any{
		"peer": in.GetPeer(), "giveaway_id": in.GetGiveawayId(), "purpose": purpose,
	}, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, mtproto.ErrInternalServerError
	}
	var updates mtproto.Updates
	if err = json.Unmarshal(raw, &updates); err != nil || updates.GetPredicateName() == "" {
		return nil, mtproto.ErrInternalServerError
	}
	switch updates.GetPredicateName() {
	case mtproto.Predicate_updates,
		mtproto.Predicate_updatesCombined,
		mtproto.Predicate_updatesTooLong,
		mtproto.Predicate_updateShort,
		mtproto.Predicate_updateShortMessage,
		mtproto.Predicate_updateShortChatMessage,
		mtproto.Predicate_updateShortSentMessage,
		mtproto.Predicate_updateAccountResetAuthorization:
	default:
		return nil, mtproto.ErrInternalServerError
	}
	return &updates, nil
}

func (c *ApiFullCore) PaymentsGetStarsGiveawayOptions(in *mtproto.TLPaymentsGetStarsGiveawayOptions) (*mtproto.Vector_StarsGiveawayOption, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	var response struct {
		Verified bool                           `json:"verified"`
		UserID   int64                          `json:"user_id"`
		Options  []*mtproto.StarsGiveawayOption `json:"options"`
	}
	err = c.callStarsRevenueProvider(c.secretContext(), struct {
		Operation string `json:"operation"`
		UserID    int64  `json:"user_id"`
	}{"get_stars_giveaway_options", uid}, &response)
	if err != nil {
		return nil, err
	}
	if !response.Verified || response.UserID != uid || len(response.Options) == 0 {
		return nil, mtproto.ErrPaymentProviderInvalid
	}
	for _, option := range response.Options {
		if option == nil || option.GetStars() <= 0 || option.GetYearlyBoosts() < 0 || option.GetAmount() < 0 || strings.TrimSpace(option.GetCurrency()) == "" {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		// The TL constructor requires a winner-tier vector. An empty vector
		// cannot describe a giveaway and must not cross the wire as success.
		if len(option.GetWinners()) == 0 {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		if option.GetStoreProduct() != nil && strings.TrimSpace(option.GetStoreProduct().GetValue()) == "" {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
		for _, winner := range option.GetWinners() {
			if winner == nil || winner.GetUsers() <= 0 || winner.GetPerUserStars() <= 0 {
				return nil, mtproto.ErrPaymentProviderInvalid
			}
			if winner.GetPredicateName() == "" {
				mtproto.MakeTLStarsGiveawayWinnersOption(winner).To_StarsGiveawayWinnersOption()
			}
			if winner.GetPredicateName() != mtproto.Predicate_starsGiveawayWinnersOption {
				return nil, mtproto.ErrPaymentProviderInvalid
			}
		}
		if option.GetPredicateName() == "" {
			mtproto.MakeTLStarsGiveawayOption(option).To_StarsGiveawayOption()
		}
		if option.GetPredicateName() != mtproto.Predicate_starsGiveawayOption {
			return nil, mtproto.ErrPaymentProviderInvalid
		}
	}
	return &mtproto.Vector_StarsGiveawayOption{Datas: response.Options}, nil
}
