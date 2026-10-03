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
	"fmt"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCSuggestedPostsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func b16PostKey(userID int64) string {
	return fmt.Sprintf("b16:%d:post", userID)
}

func suggestedNote(in *mtproto.TLMessagesToggleSuggestedPostApproval) string {
	if in == nil {
		return ""
	}
	if cmt := in.GetRejectComment(); cmt != nil && cmt.GetValue() != "" {
		return cmt.GetValue()
	}
	if in.GetMsgId() != 0 {
		return strconv.FormatInt(int64(in.GetMsgId()), 10)
	}
	return ""
}

func (c *ApiFullCore) MessagesToggleSuggestedPostApproval(in *mtproto.TLMessagesToggleSuggestedPostApproval) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = restPut(uid, "messages.toggleSuggestedPostApproval", in); err != nil {
		return nil, err
	}
	note := suggestedNote(in)
	if note != "" {
		if err = persist.Default.Set(b16PostKey(uid), note); err != nil {
			return nil, err
		}
	} else {
		note, err = persist.Default.Get(b16PostKey(uid))
		if err != nil {
			return nil, err
		}
	}
	up := &mtproto.Updates{
		Message: note,
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}
	if in != nil && in.GetMsgId() != 0 {
		up.Id = in.GetMsgId()
	}
	return mtproto.MakeTLUpdates(up).To_Updates(), nil
}
