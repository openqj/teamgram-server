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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

func (c *SponsoredMessagesCore) requireSponsoredUser() (int64, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return c.MD.UserId, nil
}

func sponsoredSet(userID int64, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return persist.Default.Set(fmt.Sprintf("sponsored:%d:%s", userID, op), string(raw))
}

func sponsoredRandom(id []byte) (string, error) {
	if len(id) == 0 {
		return "", mtproto.ErrRandomIdEmpty
	}
	return hex.EncodeToString(id), nil
}

func sponsoredPeer(p *mtproto.InputPeer) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d:%d", p.GetPredicateName(), p.GetUserId(), p.GetChatId(), p.GetChannelId())
}
