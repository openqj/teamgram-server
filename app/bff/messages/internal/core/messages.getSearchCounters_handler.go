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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesGetSearchCounters
// messages.getSearchCounters#1bbcf300 flags:# peer:InputPeer saved_peer_id:flags.2?InputPeer top_msg_id:flags.0?int filters:Vector<MessagesFilter> = Vector<messages.SearchCounter>;
func (c *MessagesCore) MessagesGetSearchCounters(in *mtproto.TLMessagesGetSearchCounters) (*mtproto.Vector_Messages_SearchCounter, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	peer, err := c.validateSearchCountersPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	if in.GetTopMsgId() != nil {
		if in.GetTopMsgId().GetValue() <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		// message.getSearchCounter has no topic/root-message scope. Returning a
		// global count here would make a valid reply look exact when it is not.
		return nil, mtproto.ErrMethodNotImpl
	}
	counters := &mtproto.Vector_Messages_SearchCounter{
		Datas: make([]*mtproto.Messages_SearchCounter, 0, len(in.GetFilters())),
	}

	if in.SavedPeerId != nil {
		if _, err = c.validateSearchCountersPeer(in.GetSavedPeerId()); err != nil {
			return nil, err
		}
		savedCounters, err := c.savedCounters(in.GetSavedPeerId(), in.GetFilters())
		if err != nil {
			if c.Logger != nil {
				c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
			}
			return nil, err
		}
		return savedCounters, nil
	}

	for _, filter := range in.GetFilters() {
		if filter == nil {
			return nil, mtproto.ErrInputFilterInvalid
		}
		/*
			{
			    "constructor": "CRC32_messages_getSearchCounters",
			    "peer": {
			        "predicate_name": "inputPeerChat",
			        "constructor": "CRC32_inputPeerChat",
			        "chat_id": 10053
			    },
			    "filters": [
			        {
			            "predicate_name": "inputMessagesFilterPhotoVideo",
			            "constructor": "CRC32_inputMessagesFilterPhotoVideo"
			        },
			        {
			            "predicate_name": "inputMessagesFilterDocument",
			            "constructor": "CRC32_inputMessagesFilterDocument"
			        },
			        {
			            "predicate_name": "inputMessagesFilterRoundVoice",
			            "constructor": "CRC32_inputMessagesFilterRoundVoice"
			        },
			        {
			            "predicate_name": "inputMessagesFilterUrl",
			            "constructor": "CRC32_inputMessagesFilterUrl"
			        },
			        {
			            "predicate_name": "inputMessagesFilterMusic",
			            "constructor": "CRC32_inputMessagesFilterMusic"
			        },
			        {
			            "predicate_name": "inputMessagesFilterGif",
			            "constructor": "CRC32_inputMessagesFilterGif"
			        }
			    ]
			}
		*/

		/*
		 */
		mType, err := searchCounterMediaType(filter)
		if err != nil {
			return nil, err
		}

		sz, err := c.svcCtx.Dao.MessageClient.MessageGetSearchCounter(
			c.ctx,
			&message.TLMessageGetSearchCounter{
				UserId:    c.MD.UserId,
				PeerType:  peer.PeerType,
				PeerId:    peer.PeerId,
				MediaType: mType,
			})
		if err != nil {
			if c.Logger != nil {
				c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
			}
			return nil, err
		}
		if sz == nil || sz.GetV() < 0 {
			if c.Logger != nil {
				c.Logger.Errorf("messages.getSearchCounters - counter RPC returned nil or negative result")
			}
			return nil, mtproto.ErrInternalServerError
		}

		counter := mtproto.MakeTLMessagesSearchCounter(&mtproto.Messages_SearchCounter{
			Inexact: false,
			Filter:  filter,
			Count:   sz.GetV(),
		}).To_Messages_SearchCounter()
		counters.Datas = append(counters.Datas, counter)
		if c.Logger != nil {
			c.Logger.Infof("messages.getSearchCounters - result: %s", counter)
		}
	}

	return counters, nil
}

func searchCounterMediaType(filter *mtproto.MessagesFilter) (int32, error) {
	if filter == nil {
		return 0, mtproto.ErrInputFilterInvalid
	}
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
	case mtproto.Predicate_inputMessagesFilterMusic:
		return mtproto.MEDIA_MUSIC, nil
	case mtproto.Predicate_inputMessagesFilterRoundVoice:
		return mtproto.MEDIA_AUDIO, nil
	case mtproto.Predicate_inputMessagesFilterVoice:
		return mtproto.MEDIA_VOICE_FILE, nil
	case mtproto.Predicate_inputMessagesFilterRoundVideo:
		return mtproto.MEDIA_ROUND_FILE, nil
	case mtproto.Predicate_inputMessagesFilterPhoneCalls:
		return mtproto.MEDIA_PHONE_CALL, nil
	case mtproto.Predicate_inputMessagesFilterChatPhotos:
		return mtproto.MEDIA_CHAT_PHOTO, nil
	case mtproto.Predicate_inputMessagesFilterPinned:
		return mtproto.MEDIA_PINNED, nil
	case mtproto.Predicate_inputMessagesFilterEmpty,
		mtproto.Predicate_inputMessagesFilterPoll,
		mtproto.Predicate_inputMessagesFilterMyMentions,
		mtproto.Predicate_inputMessagesFilterGeo,
		mtproto.Predicate_inputMessagesFilterContacts:
		return 0, mtproto.ErrMethodNotImpl
	default:
		return 0, mtproto.ErrInputFilterInvalid
	}
}

func (c *MessagesCore) validateSearchCountersPeer(input *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if input == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, input)
	if peer == nil || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF:
		return peer, nil
	case mtproto.PEER_USER:
		if err := c.validateMessagesGetHistoryUserPeer(input); err != nil {
			return nil, err
		}
		return peer, nil
	case mtproto.PEER_CHAT:
		if err := c.validateMessagesGetHistoryChatPeer(peer); err != nil {
			return nil, err
		}
		return peer, nil
	case mtproto.PEER_CHANNEL:
		if input.GetAccessHash() == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
		// The canonical message counter is backed by the per-user messages
		// relation. Channel history/media lives in channelview and has no exact
		// counter bridge, so do not expose a potentially misleading zero count.
		return nil, mtproto.ErrMethodNotImpl
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}
