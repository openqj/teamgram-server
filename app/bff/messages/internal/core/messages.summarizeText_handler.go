// Copyright 2025 Teamgram Authors
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

// MessagesSummarizeText
// messages.summarizeText#9d4104e2 flags:# peer:InputPeer id:int to_lang:flags.0?string = TextWithEntities;
func (c *MessagesCore) MessagesSummarizeText(in *mtproto.TLMessagesSummarizeText) (*mtproto.TextWithEntities, error) {
	if c == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	var result mtproto.TextWithEntities
	err := c.callAIProvider("summarize_text", struct {
		Peer   *mtproto.InputPeer `json:"peer,omitempty"`
		ID     int32              `json:"id"`
		ToLang string             `json:"to_lang,omitempty"`
	}{in.GetPeer(), in.GetId(), wrapperStringValue(in.GetToLang())}, &result)
	if err != nil {
		return nil, err
	}
	return normalizeAIText(&result)
}
