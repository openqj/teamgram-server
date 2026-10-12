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
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
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

func starRefProgramFromRecord(record persist.StarRefProgramRecord) *mtproto.StarRefProgram {
	program := &mtproto.StarRefProgram{
		PredicateName:      mtproto.Predicate_starRefProgram,
		Constructor:        mtproto.TLConstructor_CRC32_starRefProgram,
		BotId:              record.BotID,
		CommissionPermille: record.CommissionPermille,
	}
	if record.DurationMonths != nil {
		program.DurationMonths = wrapperspb.Int32(*record.DurationMonths)
	}
	if record.EndDate != nil {
		program.EndDate = wrapperspb.Int32(*record.EndDate)
	}
	if record.DailyRevenueAmount != nil && record.DailyRevenueNanos != nil {
		program.DailyRevenuePerUser = &mtproto.StarsAmount{
			PredicateName: mtproto.Predicate_starsAmount,
			Constructor:   mtproto.TLConstructor_CRC32_starsAmount,
			Amount:        *record.DailyRevenueAmount,
			Nanos:         *record.DailyRevenueNanos,
		}
	}
	return program
}

func connectedStarRefFromRecord(record persist.StarRefConnectionRecord) *mtproto.ConnectedBotStarRef {
	connected := &mtproto.ConnectedBotStarRef{
		PredicateName:      mtproto.Predicate_connectedBotStarRef,
		Constructor:        mtproto.TLConstructor_CRC32_connectedBotStarRef,
		Revoked:            record.Revoked,
		Url:                record.URL,
		Date:               record.Date,
		BotId:              record.BotID,
		CommissionPermille: record.CommissionPermille,
		Participants:       record.Participants,
		Revenue:            record.Revenue,
	}
	if record.DurationMonths != nil {
		connected.DurationMonths = wrapperspb.Int32(*record.DurationMonths)
	}
	return connected
}

func (c *ApiFullCore) referralPeer(userID int64, peer *mtproto.InputPeer) (int32, int64, error) {
	return c.authorizePollPeer(userID, peer)
}

func (c *ApiFullCore) verifyReferralBot(input *mtproto.InputUser) (int64, string, error) {
	botID, err := c.verifyMiniBot(input)
	if err != nil {
		return 0, "", err
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return 0, "", err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetUsername() == "" {
		return 0, "", mtproto.ErrBotInvalid
	}
	return botID, profile.GetUser().GetUsername(), nil
}

func referralStateError(err error) error {
	switch {
	case errors.Is(err, persist.ErrStarRefStateUnavailable):
		return mtproto.ErrMethodNotImpl
	case errors.Is(err, persist.ErrStarRefProgramMissing):
		return mtproto.ErrBotInvalid
	case errors.Is(err, persist.ErrStarRefConnectionMissing):
		return mtproto.ErrPeerIdInvalid
	case errors.Is(err, persist.ErrStarRefLinkInvalid):
		return mtproto.ErrInputRequestInvalid
	default:
		return err
	}
}

func (c *ApiFullCore) hydrateStarRefUsers(ids []int64, callerID int64) ([]*mtproto.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	users, err := c.hydrateChannelUsers(callerID, ids...)
	if err != nil {
		return nil, err
	}
	if len(users) != len(ids) {
		return nil, mtproto.ErrUserIdInvalid
	}
	return users, nil
}

func (c *ApiFullCore) BotsUpdateStarRefProgram(in *mtproto.TLBotsUpdateStarRefProgram) (*mtproto.StarRefProgram, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetCommissionPermille() < 0 || in.GetCommissionPermille() > 1000 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if persist.PostgresEnabled() {
		botID, _, verifyErr := c.verifyReferralBot(in.GetBot())
		if verifyErr != nil {
			return nil, verifyErr
		}
		if botID != callerID {
			return nil, mtproto.ErrBotInvalid
		}
		var duration *int32
		if value := in.GetDurationMonths(); value != nil {
			n := value.GetValue()
			duration = &n
		}
		record, persistErr := persist.UpsertStarRefProgram(botID, in.GetCommissionPermille(), duration, time.Now().UTC())
		if persistErr != nil {
			return nil, referralStateError(persistErr)
		}
		return starRefProgramFromRecord(record), nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetLimit() <= 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	if persist.PostgresEnabled() {
		peerType, peerID, peerErr := c.referralPeer(uid, in.GetPeer())
		if peerErr != nil {
			return nil, peerErr
		}
		offsetDate := int32(0)
		if value := in.GetOffsetDate(); value != nil {
			offsetDate = value.GetValue()
			if offsetDate < 0 {
				return nil, mtproto.ErrOffsetInvalid
			}
		}
		offsetLink := ""
		if value := in.GetOffsetLink(); value != nil {
			offsetLink = strings.TrimSpace(value.GetValue())
			if offsetLink != "" && !persist.ValidStarReferralLinkForAPI(offsetLink) {
				return nil, mtproto.ErrOffsetInvalid
			}
		}
		records, count, persistErr := persist.ListStarRefConnections(uid, peerType, peerID, offsetDate, offsetLink, int(in.GetLimit()))
		if persistErr != nil {
			return nil, referralStateError(persistErr)
		}
		// An empty local projection is not an authoritative answer. The
		// PostgreSQL path is only enabled for provisioned referral rows; fail
		// closed until the control plane publishes at least one connection.
		if count == 0 {
			return nil, mtproto.ErrMethodNotImpl
		}
		ids := make([]int64, 0, len(records))
		connected := make([]*mtproto.ConnectedBotStarRef, 0, len(records))
		for _, record := range records {
			ids = append(ids, record.BotID)
			connected = append(connected, connectedStarRefFromRecord(record))
		}
		users, hydrateErr := c.hydrateStarRefUsers(ids, uid)
		if hydrateErr != nil {
			return nil, hydrateErr
		}
		return &mtproto.Payments_ConnectedStarRefBots{
			PredicateName: mtproto.Predicate_payments_connectedStarRefBots,
			Constructor:   mtproto.TLConstructor_CRC32_payments_connectedStarRefBots,
			Count:         int32(count),
			ConnectedBots: connected,
			Users:         users,
		}, nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if persist.PostgresEnabled() {
		peerType, peerID, peerErr := c.referralPeer(uid, in.GetPeer())
		if peerErr != nil {
			return nil, peerErr
		}
		botID, _, verifyErr := c.verifyReferralBot(in.GetBot())
		if verifyErr != nil {
			return nil, verifyErr
		}
		record, found, persistErr := persist.LoadStarRefConnection(uid, int64(peerType), peerID, botID)
		if persistErr != nil {
			return nil, referralStateError(persistErr)
		}
		if !found {
			return nil, referralStateError(persist.ErrStarRefConnectionMissing)
		}
		result := &mtproto.Payments_ConnectedStarRefBots{
			PredicateName: mtproto.Predicate_payments_connectedStarRefBots,
			Constructor:   mtproto.TLConstructor_CRC32_payments_connectedStarRefBots,
		}
		result.Count = 1
		result.ConnectedBots = []*mtproto.ConnectedBotStarRef{connectedStarRefFromRecord(record)}
		users, hydrateErr := c.hydrateStarRefUsers([]int64{botID}, uid)
		if hydrateErr != nil {
			return nil, hydrateErr
		}
		result.Users = users
		return result, nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	if persist.PostgresEnabled() {
		if in.GetPeer() != nil {
			if _, _, peerErr := c.referralPeer(uid, in.GetPeer()); peerErr != nil {
				return nil, peerErr
			}
		}
		limit := int(in.GetLimit())
		if limit == 0 {
			limit = 100
		}
		offset := strings.TrimSpace(in.GetOffset())
		records, count, nextOffset, persistErr := persist.ListActiveStarRefPrograms(offset, limit, in.GetOrderByRevenue(), in.GetOrderByDate())
		if persistErr != nil {
			return nil, referralStateError(persistErr)
		}
		// Do not turn an unprovisioned catalogue into a successful empty
		// response. A control-plane row is required before this method can be
		// considered available on the PostgreSQL path.
		if count == 0 {
			return nil, mtproto.ErrMethodNotImpl
		}
		ids := make([]int64, 0, len(records))
		programs := make([]*mtproto.StarRefProgram, 0, len(records))
		for _, record := range records {
			ids = append(ids, record.BotID)
			programs = append(programs, starRefProgramFromRecord(record))
		}
		users, hydrateErr := c.hydrateStarRefUsers(ids, uid)
		if hydrateErr != nil {
			return nil, hydrateErr
		}
		result := &mtproto.Payments_SuggestedStarRefBots{
			PredicateName: mtproto.Predicate_payments_suggestedStarRefBots,
			Constructor:   mtproto.TLConstructor_CRC32_payments_suggestedStarRefBots,
			Count:         int32(count),
			SuggestedBots: programs,
			Users:         users,
		}
		if nextOffset != "" {
			result.NextOffset = wrapperspb.String(nextOffset)
		}
		return result, nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetBot() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if persist.PostgresEnabled() {
		peerType, peerID, peerErr := c.referralPeer(uid, in.GetPeer())
		if peerErr != nil {
			return nil, peerErr
		}
		botID, username, verifyErr := c.verifyReferralBot(in.GetBot())
		if verifyErr != nil {
			return nil, verifyErr
		}
		if existing, found, loadErr := persist.LoadStarRefConnection(uid, int64(peerType), peerID, botID); loadErr != nil {
			return nil, referralStateError(loadErr)
		} else if found {
			if existing.Revoked {
				record, updateErr := persist.ConnectStarRefBot(uid, int64(peerType), peerID, botID, existing.URL, time.Now().UTC())
				if updateErr != nil {
					return nil, referralStateError(updateErr)
				}
				existing = record
			}
			users, hydrateErr := c.hydrateStarRefUsers([]int64{botID}, uid)
			if hydrateErr != nil {
				return nil, hydrateErr
			}
			return &mtproto.Payments_ConnectedStarRefBots{
				PredicateName: mtproto.Predicate_payments_connectedStarRefBots,
				Constructor:   mtproto.TLConstructor_CRC32_payments_connectedStarRefBots,
				Count:         1, ConnectedBots: []*mtproto.ConnectedBotStarRef{connectedStarRefFromRecord(existing)}, Users: users,
			}, nil
		}
		referralURL, urlErr := persist.NewStarReferralURL(username, botID)
		if urlErr != nil {
			return nil, referralStateError(urlErr)
		}
		record, persistErr := persist.ConnectStarRefBot(uid, int64(peerType), peerID, botID, referralURL, time.Now().UTC())
		if persistErr != nil {
			return nil, referralStateError(persistErr)
		}
		users, hydrateErr := c.hydrateStarRefUsers([]int64{botID}, uid)
		if hydrateErr != nil {
			return nil, hydrateErr
		}
		return &mtproto.Payments_ConnectedStarRefBots{
			PredicateName: mtproto.Predicate_payments_connectedStarRefBots,
			Constructor:   mtproto.TLConstructor_CRC32_payments_connectedStarRefBots,
			Count:         1, ConnectedBots: []*mtproto.ConnectedBotStarRef{connectedStarRefFromRecord(record)}, Users: users,
		}, nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetLink() == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if persist.PostgresEnabled() {
		peerType, peerID, peerErr := c.referralPeer(uid, in.GetPeer())
		if peerErr != nil {
			return nil, peerErr
		}
		record, persistErr := persist.EditStarRefConnection(uid, int64(peerType), peerID, in.GetLink(), in.GetRevoked())
		if persistErr != nil {
			return nil, referralStateError(persistErr)
		}
		users, hydrateErr := c.hydrateStarRefUsers([]int64{record.BotID}, uid)
		if hydrateErr != nil {
			return nil, hydrateErr
		}
		return &mtproto.Payments_ConnectedStarRefBots{
			PredicateName: mtproto.Predicate_payments_connectedStarRefBots,
			Constructor:   mtproto.TLConstructor_CRC32_payments_connectedStarRefBots,
			Count:         1, ConnectedBots: []*mtproto.ConnectedBotStarRef{connectedStarRefFromRecord(record)}, Users: users,
		}, nil
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
