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
// messages.getSearchCounters#732eef00 peer:InputPeer filters:Vector<MessagesFilter> = Vector<messages.SearchCounter>;
func (c *MessagesCore) MessagesGetSearchCounters(in *mtproto.TLMessagesGetSearchCounters) (*mtproto.Vector_Messages_SearchCounter, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	)
	if peer == nil || peer.PeerType == mtproto.PEER_EMPTY || peer.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	counters := &mtproto.Vector_Messages_SearchCounter{
		Datas: make([]*mtproto.Messages_SearchCounter, 0, len(in.GetFilters())),
	}

	if in.SavedPeerId != nil {
		savedCounters, err := c.savedCounters(in.SavedPeerId, in.GetFilters())
		if err != nil {
			c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
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
		var (
			fType = mtproto.FromMessagesFilter(filter)
			mType int32
		)

		switch fType {
		case mtproto.FilterPhotos:
			mType = mtproto.MEDIA_PHOTOS_ONLY
		case mtproto.FilterVideo:
			mType = mtproto.MEDIA_VIDEOS_ONLY
		case mtproto.FilterPhotoVideo:
			mType = mtproto.MEDIA_PHOTOVIDEO
		case mtproto.FilterDocument:
			mType = mtproto.MEDIA_FILE
		case mtproto.FilterUrl:
			mType = mtproto.MEDIA_URL
		case mtproto.FilterGif:
			mType = mtproto.MEDIA_GIF
		case mtproto.FilterMusic:
			mType = mtproto.MEDIA_MUSIC
		case mtproto.FilterRoundVoice:
			mType = mtproto.MEDIA_AUDIO
		case mtproto.FilterVoice:
			mType = mtproto.MEDIA_VOICE_FILE
		case mtproto.FilterRoundVideo:
			mType = mtproto.MEDIA_ROUND_FILE
		case mtproto.FilterPhoneCalls:
			mType = mtproto.MEDIA_PHONE_CALL
		case mtproto.FilterChatPhotos:
			mType = mtproto.MEDIA_CHAT_PHOTO
		default:
			return nil, mtproto.ErrMethodNotImpl
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
			c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
			return nil, err
		}
		if sz == nil {
			c.Logger.Errorf("messages.getSearchCounters - counter RPC returned nil result")
			return nil, mtproto.ErrInternalServerError
		}

		counter := mtproto.MakeTLMessagesSearchCounter(&mtproto.Messages_SearchCounter{
			Inexact: false,
			Filter:  filter,
			Count:   sz.GetV(),
		}).To_Messages_SearchCounter()
		counters.Datas = append(counters.Datas, counter)
		c.Logger.Infof("messages.getSearchCounters - result: %s", counter)
	}

	return counters, nil
}
