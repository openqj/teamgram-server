// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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
)

// MessagesTranslateRichMessage
// messages.translateRichMessage#1a542004 flags:# peer:flags.0?InputPeer id:flags.0?Vector<int> text:flags.1?Vector<InputRichMessage> to_lang:string tone:flags.2?string = messages.TranslatedRichMessage;
func (c *MessagesCore) MessagesTranslateRichMessage(in *mtproto.TLMessagesTranslateRichMessage) (*mtproto.Messages_TranslatedRichMessage, error) {
	if c == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetToLang() == "" || len(in.GetText()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	for _, value := range in.GetText() {
		if value == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	var result []*mtproto.RichMessage
	err := c.callAIProvider("translate_rich_message", struct {
		Peer   *mtproto.InputPeer          `json:"peer,omitempty"`
		IDs    []int32                     `json:"ids,omitempty"`
		Text   []*mtproto.InputRichMessage `json:"text"`
		ToLang string                      `json:"to_lang"`
		Tone   string                      `json:"tone,omitempty"`
	}{in.GetPeer(), in.GetId(), in.GetText(), in.GetToLang(), wrapperStringValue(in.GetTone())}, &result)
	if err != nil {
		return nil, err
	}
	if len(result) != len(in.GetText()) {
		return nil, mtproto.ErrInternalServerError
	}
	for i, value := range result {
		result[i], err = normalizeAIRich(value)
		if err != nil {
			return nil, err
		}
	}
	return mtproto.MakeTLMessagesTranslatedRichMessage(&mtproto.Messages_TranslatedRichMessage{Result: result}).To_Messages_TranslatedRichMessage(), nil
}
