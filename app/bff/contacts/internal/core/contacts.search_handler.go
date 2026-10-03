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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsSearch
// contacts.search#11f812d8 q:string limit:int = contacts.Found;
func (c *ContactsCore) ContactsSearch(in *mtproto.TLContactsSearch) (*mtproto.Contacts_Found, error) {
	limit := in.GetLimit()
	if limit > 50 || limit == 0 {
		limit = 50
	}

	q := in.GetQ()
	if q == "" {
		return nil, mtproto.ErrSearchQueryEmpty
	}
	if q[0] == '@' {
		q = q[1:]
	}
	if len(q) < 3 {
		return nil, mtproto.ErrQueryTooShort
	}

	internalError := func(format string, args ...interface{}) error {
		return fmt.Errorf("contacts.search: %s: %w", fmt.Sprintf(format, args...), mtproto.ErrInternalServerError)
	}

	found := mtproto.MakeTLContactsFound(&mtproto.Contacts_Found{
		MyResults: []*mtproto.Peer{},
		Results:   []*mtproto.Peer{},
		Users:     []*mtproto.User{},
		Chats:     []*mtproto.Chat{},
	}).To_Contacts_Found()
	if limit <= 0 {
		return found, nil
	}

	contacts, err := c.svcCtx.Dao.UserClient.UserGetContactIdList(c.ctx, &userpb.TLUserGetContactIdList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("contacts.search - error: %v", err)
		return nil, err
	}
	if contacts == nil {
		return nil, internalError("user.getContactIdList returned no response")
	}
	excludedContacts := append(append([]int64{}, contacts.GetDatas()...), c.MD.UserId)
	myContactIDs := make(map[int64]struct{}, len(contacts.GetDatas()))
	for _, id := range contacts.GetDatas() {
		myContactIDs[id] = struct{}{}
	}

	usernameResults, err := c.svcCtx.Dao.UserClient.UserSearchUsername(c.ctx, &userpb.TLUserSearchUsername{
		Q:                q,
		ExcludedContacts: excludedContacts,
		Limit:            limit,
	})
	if err != nil {
		c.Logger.Errorf("contacts.search - error: %v", err)
		return nil, err
	}
	if usernameResults == nil {
		return nil, internalError("user.searchUsername returned no response")
	}

	idHelper := mtproto.NewIDListHelper(c.MD.UserId)
	for _, item := range usernameResults.GetDatas() {
		if item == nil || item.GetPeer() == nil {
			return nil, internalError("user.searchUsername returned a result without a peer")
		}
		peer := item.GetPeer()
		switch peer.GetPredicateName() {
		case mtproto.Predicate_peerUser:
			if peer.GetUserId() == 0 {
				return nil, internalError("user.searchUsername returned a user peer without an id")
			}
		case mtproto.Predicate_peerChannel:
			if peer.GetChannelId() == 0 {
				return nil, internalError("user.searchUsername returned a channel peer without an id")
			}
		case mtproto.Predicate_peerChat:
			if peer.GetChatId() == 0 {
				return nil, internalError("user.searchUsername returned a chat peer without an id")
			}
		default:
			return nil, internalError("user.searchUsername returned an unsupported peer type %q", peer.GetPredicateName())
		}
		idHelper.PickByPeer(peer)
	}

	userResults, err := c.svcCtx.Dao.UserClient.UserSearch(c.ctx, &userpb.TLUserSearch{
		Q:                q,
		ExcludedContacts: excludedContacts,
		Offset:           0,
		Limit:            limit,
	})
	if err != nil {
		c.Logger.Errorf("contacts.search - error: %v", err)
		return nil, err
	}
	if userResults == nil {
		return nil, internalError("user.search returned no response")
	}
	if userResults.GetPredicateName() != userpb.Predicate_usersIdFound {
		return nil, internalError("user.search returned %q, want %q", userResults.GetPredicateName(), userpb.Predicate_usersIdFound)
	}
	// user.search currently uses ExcludedContacts to select an ID-only response,
	// but its implementation's SQL excludes only the zero ID. Classify matching
	// contacts from the authoritative contact ID list returned above.
	for _, id := range userResults.GetIdList() {
		if id == 0 {
			return nil, internalError("user.search returned a user id of zero")
		}
		idHelper.PickByPeerUtil(mtproto.PEER_USER, id)
	}

	// The chat service only exposes query search here. It does not return a
	// proof that each result belongs to the current user, so accepting those
	// entities would allow an unscoped group to leak through contacts.search.
	// Keep this path fail-closed until a user-scoped chat lookup is available.
	if len(idHelper.ChatIdList) > 0 {
		return nil, internalError("chat search has no user-scoped search contract")
	}

	userIds := make([]int64, 0, len(idHelper.UserIdList))
	for _, id := range idHelper.UserIdList {
		if id != c.MD.UserId {
			userIds = append(userIds, id)
		}
	}
	if len(userIds) > 0 {
		lookupIds := append([]int64{c.MD.UserId}, userIds...)
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: lookupIds})
		if err != nil {
			c.Logger.Errorf("contacts.search - error: %v", err)
			return nil, err
		}
		if users == nil {
			return nil, internalError("user.getMutableUsers returned no response")
		}
		for _, user := range users.GetDatas() {
			if user == nil || user.GetUser() == nil {
				return nil, internalError("user.getMutableUsers returned an invalid user entity")
			}
		}

		me, ok := users.GetImmutableUser(c.MD.UserId)
		if !ok || me == nil || me.GetUser() == nil {
			return nil, internalError("user.getMutableUsers did not return the current user required for safe entity construction")
		}

		resolvedIds := make([]int64, 0, len(userIds))
		for _, id := range userIds {
			user, ok := users.GetImmutableUser(id)
			if !ok || user == nil || user.GetUser() == nil {
				return nil, internalError("user.getMutableUsers did not return search result user %d", id)
			}
			if user.Deleted() {
				continue
			}
			resolvedIds = append(resolvedIds, id)
			peer := mtproto.MakePeerUser(id)
			if _, isContact := myContactIDs[id]; isContact {
				found.MyResults = append(found.MyResults, peer)
			} else {
				found.Results = append(found.Results, peer)
			}
		}
		found.Users = users.GetUserListByIdList(c.MD.UserId, resolvedIds...)
	}

	if len(idHelper.ChannelIdList) > 0 {
		if c.svcCtx.Plugin == nil {
			return nil, internalError("channel search results require a configured ContactsPlugin.GetChannelListByIdList resolver; the contacts server currently wires Plugin=nil")
		}

		channels := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, idHelper.ChannelIdList...)
		requestedChannels := make(map[int64]struct{}, len(idHelper.ChannelIdList))
		for _, id := range idHelper.ChannelIdList {
			requestedChannels[id] = struct{}{}
		}
		byID := make(map[int64]*mtproto.Chat, len(channels))
		for _, channel := range channels {
			if channel == nil || (channel.GetPredicateName() != mtproto.Predicate_channel && channel.GetPredicateName() != mtproto.Predicate_channelForbidden) || channel.GetId() == 0 {
				return nil, internalError("channel resolver returned an invalid channel entity")
			}
			if _, requested := requestedChannels[channel.GetId()]; !requested {
				return nil, internalError("channel resolver returned unrequested channel %d", channel.GetId())
			}
			if _, duplicate := byID[channel.GetId()]; duplicate {
				return nil, internalError("channel resolver returned duplicate channel %d", channel.GetId())
			}
			byID[channel.GetId()] = channel
		}
		for _, id := range idHelper.ChannelIdList {
			channel := byID[id]
			if channel == nil {
				return nil, internalError("channel resolver did not return search result channel %d", id)
			}
			found.Chats = append(found.Chats, channel)
			found.Results = append(found.Results, mtproto.MakePeerChannel(id))
		}
	}

	return found, nil
}
