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
	"encoding/json"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCPromoDataServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const promoExpires int32 = 1893456000

type promoEntry struct {
	userID int64
	psa    string
}

// builtinPromos is the stable catalog. Hidden peer ids are omitted from getPromoData.
var builtinPromos = []promoEntry{
	{userID: 777000, psa: "teamgram"},
	{userID: 777001, psa: "news"},
}

func promoHideKey(userID int64) string {
	return "promo:hide:" + strconv.FormatInt(userID, 10)
}

func loadHiddenPromos(userID int64) (map[int64]struct{}, error) {
	raw, err := persist.Default.Get(promoHideKey(userID))
	if err != nil || raw == "" {
		return map[int64]struct{}{}, err
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil, err
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

func promoPeerID(p *mtproto.InputPeer) (int64, bool) {
	if p == nil {
		return 0, false
	}
	if p.GetUserId() != 0 {
		return p.GetUserId(), true
	}
	if p.GetChatId() != 0 {
		return p.GetChatId(), true
	}
	if p.GetChannelId() != 0 {
		return p.GetChannelId(), true
	}
	return 0, false
}

func (c *ApiFullCore) HelpGetPromoData(in *mtproto.TLHelpGetPromoData) (*mtproto.Help_PromoData, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	_ = in
	hidden, err := loadHiddenPromos(userID)
	if err != nil {
		return nil, err
	}
	for _, item := range builtinPromos {
		if _, skip := hidden[item.userID]; skip {
			continue
		}
		return mtproto.MakeTLHelpPromoData(&mtproto.Help_PromoData{
			Expires:    promoExpires,
			Peer:       mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: item.userID}).To_Peer(),
			PsaType:    wrapperspb.String(item.psa),
			PsaMessage: wrapperspb.String(item.psa),
			Chats:      []*mtproto.Chat{},
			Users:      []*mtproto.User{},
		}).To_Help_PromoData(), nil
	}
	return mtproto.MakeTLHelpPromoDataEmpty(&mtproto.Help_PromoData{
		Expires: promoExpires,
	}).To_Help_PromoData(), nil
}

func (c *ApiFullCore) HelpHidePromoData(in *mtproto.TLHelpHidePromoData) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
	}
	id, ok := promoPeerID(peer)
	if !ok {
		return mtproto.BoolTrue, nil
	}
	hidden, err := loadHiddenPromos(userID)
	if err != nil {
		return nil, err
	}
	if _, exists := hidden[id]; !exists {
		ids := make([]int64, 0, len(hidden)+1)
		for k := range hidden {
			ids = append(ids, k)
		}
		ids = append(ids, id)
		b, err := json.Marshal(ids)
		if err != nil {
			return nil, err
		}
		if err = persist.Default.Set(promoHideKey(userID), string(b)); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}
