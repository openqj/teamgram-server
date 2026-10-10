// Copyright 2026 Teamgram Authors
//
//	All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func collectionOwner(self int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil {
		return 0, mtproto.ErrInputConstructorInvalid
	}
	peerType, peerID := apifullPeerTypeID(self, peer)
	if peerType == mtproto.PEER_SELF {
		return self, nil
	}
	if peerType != mtproto.PEER_USER || peerID != self {
		return 0, mtproto.ErrPeerIdInvalid
	}
	return self, nil
}

func collectionGiftRefs(owner int64, inputs []*mtproto.InputSavedStarGift) ([]int64, error) {
	ids := make([]int64, 0, len(inputs))
	seen := make(map[int64]struct{}, len(inputs))
	for _, input := range inputs {
		if input == nil {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		predicate := input.GetPredicateName()
		if predicate != "" && predicate != mtproto.Predicate_inputSavedStarGiftSlug {
			return nil, mtproto.ErrMethodNotImpl
		}
		if input.GetMsgId() != 0 || input.GetPeer() != nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		if input.GetSavedId() < 0 || (input.GetSavedId() == 0 && strings.TrimSpace(input.GetSlug()) == "") || input.GetSlug() != strings.TrimSpace(input.GetSlug()) {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		id, err := domain.ResolveOwnedGift(owner, domain.GiftRef{ID: input.GetSavedId(), Slug: input.GetSlug()})
		if err != nil {
			return nil, giftCollectionDomainError(err)
		}
		if _, ok := seen[id]; ok {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func giftCollectionDomainError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrGiftCollectionNotFound) || errors.Is(err, domain.ErrGiftCollectionInvalid) {
		return mtproto.ErrMethodNotImpl
	}
	return err
}

func wireGiftCollection(collection domain.GiftCollection) *mtproto.StarGiftCollection {
	return mtproto.MakeTLStarGiftCollection(&mtproto.StarGiftCollection{CollectionId: int32(collection.ID), Title: collection.Title, GiftsCount: collection.GiftsCnt, Hash: collection.Hash}).To_StarGiftCollection()
}

func (c *ApiFullCore) PaymentsCreateStarGiftCollection(in *mtproto.TLPaymentsCreateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	owner, err := collectionOwner(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GetTitle()) == "" || in.GetTitle() != strings.TrimSpace(in.GetTitle()) {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	ids, err := collectionGiftRefs(owner, in.GetStargift())
	if err != nil {
		return nil, err
	}
	collection, err := domain.CreateGiftCollection(owner, in.GetTitle(), ids)
	if err != nil {
		return nil, giftCollectionDomainError(err)
	}
	return wireGiftCollection(collection), nil
}

func (c *ApiFullCore) PaymentsUpdateStarGiftCollection(in *mtproto.TLPaymentsUpdateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	owner, err := collectionOwner(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	deletes, err := collectionGiftRefs(owner, in.GetDeleteStargift())
	if err != nil {
		return nil, err
	}
	adds, err := collectionGiftRefs(owner, in.GetAddStargift())
	if err != nil {
		return nil, err
	}
	order, err := collectionGiftRefs(owner, in.GetOrder())
	if err != nil {
		return nil, err
	}
	var title *string
	if in.GetTitle() != nil {
		value := in.GetTitle().GetValue()
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		title = &value
	}
	collection, err := domain.UpdateGiftCollection(owner, int64(in.GetCollectionId()), title, deletes, adds, order)
	if err != nil {
		return nil, giftCollectionDomainError(err)
	}
	return wireGiftCollection(collection), nil
}

func (c *ApiFullCore) PaymentsReorderStarGiftCollections(in *mtproto.TLPaymentsReorderStarGiftCollections) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	owner, err := collectionOwner(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if len(in.GetOrder()) == 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	order := make([]int64, len(in.GetOrder()))
	seen := make(map[int64]struct{}, len(order))
	for i, id := range in.GetOrder() {
		if id <= 0 {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		order[i] = int64(id)
		if _, ok := seen[order[i]]; ok {
			return nil, mtproto.ErrInputConstructorInvalid
		}
		seen[order[i]] = struct{}{}
	}
	if err = domain.ReorderGiftCollections(owner, order); err != nil {
		return nil, giftCollectionDomainError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PaymentsDeleteStarGiftCollection(in *mtproto.TLPaymentsDeleteStarGiftCollection) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetCollectionId() <= 0 {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	owner, err := collectionOwner(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = domain.DeleteGiftCollection(owner, int64(in.GetCollectionId())); err != nil {
		return nil, giftCollectionDomainError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PaymentsGetStarGiftCollections(in *mtproto.TLPaymentsGetStarGiftCollections) (*mtproto.Payments_StarGiftCollections, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	owner, err := collectionOwner(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	collections, hash, err := domain.ListGiftCollections(owner)
	if err != nil {
		return nil, giftCollectionDomainError(err)
	}
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLPaymentsStarGiftCollectionsNotModified(&mtproto.Payments_StarGiftCollections{}).To_Payments_StarGiftCollections(), nil
	}
	wire := make([]*mtproto.StarGiftCollection, 0, len(collections))
	for _, collection := range collections {
		wire = append(wire, wireGiftCollection(collection))
	}
	return mtproto.MakeTLPaymentsStarGiftCollections(&mtproto.Payments_StarGiftCollections{Collections: wire}).To_Payments_StarGiftCollections(), nil
}
