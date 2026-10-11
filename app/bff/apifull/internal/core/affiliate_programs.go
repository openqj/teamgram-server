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
	"github.com/teamgram/proto/mtproto"
)

// RPCAffiliateProgramsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) affiliateProgramsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func validStarRefProgram(program *mtproto.StarRefProgram) bool {
	if program == nil || program.GetPredicateName() != mtproto.Predicate_starRefProgram || program.GetBotId() <= 0 || program.GetCommissionPermille() < 0 || program.GetCommissionPermille() > 1000 {
		return false
	}
	if duration := program.GetDurationMonths(); duration != nil && (duration.GetValue() <= 0 || duration.GetValue() > 120) {
		return false
	}
	if endDate := program.GetEndDate(); endDate != nil && endDate.GetValue() < 0 {
		return false
	}
	if revenue := program.GetDailyRevenuePerUser(); revenue != nil && (revenue.GetPredicateName() != mtproto.Predicate_starsAmount || revenue.GetAmount() < 0 || revenue.GetNanos() < 0) {
		return false
	}
	return true
}

func validAffiliateUsers(users []*mtproto.User) bool {
	if len(users) > 10000 {
		return false
	}
	for _, user := range users {
		if user == nil || user.GetPredicateName() == "" {
			return false
		}
	}
	return true
}

func validConnectedStarRefBots(result *mtproto.Payments_ConnectedStarRefBots) bool {
	if result == nil || result.GetPredicateName() != mtproto.Predicate_payments_connectedStarRefBots || result.GetCount() < 0 || len(result.GetConnectedBots()) > 10000 || !validAffiliateUsers(result.GetUsers()) {
		return false
	}
	for _, bot := range result.GetConnectedBots() {
		if bot == nil || bot.GetPredicateName() != mtproto.Predicate_connectedBotStarRef || bot.GetBotId() <= 0 || bot.GetCommissionPermille() < 0 || bot.GetCommissionPermille() > 1000 || bot.GetUrl() == "" || bot.GetDate() < 0 || bot.GetParticipants() < 0 || bot.GetRevenue() < 0 {
			return false
		}
	}
	return true
}

func validSuggestedStarRefBots(result *mtproto.Payments_SuggestedStarRefBots) bool {
	if result == nil || result.GetPredicateName() != mtproto.Predicate_payments_suggestedStarRefBots || result.GetCount() < 0 || len(result.GetSuggestedBots()) > 10000 || !validAffiliateUsers(result.GetUsers()) {
		return false
	}
	for _, program := range result.GetSuggestedBots() {
		if !validStarRefProgram(program) {
			return false
		}
	}
	return true
}

func (c *ApiFullCore) BotsUpdateStarRefProgram(in *mtproto.TLBotsUpdateStarRefProgram) (*mtproto.StarRefProgram, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetCommissionPermille() < 0 || in.GetCommissionPermille() > 1000 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.StarRefProgram)
	if err := c.callBusinessProvider("bots.updateStarRefProgram", map[string]any{
		"bot": in.GetBot(), "commission_permille": in.GetCommissionPermille(), "duration_months": in.GetDurationMonths(),
	}, result); err != nil {
		return nil, err
	}
	if !validStarRefProgram(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) PaymentsGetConnectedStarRefBots(in *mtproto.TLPaymentsGetConnectedStarRefBots) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	result := new(mtproto.Payments_ConnectedStarRefBots)
	if err := c.callBusinessProvider("payments.getConnectedStarRefBots", map[string]any{
		"peer": in.GetPeer(), "offset_date": in.GetOffsetDate(), "offset_link": in.GetOffsetLink(), "limit": in.GetLimit(),
	}, result); err != nil {
		return nil, err
	}
	if !validConnectedStarRefBots(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) PaymentsGetConnectedStarRefBot(in *mtproto.TLPaymentsGetConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.Payments_ConnectedStarRefBots)
	if err := c.callBusinessProvider("payments.getConnectedStarRefBot", map[string]any{
		"peer": in.GetPeer(), "bot": in.GetBot(),
	}, result); err != nil {
		return nil, err
	}
	if !validConnectedStarRefBots(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) PaymentsGetSuggestedStarRefBots(in *mtproto.TLPaymentsGetSuggestedStarRefBots) (*mtproto.Payments_SuggestedStarRefBots, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	result := new(mtproto.Payments_SuggestedStarRefBots)
	if err := c.callBusinessProvider("payments.getSuggestedStarRefBots", map[string]any{
		"order_by_revenue": in.GetOrderByRevenue(), "order_by_date": in.GetOrderByDate(), "peer": in.GetPeer(), "offset": in.GetOffset(), "limit": in.GetLimit(),
	}, result); err != nil {
		return nil, err
	}
	if !validSuggestedStarRefBots(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) PaymentsConnectStarRefBot(in *mtproto.TLPaymentsConnectStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.Payments_ConnectedStarRefBots)
	if err := c.callBusinessProvider("payments.connectStarRefBot", map[string]any{
		"peer": in.GetPeer(), "bot": in.GetBot(),
	}, result); err != nil {
		return nil, err
	}
	if !validConnectedStarRefBots(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) PaymentsEditConnectedStarRefBot(in *mtproto.TLPaymentsEditConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetLink() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.Payments_ConnectedStarRefBots)
	if err := c.callBusinessProvider("payments.editConnectedStarRefBot", map[string]any{
		"revoked": in.GetRevoked(), "peer": in.GetPeer(), "link": in.GetLink(),
	}, result); err != nil {
		return nil, err
	}
	if !validConnectedStarRefBots(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}
