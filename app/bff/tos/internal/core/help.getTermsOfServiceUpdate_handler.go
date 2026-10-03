// Copyright 2022 Teamgram Authors
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
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

const tosIDData = `{"country":""}`

func tosKey(userId int64) string {
	return "tos:" + strconv.FormatInt(userId, 10)
}

// HelpGetTermsOfServiceUpdate
// help.getTermsOfServiceUpdate#2ca51fd1 = help.TermsOfServiceUpdate;
func (c *TosCore) HelpGetTermsOfServiceUpdate(in *mtproto.TLHelpGetTermsOfServiceUpdate) (*mtproto.Help_TermsOfServiceUpdate, error) {
	_ = in

	accepted, err := persist.Default.Get(tosKey(c.MD.UserId))
	if err != nil {
		c.Logger.Errorf("help.getTermsOfServiceUpdate - error: %v", err)
		return nil, err
	}
	expires := int32(time.Now().Unix() + 3600)
	if accepted == tosIDData {
		return mtproto.MakeTLHelpTermsOfServiceUpdateEmpty(&mtproto.Help_TermsOfServiceUpdate{
			Expires: expires,
		}).To_Help_TermsOfServiceUpdate(), nil
	}

	return mtproto.MakeTLHelpTermsOfServiceUpdate(&mtproto.Help_TermsOfServiceUpdate{
		Expires: expires,
		TermsOfService: mtproto.MakeTLHelpTermsOfService(&mtproto.Help_TermsOfService{
			Popup: false,
			Id: mtproto.MakeTLDataJSON(&mtproto.DataJSON{
				Data: tosIDData,
			}).To_DataJSON(),
			Text:     "Terms of Service",
			Entities: []*mtproto.MessageEntity{},
		}).To_Help_TermsOfService(),
	}).To_Help_TermsOfServiceUpdate(), nil
}
