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

// MessagesSearchSentMedia
// messages.searchSentMedia#107e31a0 q:string filter:MessagesFilter limit:int = messages.Messages;
func (c *MessagesCore) MessagesSearchSentMedia(in *mtproto.TLMessagesSearchSentMedia) (*mtproto.Messages_Messages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetLimit() < 0 || in.GetLimit() > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	if in.GetFilter() == nil {
		return nil, mtproto.ErrInputFilterInvalid
	}
	filterType := mtproto.FromMessagesFilter(in.GetFilter())
	if filterType == mtproto.FilterEmpty && in.GetFilter().GetPredicateName() != mtproto.Predicate_inputMessagesFilterEmpty {
		return nil, mtproto.ErrInputFilterInvalid
	}

	if c.MD.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}

	mediaType, err := sentMediaFilterType(in.GetFilter())
	if err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	limit := in.GetLimit()
	if limit == 0 {
		limit = 50
	}
	boxList, err := c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
		UserId:    c.MD.UserId,
		PeerType:  mtproto.PEER_UNKNOWN,
		PeerId:    0,
		MediaType: mediaType,
		Offset:    int32(^uint32(0) >> 1),
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	if boxList == nil {
		return nil, mtproto.ErrInternalServerError
	}
	result := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()
	if err := c.populateSearchResult(boxList, result); err != nil {
		return nil, err
	}
	return result, nil
}

func sentMediaFilterType(filter *mtproto.MessagesFilter) (int32, error) {
	if filter == nil {
		return 0, mtproto.ErrInputFilterInvalid
	}
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterPhotos:
		return mtproto.MEDIA_PHOTOS_ONLY, nil
	case mtproto.FilterVideo:
		return mtproto.MEDIA_VIDEOS_ONLY, nil
	case mtproto.FilterPhotoVideo:
		return mtproto.MEDIA_PHOTOVIDEO, nil
	case mtproto.FilterDocument:
		return mtproto.MEDIA_FILE, nil
	case mtproto.FilterUrl:
		return mtproto.MEDIA_URL, nil
	case mtproto.FilterGif:
		return mtproto.MEDIA_GIF, nil
	case mtproto.FilterVoice:
		return mtproto.MEDIA_VOICE_FILE, nil
	case mtproto.FilterMusic:
		return mtproto.MEDIA_MUSIC, nil
	case mtproto.FilterChatPhotos:
		return mtproto.MEDIA_CHAT_PHOTO, nil
	case mtproto.FilterPhoneCalls:
		return mtproto.MEDIA_PHONE_CALL, nil
	case mtproto.FilterRoundVoice:
		return mtproto.MEDIA_AUDIO, nil
	case mtproto.FilterRoundVideo:
		return mtproto.MEDIA_ROUND_FILE, nil
	default:
		return 0, mtproto.ErrInputFilterInvalid
	}
}
