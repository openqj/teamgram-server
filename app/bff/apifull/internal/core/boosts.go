// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCBoostsServer: Layer 229 durable boost inventory and target state.

func (c *ApiFullCore) boostsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

type boostPeerRef struct {
	Scope    string
	PeerType int32
	PeerID   int64
}

func boostPeerRefFromInput(self int64, scope string, peer *mtproto.InputPeer) (boostPeerRef, error) {
	if peer == nil {
		return boostPeerRef{}, mtproto.ErrInputConstructorInvalid
	}
	peerType, peerID := apifullPeerTypeID(self, peer)
	if peerType <= 0 || peerID <= 0 {
		// Username and access-hash resolution belongs to the authoritative
		// users/channels services. Keep this local handler fail-closed until
		// that resolver can provide a canonical peer id.
		return boostPeerRef{}, mtproto.ErrMethodNotImpl
	}
	return boostPeerRef{Scope: scope, PeerType: peerType, PeerID: peerID}, nil
}

func boostInputChannel(self int64, channel *mtproto.InputChannel) (boostPeerRef, error) {
	if channel == nil || channel.GetChannelId() <= 0 {
		return boostPeerRef{}, mtproto.ErrInputConstructorInvalid
	}
	if channel.GetPeer() != nil {
		return boostPeerRefFromInput(self, domain.BoostScopePremium, channel.GetPeer())
	}
	return boostPeerRef{Scope: domain.BoostScopePremium, PeerType: mtproto.PEER_CHANNEL, PeerID: channel.GetChannelId()}, nil
}

func boostInputUserID(self int64, user *mtproto.InputUser) (int64, error) {
	if user == nil {
		return 0, mtproto.ErrInputConstructorInvalid
	}
	if user.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return self, nil
	}
	if user.GetUserId() <= 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return user.GetUserId(), nil
}

func boostDomainError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrBoostSlotInvalid), errors.Is(err, domain.ErrBoostSlotUnavailable):
		return mtproto.ErrPremiumAccountRequired
	case errors.Is(err, domain.ErrBoostForbidden):
		return mtproto.ErrChatAdminRequired
	case errors.Is(err, domain.ErrBoostTargetNotFound):
		return mtproto.ErrPeerIdInvalid
	default:
		return err
	}
}

func boostPeer(peerType int32, id int64) *mtproto.Peer {
	switch peerType {
	case mtproto.PEER_USER:
		return mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: id}).To_Peer()
	case mtproto.PEER_CHAT:
		return mtproto.MakeTLPeerChat(&mtproto.Peer{ChatId: id}).To_Peer()
	case mtproto.PEER_CHANNEL:
		return mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: id}).To_Peer()
	default:
		return nil
	}
}

func boostUser(id, self int64) *mtproto.User {
	return mtproto.MakeTLUser(&mtproto.User{Id: id, Self: id == self}).To_User()
}

func boostChat(peerType int32, id int64) *mtproto.Chat {
	switch peerType {
	case mtproto.PEER_CHANNEL:
		return mtproto.MakeTLChannel(&mtproto.Chat{Id: id, Title: fmt.Sprintf("Channel %d", id), AccessHash_FLAGINT64: mtproto.MakeFlagsInt64(id)}).To_Chat()
	case mtproto.PEER_CHAT:
		return mtproto.MakeTLChat(&mtproto.Chat{Id: id, Title: fmt.Sprintf("Chat %d", id)}).To_Chat()
	default:
		return nil
	}
}

func boostWire(row domain.BoostSlot) *mtproto.Boost {
	id := fmt.Sprintf("%d:%d", row.UserID, row.Slot)
	out := &mtproto.Boost{Gift: row.Gift, Giveaway: row.Giveaway, Unclaimed: row.Unclaimed,
		Id: id, UserId: wrapperspb.Int64(row.UserID), Date: row.Date, Expires: row.Expires,
		Multiplier: wrapperspb.Int32(row.Multiplier), Stars: wrapperspb.Int64(row.Stars)}
	if row.UsedGiftSlug != "" {
		out.UsedGiftSlug = wrapperspb.String(row.UsedGiftSlug)
	}
	return mtproto.MakeTLBoost(out).To_Boost()
}

func myBoostWire(row domain.BoostSlot) *mtproto.MyBoost {
	out := &mtproto.MyBoost{Slot: row.Slot, Peer: boostPeer(row.PeerType, row.PeerID), Date: row.Date, Expires: row.Expires}
	if row.CooldownUntil > 0 {
		out.CooldownUntilDate = wrapperspb.Int32(row.CooldownUntil)
	}
	return mtproto.MakeTLMyBoost(out).To_MyBoost()
}

func pageBoostOffset(raw string) (int32, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return 0, mtproto.ErrOffsetInvalid
	}
	return int32(n), nil
}

func boostLimit(raw int32) (int32, error) {
	if raw < 0 || raw > 100 {
		return 0, mtproto.ErrLimitInvalid
	}
	if raw == 0 {
		return 100, nil
	}
	return raw, nil
}

func boostUsers(rows []domain.BoostSlot, self int64) []*mtproto.User {
	seen := make(map[int64]struct{}, len(rows))
	out := make([]*mtproto.User, 0, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.UserID]; ok {
			continue
		}
		seen[row.UserID] = struct{}{}
		out = append(out, boostUser(row.UserID, self))
	}
	return out
}

func boostChats(rows []domain.BoostSlot) []*mtproto.Chat {
	seen := make(map[string]struct{}, len(rows))
	out := make([]*mtproto.Chat, 0)
	for _, row := range rows {
		chat := boostChat(row.PeerType, row.PeerID)
		if chat == nil {
			continue
		}
		key := fmt.Sprintf("%d:%d", row.PeerType, row.PeerID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, chat)
	}
	return out
}

func premiumBoostStatus(target domain.BoostTarget, mine bool) *mtproto.Premium_BoostsStatus {
	level := target.Boosts / 10
	current := target.Boosts % 10
	return mtproto.MakeTLPremiumBoostsStatus(&mtproto.Premium_BoostsStatus{MyBoost: mine,
		Level: level, CurrentLevelBoosts: current, Boosts: target.Boosts,
		NextLevelBoosts: wrapperspb.Int32((level + 1) * 10),
		BoostUrl:        fmt.Sprintf("https://t.me/boost/%d", target.PeerID)}).To_Premium_BoostsStatus()
}

func storiesBoostStatus(target domain.BoostTarget, mine bool) *mtproto.Stories_BoostsStatus {
	level := target.Boosts / 10
	current := target.Boosts % 10
	return mtproto.MakeTLStoriesBoostsStatus(&mtproto.Stories_BoostsStatus{MyBoost: mine,
		Level: level, CurrentLevelBoosts: current, Boosts: target.Boosts,
		NextLevelBoosts: wrapperspb.Int32((level + 1) * 10),
		BoostUrl:        fmt.Sprintf("https://t.me/boost/%d", target.PeerID)}).To_Stories_BoostsStatus()
}

func (c *ApiFullCore) ChannelsSetBoostsToUnblockRestrictions(in *mtproto.TLChannelsSetBoostsToUnblockRestrictions) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostInputChannel(uid, in.GetChannel())
	if err != nil {
		return nil, boostDomainError(err)
	}
	if peer.PeerType != mtproto.PEER_CHANNEL {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetBoosts() < 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = domain.SetBoostRestrictions(uid, peer.PeerType, peer.PeerID, in.GetBoosts()); err != nil {
		return nil, boostDomainError(err)
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PremiumGetBoostsList(in *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopePremium, in.GetPeer())
	if err != nil {
		return nil, err
	}
	offset, err := pageBoostOffset(in.GetOffset())
	if err != nil {
		return nil, err
	}
	limit, err := boostLimit(in.GetLimit())
	if err != nil {
		return nil, err
	}
	rows, count, err := domain.ListBoosts(peer.Scope, peer.PeerType, peer.PeerID, nil, in.GetGifts(), offset, limit)
	if err != nil {
		return nil, boostDomainError(err)
	}
	result := &mtproto.Premium_BoostsList{Count: count, Boosts: make([]*mtproto.Boost, 0, len(rows)), Users: boostUsers(rows, uid)}
	for _, row := range rows {
		result.Boosts = append(result.Boosts, boostWire(row))
	}
	if offset+int32(len(rows)) < count {
		result.NextOffset = wrapperspb.String(strconv.Itoa(int(offset + int32(len(rows)))))
	}
	return mtproto.MakeTLPremiumBoostsList(result).To_Premium_BoostsList(), nil
}

func (c *ApiFullCore) PremiumGetMyBoosts(in *mtproto.TLPremiumGetMyBoosts) (*mtproto.Premium_MyBoosts, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	rows, err := domain.ListUserBoostSlots(uid)
	if err != nil {
		return nil, boostDomainError(err)
	}
	result := &mtproto.Premium_MyBoosts{MyBoosts: make([]*mtproto.MyBoost, 0, len(rows)), Chats: boostChats(rows), Users: boostUsers(rows, uid)}
	for _, row := range rows {
		result.MyBoosts = append(result.MyBoosts, myBoostWire(row))
	}
	return mtproto.MakeTLPremiumMyBoosts(result).To_Premium_MyBoosts(), nil
}

func (c *ApiFullCore) PremiumApplyBoost(in *mtproto.TLPremiumApplyBoost) (*mtproto.Premium_MyBoosts, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopePremium, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if len(in.GetSlots()) == 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if _, err = domain.ApplyBoost(uid, peer.Scope, peer.PeerType, peer.PeerID, in.GetSlots()); err != nil {
		return nil, boostDomainError(err)
	}
	return c.PremiumGetMyBoosts(nil)
}

func (c *ApiFullCore) PremiumGetBoostsStatus(in *mtproto.TLPremiumGetBoostsStatus) (*mtproto.Premium_BoostsStatus, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopePremium, in.GetPeer())
	if err != nil {
		return nil, err
	}
	target, err := domain.BoostStatus(peer.Scope, peer.PeerType, peer.PeerID)
	if err != nil {
		return nil, boostDomainError(err)
	}
	rows, _, err := domain.ListBoosts(peer.Scope, peer.PeerType, peer.PeerID, &uid, false, 0, 100)
	if err != nil {
		return nil, boostDomainError(err)
	}
	status := premiumBoostStatus(target, len(rows) > 0)
	if len(rows) > 0 {
		status.MyBoostSlots = make([]int32, 0, len(rows))
		for _, row := range rows {
			status.MyBoostSlots = append(status.MyBoostSlots, row.Slot)
		}
	}
	return status, nil
}

func (c *ApiFullCore) PremiumGetUserBoosts(in *mtproto.TLPremiumGetUserBoosts) (*mtproto.Premium_BoostsList, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopePremium, in.GetPeer())
	if err != nil {
		return nil, err
	}
	userID, err := boostInputUserID(uid, in.GetUserId())
	if err != nil {
		return nil, err
	}
	rows, count, err := domain.ListBoosts(peer.Scope, peer.PeerType, peer.PeerID, &userID, false, 0, 100)
	if err != nil {
		return nil, boostDomainError(err)
	}
	result := &mtproto.Premium_BoostsList{Count: count, Boosts: make([]*mtproto.Boost, 0, len(rows)), Users: boostUsers(rows, uid)}
	for _, row := range rows {
		result.Boosts = append(result.Boosts, boostWire(row))
	}
	return mtproto.MakeTLPremiumBoostsList(result).To_Premium_BoostsList(), nil
}

func (c *ApiFullCore) StoriesGetBoostsStatus(in *mtproto.TLStoriesGetBoostsStatus) (*mtproto.Stories_BoostsStatus, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopeStories, in.GetPeer())
	if err != nil {
		return nil, err
	}
	target, err := domain.BoostStatus(peer.Scope, peer.PeerType, peer.PeerID)
	if err != nil {
		return nil, boostDomainError(err)
	}
	rows, _, err := domain.ListBoosts(peer.Scope, peer.PeerType, peer.PeerID, &uid, false, 0, 1)
	if err != nil {
		return nil, boostDomainError(err)
	}
	return storiesBoostStatus(target, len(rows) > 0), nil
}

func (c *ApiFullCore) StoriesGetBoostersList(in *mtproto.TLStoriesGetBoostersList) (*mtproto.Stories_BoostersList, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopeStories, in.GetPeer())
	if err != nil {
		return nil, err
	}
	offset, err := pageBoostOffset(in.GetOffset())
	if err != nil {
		return nil, err
	}
	limit, err := boostLimit(in.GetLimit())
	if err != nil {
		return nil, err
	}
	rows, count, err := domain.ListBoosts(peer.Scope, peer.PeerType, peer.PeerID, nil, false, offset, limit)
	if err != nil {
		return nil, boostDomainError(err)
	}
	result := &mtproto.Stories_BoostersList{Count: count, Boosters: make([]*mtproto.Booster, 0, len(rows)), Users: boostUsers(rows, uid)}
	for _, row := range rows {
		result.Boosters = append(result.Boosters, mtproto.MakeTLBooster(&mtproto.Booster{UserId: row.UserID, Expires: row.Expires}).To_Booster())
	}
	if offset+int32(len(rows)) < count {
		result.NextOffset = wrapperspb.String(strconv.Itoa(int(offset + int32(len(rows)))))
	}
	return mtproto.MakeTLStoriesBoostersList(result).To_Stories_BoostersList(), nil
}

func (c *ApiFullCore) StoriesCanApplyBoost(in *mtproto.TLStoriesCanApplyBoost) (*mtproto.Stories_CanApplyBoostResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopeStories, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if _, err = domain.FirstFreeBoostSlot(uid); err == nil {
		return mtproto.MakeTLStoriesCanApplyBoostOk(&mtproto.Stories_CanApplyBoostResult{}).To_Stories_CanApplyBoostResult(), nil
	}
	rows, err := domain.ListUserBoostSlots(uid)
	if err != nil {
		return nil, boostDomainError(err)
	}
	for _, row := range rows {
		if row.Scope == peer.Scope && row.PeerType == peer.PeerType && row.PeerID == peer.PeerID {
			return mtproto.MakeTLStoriesCanApplyBoostOk(&mtproto.Stories_CanApplyBoostResult{}).To_Stories_CanApplyBoostResult(), nil
		}
	}
	if len(rows) == 0 {
		return nil, mtproto.ErrPremiumAccountRequired
	}
	replace := &mtproto.Stories_CanApplyBoostResult{CurrentBoost: boostPeer(rows[0].PeerType, rows[0].PeerID), Chats: boostChats(rows)}
	return mtproto.MakeTLStoriesCanApplyBoostReplace(replace).To_Stories_CanApplyBoostResult(), nil
}

func (c *ApiFullCore) StoriesApplyBoost(in *mtproto.TLStoriesApplyBoost) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, c.boostsUnavailable()
	}
	peer, err := boostPeerRefFromInput(uid, domain.BoostScopeStories, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if _, err = domain.ApplyFirstFreeBoost(uid, peer.Scope, peer.PeerType, peer.PeerID); err != nil {
		return nil, boostDomainError(err)
	}
	return mtproto.BoolTrue, nil
}
