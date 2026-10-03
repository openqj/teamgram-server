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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

const (
	// The schema has no published maximum; bound response allocation locally.
	searchResultsPositionsMaxLimit int32 = 2000
	searchResultsPositionsPageSize int32 = 1000
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
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetSavedPeerId() != nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	mediaType, err := searchResultsPositionMediaType(in.GetFilter())
	if err != nil {
		return nil, err
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

	offsetID := in.GetOffsetId()
	if offsetID == 0 {
		offsetID = math.MaxInt32
	}
	boxList, err := c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
		UserId:    c.MD.UserId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		MediaType: mediaType,
		Offset:    offsetID,
		Limit:     in.GetLimit(),
	})
	if err != nil {
		return nil, err
	}
	if boxList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	startPositionOffset := positionOffset
	positions := make([]*mtproto.SearchResultsPosition, 0, in.GetLimit())
	previousID := offsetID
	for _, box := range boxList.GetBoxList() {
		if int32(len(positions)) == in.GetLimit() {
			break
		}
		messageID, date, err := searchResultPositionData(box)
		if err != nil || messageID >= previousID || positionOffset >= count.GetV() {
			return nil, mtproto.ErrInternalServerError
		}
		positions = append(positions, mtproto.MakeTLSearchResultPosition(&mtproto.SearchResultsPosition{
			MsgId:  messageID,
			Date:   date,
			Offset: positionOffset,
		}).To_SearchResultsPosition())
		positionOffset++
		previousID = messageID
	}
	expectedPositions := count.GetV() - startPositionOffset
	if expectedPositions > in.GetLimit() {
		expectedPositions = in.GetLimit()
	}
	if int32(len(positions)) != expectedPositions {
		return nil, mtproto.ErrInternalServerError
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
