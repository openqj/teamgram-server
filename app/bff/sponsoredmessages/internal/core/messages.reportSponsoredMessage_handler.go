// Copyright 2024 Teamgram Authors
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
	"encoding/hex"

	"github.com/teamgram/proto/mtproto"
)

// MessagesReportSponsoredMessage
// messages.reportSponsoredMessage#12cbf0c4 random_id:bytes option:bytes = channels.SponsoredMessageReportResult;
func (c *SponsoredMessagesCore) MessagesReportSponsoredMessage(in *mtproto.TLMessagesReportSponsoredMessage) (*mtproto.Channels_SponsoredMessageReportResult, error) {
	uid, err := c.requireSponsoredUser()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	rid, err := sponsoredRandom(in.GetRandomId())
	if err != nil {
		return nil, err
	}
	if err = sponsoredSet(c.ctx, uid, "message_report:"+rid, map[string]any{"random_id": rid, "option": hex.EncodeToString(in.GetOption())}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLChannelsSponsoredMessageReportResultReported(nil).To_Channels_SponsoredMessageReportResult(), nil
}
