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

// ContactsGetSponsoredPeers
// contacts.getSponsoredPeers#b6c8c393 q:string = contacts.SponsoredPeers;
func (c *SponsoredMessagesCore) ContactsGetSponsoredPeers(in *mtproto.TLContactsGetSponsoredPeers) (*mtproto.Contacts_SponsoredPeers, error) {
	if _, err := c.requireSponsoredUser(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}

	// Sponsored peers are optional inventory. Until an ad provider is
	// configured, report the protocol's empty result instead of failing every
	// contact search with METHOD_NOT_IMPL.
	return mtproto.MakeTLContactsSponsoredPeersEmpty(&mtproto.Contacts_SponsoredPeers{
		Peers: []*mtproto.SponsoredPeer{},
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Contacts_SponsoredPeers(), nil
}
