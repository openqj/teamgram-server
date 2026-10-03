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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCMessageEffectsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type effectJSON struct {
	Id                int64  `json:"id"`
	Emoticon          string `json:"emoticon,omitempty"`
	Premium           bool   `json:"premium,omitempty"`
	StaticIconId      int64  `json:"staticIconId,omitempty"`
	EffectStickerId   int64  `json:"effectStickerId,omitempty"`
	EffectAnimationId int64  `json:"effectAnimationId,omitempty"`
}

const effectsCatalogKey = "effects:catalog"

func loadEffectCatalog() ([]effectJSON, int32, error) {
	raw, err := persist.Default.Get(effectsCatalogKey)
	if err != nil || raw == "" {
		return nil, 0, err
	}
	var list []effectJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, 0, nil
	}
	var h int32
	for _, e := range list {
		h = h*31 + int32(e.Id)
		for _, r := range e.Emoticon {
			h = h*31 + int32(r)
		}
	}
	return list, h, nil
}

func (c *ApiFullCore) MessagesGetAvailableEffects(in *mtproto.TLMessagesGetAvailableEffects) (*mtproto.Messages_AvailableEffects, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	list, hash, err := loadEffectCatalog()
	if err != nil {
		return nil, err
	}
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLMessagesAvailableEffectsNotModified(&mtproto.Messages_AvailableEffects{}).To_Messages_AvailableEffects(), nil
	}
	effects := make([]*mtproto.AvailableEffect, 0, len(list))
	for _, e := range list {
		row := &mtproto.AvailableEffect{
			Id:              e.Id,
			Emoticon:        e.Emoticon,
			PremiumRequired: e.Premium,
			EffectStickerId: e.EffectStickerId,
		}
		if e.StaticIconId != 0 {
			row.StaticIconId = wrapperspb.Int64(e.StaticIconId)
		}
		if e.EffectAnimationId != 0 {
			row.EffectAnimationId = wrapperspb.Int64(e.EffectAnimationId)
		}
		effects = append(effects, mtproto.MakeTLAvailableEffect(row).To_AvailableEffect())
	}
	return mtproto.MakeTLMessagesAvailableEffects(&mtproto.Messages_AvailableEffects{
		Hash:      hash,
		Effects:   effects,
		Documents: []*mtproto.Document{},
	}).To_Messages_AvailableEffects(), nil
}
