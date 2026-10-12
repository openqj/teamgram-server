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
)

// RPCSuggestedPostsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesToggleSuggestedPostApproval(in *mtproto.TLMessagesToggleSuggestedPostApproval) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch in.GetPeer().GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf,
		mtproto.Predicate_inputPeerUser,
		mtproto.Predicate_inputPeerChat,
		mtproto.Predicate_inputPeerChannel:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	var raw json.RawMessage
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.toggleSuggestedPostApproval", businessControlPeerKey(c.MD.GetUserId(), in.GetPeer())+":msg:"+strconv.FormatInt(int64(in.GetMsgId()), 10), &raw); loadErr != nil {
		return nil, loadErr
	} else if local {
		if len(raw) == 0 || string(raw) == "null" {
			return nil, mtproto.ErrInternalServerError
		}
		var updates mtproto.Updates
		if err := json.Unmarshal(raw, &updates); err != nil || updates.GetPredicateName() == "" {
			return nil, mtproto.ErrInternalServerError
		}
		return &updates, nil
	}
	if err := c.callBusinessProvider("messages.toggleSuggestedPostApproval", map[string]any{
		"peer": in.GetPeer(), "msg_id": in.GetMsgId(), "reject": in.GetReject(),
		"schedule_date": in.GetScheduleDate(), "reject_comment": in.GetRejectComment(),
	}, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, mtproto.ErrInternalServerError
	}
	var updates mtproto.Updates
	if err := json.Unmarshal(raw, &updates); err != nil || updates.GetPredicateName() == "" {
		return nil, mtproto.ErrInternalServerError
	}
	return &updates, nil
}
