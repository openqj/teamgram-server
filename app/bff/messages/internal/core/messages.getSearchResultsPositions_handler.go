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
	"math"

	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	// The schema has no published maximum; bound response allocation locally.
	searchResultsPositionsMaxLimit int32 = 2000
	// Keep position pages at 50 rows so the cursor remains bounded even though
	// the provider accepts the public searchSentMedia limit of up to 100.
	searchResultsPositionsPageSize int32 = 50
)

// MessagesGetSearchResultsPositions
// messages.getSearchResultsPositions#9c7f2f10 flags:# peer:InputPeer saved_peer_id:flags.2?InputPeer filter:MessagesFilter offset_id:int limit:int = messages.SearchResultsPositions;
func (c *MessagesCore) MessagesGetSearchResultsPositions(in *mtproto.TLMessagesGetSearchResultsPositions) (*mtproto.Messages_SearchResultsPositions, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetFilter() == nil {
		return nil, mtproto.ErrInputFilterInvalid
	}
	if c == nil || c.MD == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if c.MD.UserId <= 0 {
		return nil, mtproto.ErrActiveUserRequired
	}
	if c.MD.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if in.GetOffsetId() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if in.GetLimit() <= 0 || in.GetLimit() > searchResultsPositionsMaxLimit {
		return nil, mtproto.ErrLimitInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil || peer.PeerId <= 0 || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	// Channel search is served by channelview, which has no media-position or
	// exact-count API. The message service's local cache is not authoritative.
	if peer.IsChannel() {
		if in.GetPeer().GetAccessHash() == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetSavedPeerId() != nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	mediaType, err := searchResultsPositionMediaType(in.GetFilter())
	if err != nil {
		return nil, err
	}
	switch peer.PeerType {
	case mtproto.PEER_USER:
		if err = c.validateSearchResultsPositionsUserPeer(in.GetPeer()); err != nil {
			return nil, err
		}
	case mtproto.PEER_CHAT:
		if err = c.validateSearchResultsPositionsChatPeer(peer); err != nil {
			return nil, err
		}
	case mtproto.PEER_SELF:
		// InputPeerSelf is already bound to the authenticated user.
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	count, err := c.svcCtx.Dao.MessageClient.MessageGetSearchCounter(c.ctx, &message.TLMessageGetSearchCounter{
		UserId:    c.MD.UserId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		MediaType: mediaType,
	})
	if err != nil {
		return nil, err
	}
	if count == nil || count.GetV() < 0 {
		return nil, mtproto.ErrInternalServerError
	}
	if count.GetV() == 0 {
		return makeSearchResultsPositions(0, nil), nil
	}

	positionOffset, err := c.searchResultsPositionOffset(peer, mediaType, in.GetOffsetId(), count.GetV())
	if err != nil {
		return nil, err
	}

	positions, err := c.searchResultsPositionPage(peer, mediaType, in.GetOffsetId(), positionOffset, count.GetV(), in.GetLimit())
	if err != nil {
		return nil, err
	}

	return makeSearchResultsPositions(count.GetV(), positions), nil
}

func makeSearchResultsPositions(count int32, positions []*mtproto.SearchResultsPosition) *mtproto.Messages_SearchResultsPositions {
	if positions == nil {
		positions = []*mtproto.SearchResultsPosition{}
	}
	return mtproto.MakeTLMessagesSearchResultsPositions(&mtproto.Messages_SearchResultsPositions{
		Count:     count,
		Positions: positions,
	}).To_Messages_SearchResultsPositions()
}

func (c *MessagesCore) validateSearchResultsPositionsUserPeer(input *mtproto.InputPeer) error {
	if input == nil || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrInternalServerError
	}
	users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: []int64{input.GetUserId()},
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		return err
	}
	if users == nil {
		return mtproto.ErrInternalServerError
	}
	for _, user := range users.GetDatas() {
		if user == nil || user.GetUser() == nil {
			continue
		}
		data := user.GetUser()
		if data.GetId() == input.GetUserId() && !data.GetDeleted() && data.GetAccessHash() == input.GetAccessHash() {
			return nil
		}
	}
	return mtproto.ErrPeerIdInvalid
}

func (c *MessagesCore) validateSearchResultsPositionsChatPeer(peer *mtproto.PeerUtil) error {
	if peer == nil || peer.PeerId <= 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
		return mtproto.ErrInternalServerError
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: peer.PeerId,
	})
	if err != nil {
		return err
	}
	if chat == nil {
		return mtproto.ErrPeerIdInvalid
	}
	member, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || member == nil || !member.IsChatMemberStateNormal() {
		return mtproto.ErrUserNotParticipant
	}
	return nil
}

func (c *MessagesCore) searchResultsPositionPage(peer *mtproto.PeerUtil, mediaType, offsetID, positionOffset, count, limit int32) ([]*mtproto.SearchResultsPosition, error) {
	if positionOffset < 0 || positionOffset > count {
		return nil, mtproto.ErrInternalServerError
	}
	if positionOffset == count {
		return []*mtproto.SearchResultsPosition{}, nil
	}
	if offsetID == 0 {
		offsetID = math.MaxInt32
	}
	positions := make([]*mtproto.SearchResultsPosition, 0, limit)
	position := positionOffset
	cursor := offsetID
	for int32(len(positions)) < limit && position < count {
		requestLimit := limit - int32(len(positions))
		if requestLimit > searchResultsPositionsPageSize {
			requestLimit = searchResultsPositionsPageSize
		}
		boxList, err := c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mediaType,
			Offset:    cursor,
			Limit:     requestLimit,
		})
		if err != nil {
			return nil, err
		}
		if boxList == nil {
			return nil, mtproto.ErrInternalServerError
		}
		boxes := boxList.GetBoxList()
		if int32(len(boxes)) > requestLimit {
			return nil, mtproto.ErrInternalServerError
		}
		previousID := cursor
		for _, box := range boxes {
			messageID, date, err := searchResultPositionData(box)
			if err != nil || messageID >= previousID || position >= count {
				return nil, mtproto.ErrInternalServerError
			}
			positions = append(positions, mtproto.MakeTLSearchResultPosition(&mtproto.SearchResultsPosition{
				MsgId:  messageID,
				Date:   date,
				Offset: position,
			}).To_SearchResultsPosition())
			position++
			previousID = messageID
		}
		if len(boxes) == 0 {
			if position != count {
				return nil, mtproto.ErrInternalServerError
			}
			break
		}
		cursor = previousID
		if int32(len(boxes)) < requestLimit {
			if position != count && int32(len(positions)) < limit {
				return nil, mtproto.ErrInternalServerError
			}
			break
		}
	}
	expectedPositions := count - positionOffset
	if expectedPositions > limit {
		expectedPositions = limit
	}
	if int32(len(positions)) != expectedPositions {
		return nil, mtproto.ErrInternalServerError
	}
	return positions, nil
}

func (c *MessagesCore) searchResultsPositionOffset(peer *mtproto.PeerUtil, mediaType, offsetID, count int32) (int32, error) {
	if offsetID == 0 || count == 0 {
		return 0, nil
	}

	cursor := int32(math.MaxInt32)
	position := int32(0)
	rank := int32(-1)
	for {
		boxList, err := c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mediaType,
			Offset:    cursor,
			Limit:     searchResultsPositionsPageSize,
		})
		if err != nil {
			return 0, err
		}
		if boxList == nil {
			return 0, mtproto.ErrInternalServerError
		}

		boxes := boxList.GetBoxList()
		if int32(len(boxes)) > searchResultsPositionsPageSize {
			return 0, mtproto.ErrInternalServerError
		}
		if len(boxes) == 0 {
			if position == count {
				if rank >= 0 {
					return rank, nil
				}
				return position, nil
			}
			return 0, mtproto.ErrInternalServerError
		}

		nextCursor := cursor
		for _, box := range boxes {
			messageID, _, err := searchResultPositionData(box)
			if err != nil || messageID >= nextCursor {
				return 0, mtproto.ErrInternalServerError
			}
			if position >= count {
				return 0, mtproto.ErrInternalServerError
			}
			if rank < 0 && messageID < offsetID {
				rank = position
			}
			position++
			nextCursor = messageID
		}

		if int32(len(boxes)) < searchResultsPositionsPageSize {
			if position != count {
				return 0, mtproto.ErrInternalServerError
			}
			if rank >= 0 {
				return rank, nil
			}
			return position, nil
		}
		cursor = nextCursor
	}
}

func searchResultPositionData(box *mtproto.MessageBox) (int32, int32, error) {
	if box == nil || box.GetMessage() == nil {
		return 0, 0, mtproto.ErrInternalServerError
	}
	messageID := box.GetMessageId()
	if messageID <= 0 {
		return 0, 0, mtproto.ErrInternalServerError
	}
	return messageID, box.GetMessage().GetDate(), nil
}

func searchResultsPositionMediaType(filter *mtproto.MessagesFilter) (int32, error) {
	switch filter.GetPredicateName() {
	case mtproto.Predicate_inputMessagesFilterPhotos:
		return mtproto.MEDIA_PHOTOS_ONLY, nil
	case mtproto.Predicate_inputMessagesFilterVideo:
		return mtproto.MEDIA_VIDEOS_ONLY, nil
	case mtproto.Predicate_inputMessagesFilterPhotoVideo:
		return mtproto.MEDIA_PHOTOVIDEO, nil
	case mtproto.Predicate_inputMessagesFilterDocument:
		return mtproto.MEDIA_FILE, nil
	case mtproto.Predicate_inputMessagesFilterUrl:
		return mtproto.MEDIA_URL, nil
	case mtproto.Predicate_inputMessagesFilterGif:
		return mtproto.MEDIA_GIF, nil
	case mtproto.Predicate_inputMessagesFilterVoice:
		return mtproto.MEDIA_VOICE_FILE, nil
	case mtproto.Predicate_inputMessagesFilterMusic:
		return mtproto.MEDIA_MUSIC, nil
	case mtproto.Predicate_inputMessagesFilterChatPhotos:
		return mtproto.MEDIA_CHAT_PHOTO, nil
	case mtproto.Predicate_inputMessagesFilterRoundVoice:
		return mtproto.MEDIA_AUDIO, nil
	case mtproto.Predicate_inputMessagesFilterRoundVideo:
		return mtproto.MEDIA_ROUND_FILE, nil
	case mtproto.Predicate_inputMessagesFilterEmpty,
		mtproto.Predicate_inputMessagesFilterPhoneCalls,
		mtproto.Predicate_inputMessagesFilterMyMentions,
		mtproto.Predicate_inputMessagesFilterGeo,
		mtproto.Predicate_inputMessagesFilterContacts,
		mtproto.Predicate_inputMessagesFilterPinned,
		mtproto.Predicate_inputMessagesFilterPoll:
		return 0, mtproto.ErrFilterNotSupported
	default:
		return 0, mtproto.ErrInputFilterInvalid
	}
}
