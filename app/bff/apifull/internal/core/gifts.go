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
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCGiftsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) giftsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PaymentsGetStarGifts(in *mtproto.TLPaymentsGetStarGifts) (*mtproto.Payments_StarGifts, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	records, err := domain.GiftCatalog()
	if err != nil {
		return nil, giftDomainError(err)
	}
	return mtproto.MakeTLPaymentsStarGifts(&mtproto.Payments_StarGifts{
		Hash:  0,
		Gifts: makeStarGifts(records),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Payments_StarGifts(), nil
}

func (c *ApiFullCore) PaymentsSaveStarGift(in *mtproto.TLPaymentsSaveStarGift) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetMsgId() != 0 || in.GetUserId() != nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	slug, err := localGiftSlug(in.GetStargift())
	if err != nil {
		return nil, err
	}
	if err = domain.SetGiftSaved(uid, nil, slug, !in.GetUnsave()); err != nil {
		return nil, giftDomainError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PaymentsConvertStarGift(in *mtproto.TLPaymentsConvertStarGift) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetMsgId() != 0 || in.GetUserId() != nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	slug, err := localGiftSlug(in.GetStargift())
	if err != nil {
		return nil, err
	}
	if err = domain.ConvertGift(uid, nil, slug); err != nil {
		return nil, giftDomainError(err)
	}
	return mtproto.BoolTrue, nil
}

// localGiftSlug is the only saved-gift reference the local ledger can resolve.
// Message and chat references need the message/chat stores to establish
// ownership; accepting their decoded fields here would mutate an unrelated
// row selected only by a client supplied slug.
func localGiftSlug(stargift *mtproto.InputSavedStarGift) (string, error) {
	if stargift == nil {
		return "", mtproto.ErrInputConstructorInvalid
	}
	predicate := stargift.GetPredicateName()
	if predicate != "" && predicate != mtproto.Predicate_inputSavedStarGiftSlug {
		return "", mtproto.ErrMethodNotImpl
	}
	if stargift.GetMsgId() != 0 || stargift.GetPeer() != nil || stargift.GetSavedId() != 0 {
		return "", mtproto.ErrMethodNotImpl
	}
	slug := stargift.GetSlug()
	if slug == "" || slug != strings.TrimSpace(slug) {
		return "", mtproto.ErrInputConstructorInvalid
	}
	return slug, nil
}

func (c *ApiFullCore) PaymentsGetStarGiftUpgradePreview(in *mtproto.TLPaymentsGetStarGiftUpgradePreview) (*mtproto.Payments_StarGiftUpgradePreview, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsUpgradeStarGift(in *mtproto.TLPaymentsUpgradeStarGift) (*mtproto.Updates, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsTransferStarGift(in *mtproto.TLPaymentsTransferStarGift) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetStargift() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	stargift := in.GetStargift()
	if predicate := stargift.GetPredicateName(); predicate != "" &&
		predicate != mtproto.Predicate_inputSavedStarGiftSlug &&
		predicate != mtproto.Predicate_inputSavedStarGiftUser {
		return nil, mtproto.ErrMethodNotImpl
	}
	// The local ledger has no message or chat gift resolver. Reject those
	// references before falling back to a client-supplied slug or saved ID;
	// otherwise a message reference could mutate an unrelated local row.
	if stargift.GetMsgId() != 0 || stargift.GetPredicateName() == mtproto.Predicate_inputSavedStarGiftUser {
		return nil, mtproto.ErrMethodNotImpl
	}
	if stargift.GetSavedId() < 0 || (stargift.GetSavedId() == 0 && stargift.GetSlug() == "") {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if stargift.GetSlug() != strings.TrimSpace(stargift.GetSlug()) {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if peer := stargift.GetPeer(); peer != nil {
		switch peer.GetPredicateName() {
		case mtproto.Predicate_inputPeerSelf:
		case mtproto.Predicate_inputPeerUser:
			if peer.GetUserId() != uid {
				return nil, mtproto.ErrPeerIdInvalid
			}
		default:
			return nil, mtproto.ErrPeerIdInvalid
		}
	}
	target, err := giftTransferTarget(uid, in)
	if err != nil {
		return nil, err
	}
	if err = domain.TransferGift(uid, target, stargift.GetSavedId(), stargift.GetSlug()); err != nil {
		return nil, giftDomainError(err)
	}
	// The durable inventory mutation is the source of truth. The generic
	// updates envelope acknowledges it; no unbacked gift/message update is
	// synthesized here.
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PaymentsGetUniqueStarGift(in *mtproto.TLPaymentsGetUniqueStarGift) (*mtproto.Payments_UniqueStarGift, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetSlug()) == "" {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	record, ok, err := domain.FindGiftBySlug(in.GetSlug())
	if err != nil {
		return nil, giftDomainError(err)
	}
	if !ok {
		return nil, mtproto.ErrMethodNotImpl
	}
	return mtproto.MakeTLPaymentsUniqueStarGift(&mtproto.Payments_UniqueStarGift{
		Gift:  makeWireStarGift(record),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Payments_UniqueStarGift(), nil
}

func (c *ApiFullCore) PaymentsGetSavedStarGifts(in *mtproto.TLPaymentsGetSavedStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in != nil {
		if in.GetLimit() < 0 {
			return nil, mtproto.ErrLimitInvalid
		}
		if peer := in.GetPeer(); peer != nil {
			switch peer.GetPredicateName() {
			case mtproto.Predicate_inputPeerSelf:
			case mtproto.Predicate_inputPeerUser:
				if peer.GetUserId() != uid {
					return nil, mtproto.ErrPeerIdInvalid
				}
			default:
				return nil, mtproto.ErrPeerIdInvalid
			}
		}
		if _, err = parseStarsOffset(in.GetOffset()); err != nil {
			return nil, err
		}
		if in.GetExcludeUnlimited() || in.GetExcludeUnique() || in.GetSortByValue() ||
			in.GetExcludeUpgradable() || in.GetExcludeUnupgradable() || in.GetPeerColorAvailable() ||
			in.GetExcludeHosted() || in.GetCollectionId() != nil || in.GetExcludeLimited() {
			return nil, mtproto.ErrMethodNotImpl
		}
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	records, err := domain.ListGifts(uid, false)
	if err != nil {
		return nil, giftDomainError(err)
	}
	filtered := records[:0]
	for _, record := range records {
		if (record.Saved && in.GetExcludeSaved()) || (!record.Saved && in.GetExcludeUnsaved()) {
			continue
		}
		filtered = append(filtered, record)
	}
	start, limit, err := giftPage(in.GetOffset(), in.GetLimit())
	if err != nil {
		return nil, err
	}
	page, next := sliceGifts(filtered, start, limit)
	var nextOffset *wrapperspb.StringValue
	if next >= 0 {
		nextOffset = wrapperspb.String(strconv.Itoa(next))
	}
	return mtproto.MakeTLPaymentsSavedStarGifts(&mtproto.Payments_SavedStarGifts{
		Count:      len32(filtered),
		Gifts:      makeSavedStarGifts(page),
		NextOffset: nextOffset,
		Chats:      []*mtproto.Chat{},
		Users:      []*mtproto.User{},
	}).To_Payments_SavedStarGifts(), nil
}

func makeStarGifts(records []domain.Gift) []*mtproto.StarGift {
	gifts := make([]*mtproto.StarGift, 0, len(records))
	for _, record := range records {
		gifts = append(gifts, makeWireStarGift(record))
	}
	return gifts
}

func makeStarGift(record domain.Gift) *mtproto.StarGift {
	gift := &mtproto.StarGift{
		Id:    record.ID,
		Slug:  record.Slug,
		Stars: record.Stars,
	}
	if record.Stars > 0 {
		gift.ConvertStars = record.Stars
	}
	return mtproto.MakeTLStarGift(&mtproto.StarGift{
		Id:           gift.Id,
		Slug:         gift.Slug,
		Stars:        gift.Stars,
		ConvertStars: gift.ConvertStars,
	}).To_StarGift()
}

// makeWireStarGift supplies the required Document union member at the wire
// boundary. The local gift ledger does not own sticker media, so documentEmpty
// is the honest representation; it prevents the generated Layer 229 encoder
// from dereferencing a nil required field.
func makeWireStarGift(record domain.Gift) *mtproto.StarGift {
	gift := makeStarGift(record)
	gift.Sticker = mtproto.MakeTLDocumentEmpty(&mtproto.Document{}).To_Document()
	return gift
}

func makeSavedStarGifts(records []domain.Gift) []*mtproto.SavedStarGift {
	gifts := make([]*mtproto.SavedStarGift, 0, len(records))
	for _, record := range records {
		gifts = append(gifts, mtproto.MakeTLSavedStarGift(&mtproto.SavedStarGift{
			Gift:    makeWireStarGift(record),
			SavedId: wrapperspb.Int64(record.ID),
		}).To_SavedStarGift())
	}
	return gifts
}

func (c *ApiFullCore) PaymentsGetSavedStarGift(in *mtproto.TLPaymentsGetSavedStarGift) (*mtproto.Payments_SavedStarGifts, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || len(in.GetStargift()) == 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	for _, input := range in.GetStargift() {
		if input == nil {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		if predicate := input.GetPredicateName(); predicate != "" &&
			predicate != mtproto.Predicate_inputSavedStarGiftSlug &&
			predicate != mtproto.Predicate_inputSavedStarGiftUser &&
			predicate != mtproto.Predicate_inputSavedStarGiftChat {
			return nil, mtproto.ErrMethodNotImpl
		}
		if input.GetPredicateName() == mtproto.Predicate_inputSavedStarGiftUser ||
			input.GetPredicateName() == mtproto.Predicate_inputSavedStarGiftChat || input.GetMsgId() != 0 {
			return nil, mtproto.ErrMethodNotImpl
		}
		if input.GetSavedId() < 0 {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		if input.GetSavedId() == 0 && input.GetSlug() == "" && input.GetMsgId() == 0 {
			return nil, mtproto.ErrInputConstructorInvalid
		}
	}
	records, err := domain.ListGifts(uid, false)
	if err != nil {
		return nil, giftDomainError(err)
	}
	matched := make([]domain.Gift, 0, len(in.GetStargift()))
	for _, input := range in.GetStargift() {
		for _, record := range records {
			if !record.Saved || (input.GetSavedId() != 0 && input.GetSavedId() != record.ID) ||
				(input.GetSavedId() == 0 && input.GetSlug() != record.Slug) {
				continue
			}
			matched = append(matched, record)
		}
	}
	return mtproto.MakeTLPaymentsSavedStarGifts(&mtproto.Payments_SavedStarGifts{
		Count: len32(matched),
		Gifts: makeSavedStarGifts(matched),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Payments_SavedStarGifts(), nil
}

func (c *ApiFullCore) PaymentsGetStarGiftWithdrawalUrl(in *mtproto.TLPaymentsGetStarGiftWithdrawalUrl) (*mtproto.Payments_StarGiftWithdrawalUrl, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsToggleChatStarGiftNotifications(in *mtproto.TLPaymentsToggleChatStarGiftNotifications) (*mtproto.Bool, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsToggleStarGiftsPinnedToTop(in *mtproto.TLPaymentsToggleStarGiftsPinnedToTop) (*mtproto.Bool, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetResaleStarGifts(in *mtproto.TLPaymentsGetResaleStarGifts) (*mtproto.Payments_ResaleStarGifts, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsUpdateStarGiftPrice(in *mtproto.TLPaymentsUpdateStarGiftPrice) (*mtproto.Updates, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetUniqueStarGiftValueInfo(in *mtproto.TLPaymentsGetUniqueStarGiftValueInfo) (*mtproto.Payments_UniqueStarGiftValueInfo, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsCheckCanSendGift(in *mtproto.TLPaymentsCheckCanSendGift) (*mtproto.Payments_CheckCanSendGiftResult, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetStarGiftAuctionState(in *mtproto.TLPaymentsGetStarGiftAuctionState) (*mtproto.Payments_StarGiftAuctionState, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetStarGiftAuctionAcquiredGifts(in *mtproto.TLPaymentsGetStarGiftAuctionAcquiredGifts) (*mtproto.Payments_StarGiftAuctionAcquiredGifts, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetStarGiftActiveAuctions(in *mtproto.TLPaymentsGetStarGiftActiveAuctions) (*mtproto.Payments_StarGiftActiveAuctions, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsResolveStarGiftOffer(in *mtproto.TLPaymentsResolveStarGiftOffer) (*mtproto.Updates, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsSendStarGiftOffer(in *mtproto.TLPaymentsSendStarGiftOffer) (*mtproto.Updates, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetStarGiftUpgradeAttributes(in *mtproto.TLPaymentsGetStarGiftUpgradeAttributes) (*mtproto.Payments_StarGiftUpgradeAttributes, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetCraftStarGifts(in *mtproto.TLPaymentsGetCraftStarGifts) (*mtproto.Payments_SavedStarGifts, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsCraftStarGift(in *mtproto.TLPaymentsCraftStarGift) (*mtproto.Updates, error) {
	_ = in
	return nil, c.giftsUnavailable()
}

func (c *ApiFullCore) PaymentsGetUserStarGifts(in *mtproto.TLPaymentsGetUserStarGifts) (*mtproto.Payments_UserStarGifts, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if _, err = localGiftUserID(uid, in.GetUserId()); err != nil {
		return nil, err
	}
	if _, err = parseStarsOffset(in.GetOffset()); err != nil {
		return nil, err
	}
	if in.GetLimit() < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	records, err := domain.ListGifts(uid, false)
	if err != nil {
		return nil, giftDomainError(err)
	}
	start, limit, err := giftPage(in.GetOffset(), in.GetLimit())
	if err != nil {
		return nil, err
	}
	page, next := sliceGifts(records, start, limit)
	var nextOffset *wrapperspb.StringValue
	if next >= 0 {
		nextOffset = wrapperspb.String(strconv.Itoa(next))
	}
	return mtproto.MakeTLPaymentsUserStarGifts(&mtproto.Payments_UserStarGifts{
		Count:      len32(records),
		Gifts:      makeUserStarGifts(page),
		NextOffset: nextOffset,
		Users:      []*mtproto.User{},
	}).To_Payments_UserStarGifts(), nil
}

func (c *ApiFullCore) PaymentsGetUserStarGift(in *mtproto.TLPaymentsGetUserStarGift) (*mtproto.Payments_UserStarGifts, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || len(in.GetMsgId()) == 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	// The local gift table has no authoritative message-id mapping.
	return nil, mtproto.ErrMethodNotImpl
}

func localGiftUserID(selfID int64, user *mtproto.InputUser) (int64, error) {
	peer := mtproto.FromInputUser(selfID, user)
	switch peer.PeerType {
	case mtproto.PEER_SELF:
		return selfID, nil
	case mtproto.PEER_USER:
		if peer.PeerId == selfID {
			return selfID, nil
		}
		// The APIFull gift ledger has no public-gift privacy provider. Do not
		// expose another user's inventory as if it were authoritative.
		return 0, mtproto.ErrUserIdInvalid
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}

func giftTransferTarget(selfID int64, in *mtproto.TLPaymentsTransferStarGift) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrInputConstructorInvalid
	}
	if in.GetToId_INPUTPEER() != nil && in.GetToId_INPUTUSER() != nil {
		return 0, mtproto.ErrInputConstructorInvalid
	}
	if peer := in.GetToId_INPUTPEER(); peer != nil {
		switch peer.GetPredicateName() {
		case mtproto.Predicate_inputPeerSelf:
			return 0, mtproto.ErrPeerIdInvalid
		case mtproto.Predicate_inputPeerUser:
			if peer.GetUserId() <= 0 || peer.GetUserId() == selfID {
				return 0, mtproto.ErrPeerIdInvalid
			}
			return peer.GetUserId(), nil
		default:
			return 0, mtproto.ErrPeerIdInvalid
		}
	}
	if user := in.GetToId_INPUTUSER(); user != nil {
		peer := mtproto.FromInputUser(selfID, user)
		if peer == nil || peer.PeerType != mtproto.PEER_USER || peer.PeerId <= 0 || peer.PeerId == selfID {
			return 0, mtproto.ErrPeerIdInvalid
		}
		return peer.PeerId, nil
	}
	return 0, mtproto.ErrPeerIdInvalid
}

func makeUserStarGifts(records []domain.Gift) []*mtproto.UserStarGift {
	gifts := make([]*mtproto.UserStarGift, 0, len(records))
	for _, record := range records {
		gift := &mtproto.UserStarGift{
			Unsaved: record.Saved == false,
			Gift:    makeWireStarGift(record),
		}
		if record.From > 0 {
			gift.FromId = wrapperspb.Int64(record.From)
		}
		if record.Stars > 0 {
			gift.ConvertStars = wrapperspb.Int64(record.Stars)
		}
		gifts = append(gifts, mtproto.MakeTLUserStarGift(gift).To_UserStarGift())
	}
	return gifts
}

func giftDomainError(err error) error {
	if err == nil {
		return nil
	}
	if !domain.Ready() || errors.Is(err, domain.ErrGiftNotFound) || errors.Is(err, domain.ErrGiftNotConvertible) {
		return mtproto.ErrMethodNotImpl
	}
	return err
}

func giftPage(offset string, limit int32) (int, int, error) {
	start, err := parseStarsOffset(offset)
	if err != nil {
		return 0, 0, err
	}
	if limit < 0 {
		return 0, 0, mtproto.ErrLimitInvalid
	}
	if limit == 0 {
		limit = 100
	}
	return start, int(limit), nil
}

func sliceGifts(records []domain.Gift, start, limit int) ([]domain.Gift, int) {
	if start >= len(records) {
		return []domain.Gift{}, -1
	}
	end := start + limit
	if end > len(records) {
		end = len(records)
	}
	next := -1
	if end < len(records) {
		next = end
	}
	return records[start:end], next
}

func len32[T any](items []T) int32 {
	if len(items) > int(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(len(items))
}
