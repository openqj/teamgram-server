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
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
)

// InboxUpdatePinnedMessageV2
// inbox.updatePinnedMessageV2 flags:# user_id:long unpin:flags.1?true peer_type:int peer_id:long id:int dialog_message_id:long layer:flags.3?int server_id:flags.4?string session_id:flags.5?long client_req_msg_id:flags.6?long = Void;
func (c *InboxCore) InboxUpdatePinnedMessageV2(in *inbox.TLInboxUpdatePinnedMessageV2) (*mtproto.Void, error) {
	if in == nil || in.DialogMessageId == 0 || in.Id == 0 {
		c.Logger.Errorf("inbox.updatePinnedMessageV2 - invalid request")
		return nil, mtproto.ErrMessageIdInvalid
	}
	switch in.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		c.Logger.Errorf("inbox.updatePinnedMessageV2 - invalid peer type")
		return nil, mtproto.ErrPeerIdInvalid
	}
	return c.InboxUpdatePinnedMessage(&inbox.TLInboxUpdatePinnedMessage{
		UserId:          in.UserId,
		Unpin:           in.GetUnpin(),
		PeerType:        in.PeerType,
		PeerId:          in.PeerId,
		Id:              in.Id,
		DialogMessageId: in.DialogMessageId,
	})
}
