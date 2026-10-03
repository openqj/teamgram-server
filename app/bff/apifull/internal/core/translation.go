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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCTranslationServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func translateRequestString(in *mtproto.TLMessagesTranslateText) string {
	if in == nil {
		return ""
	}
	if s := in.GetText_FLAGSTRING(); s != nil && s.GetValue() != "" {
		return s.GetValue()
	}
	for _, part := range in.GetText_FLAGVECTORTEXTWITHENTITIES() {
		if part != nil && part.GetText() != "" {
			return part.GetText()
		}
	}
	if lang := in.GetToLang(); lang != "" {
		return lang
	}
	if s := in.GetFromLang(); s != nil && s.GetValue() != "" {
		return s.GetValue()
	}
	if s := in.GetTone(); s != nil && s.GetValue() != "" {
		return s.GetValue()
	}
	return ""
}

func (c *ApiFullCore) MessagesTranslateText(in *mtproto.TLMessagesTranslateText) (*mtproto.Messages_TranslatedText, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if s := translateRequestString(in); s != "" {
		if err = persist.Default.Set(b13Key(uid), s); err != nil {
			return nil, err
		}
	}
	stored, err := loadB13(uid)
	if err != nil {
		return nil, err
	}
	result := []*mtproto.TextWithEntities{}
	text := ""
	if stored != "" {
		result = []*mtproto.TextWithEntities{{Text: stored}}
		text = stored
	}
	return mtproto.MakeTLMessagesTranslateResult(&mtproto.Messages_TranslatedText{
		Result: result,
		Text:   text,
	}).To_Messages_TranslatedText(), nil
}

func (c *ApiFullCore) MessagesTogglePeerTranslations(in *mtproto.TLMessagesTogglePeerTranslations) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = smsPut(uid, "togglePeerTranslations", in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) ChannelsToggleAutotranslation(in *mtproto.TLChannelsToggleAutotranslation) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = smsPut(uid, "toggleAutotranslation", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}
