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
)

// ContactsResetTopPeerRating
// contacts.resetTopPeerRating#1ae373ac category:TopPeerCategory peer:InputPeer = Bool;
func (c *ContactsCore) ContactsResetTopPeerRating(in *mtproto.TLContactsResetTopPeerRating) (*mtproto.Bool, error) {
	if in.GetCategory() == nil || in.GetPeer() == nil {
		c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", mtproto.ErrInputRequestInvalid)
		return nil, mtproto.ErrInputRequestInvalid
	}
	pred := in.GetCategory().GetPredicateName()
	if !knownTopPeerCategory(pred) {
		c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", mtproto.ErrInputConstructorInvalid)
		return nil, mtproto.ErrInputConstructorInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	peerType := peer.PeerType
	peerId := peer.PeerId
	if peerType == mtproto.PEER_SELF {
		peerType = mtproto.PEER_USER
		if peerId == 0 {
			peerId = peer.SelfId
		}
	}
	switch peerType {
	case mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		if peerId == 0 {
			c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", mtproto.ErrPeerIdInvalid)
			return nil, mtproto.ErrPeerIdInvalid
		}
	default:
		c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}

	st, err := loadTopPeersState(c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", err)
		return nil, err
	}
	key := fmt.Sprintf("%d:%d", peerType, peerId)
	for _, existing := range st.Hidden[pred] {
		if existing == key {
			return mtproto.BoolTrue, nil
		}
	}
	st.Hidden[pred] = append(st.Hidden[pred], key)
	if err = saveTopPeersState(c.MD.UserId, st); err != nil {
		c.Logger.Errorf("contacts.resetTopPeerRating - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
