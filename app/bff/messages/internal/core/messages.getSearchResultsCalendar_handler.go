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
	"github.com/teamgram/proto/mtproto"
)

// MessagesGetSearchResultsCalendar
// messages.getSearchResultsCalendar#6aa3f6bd flags:# peer:InputPeer saved_peer_id:flags.2?InputPeer filter:MessagesFilter offset_id:int offset_date:int = messages.SearchResultsCalendar;
func (c *MessagesCore) MessagesGetSearchResultsCalendar(in *mtproto.TLMessagesGetSearchResultsCalendar) (*mtproto.Messages_SearchResultsCalendar, error) {
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetFilter() == nil {
		return nil, mtproto.ErrInputFilterInvalid
	}

	const limit int32 = 50
	found, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:        in.Peer,
		SavedPeerId: in.SavedPeerId,
		Filter:      in.Filter,
		OffsetId:    in.OffsetId,
		MaxDate:     in.OffsetDate,
		Limit:       limit,
	})
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsCalendar - error: %v", err)
		return nil, err
	}
	if found == nil {
		c.Logger.Errorf("messages.getSearchResultsCalendar - search returned nil result")
		return nil, mtproto.ErrInternalServerError
	}

	return bucketByDay(found, int32(len(found.GetMessages())) >= limit), nil
}
