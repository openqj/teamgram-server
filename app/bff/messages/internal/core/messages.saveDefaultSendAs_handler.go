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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// MessagesSaveDefaultSendAs
// messages.saveDefaultSendAs#ccfddf96 peer:InputPeer send_as:InputPeer = Bool;
func (c *MessagesCore) MessagesSaveDefaultSendAs(in *mtproto.TLMessagesSaveDefaultSendAs) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetPeer() == nil || in.GetSendAs() == nil {
		c.Logger.Errorf("messages.saveDefaultSendAs - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	sendAs := mtproto.FromInputPeer2(c.MD.UserId, in.SendAs)
	if !peer.IsUserOrChatOrChannel() || peer.PeerId == 0 || !sendAs.IsUserOrChatOrChannel() || sendAs.PeerId == 0 {
		c.Logger.Errorf("messages.saveDefaultSendAs - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}

	key := fmt.Sprintf("default_send_as:%d:%d:%d", c.MD.UserId, peer.PeerType, peer.PeerId)
	if err := persist.Default.Set(key, fmt.Sprintf("%d:%d", sendAs.PeerType, sendAs.PeerId)); err != nil {
		c.Logger.Errorf("messages.saveDefaultSendAs - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
