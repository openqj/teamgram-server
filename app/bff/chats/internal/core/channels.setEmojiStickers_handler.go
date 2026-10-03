// Copyright 2024 Teamgram Authors
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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// ChannelsSetEmojiStickers
// channels.setEmojiStickers#3cd930b7 channel:InputChannel stickerset:InputStickerSet = Bool;
func (c *ChatsCore) ChannelsSetEmojiStickers(in *mtproto.TLChannelsSetEmojiStickers) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	channelId, err := inputChannelId(in.GetChannel())
	if err != nil {
		return nil, err
	}
	chat, err := c.loadMutableChat(channelId)
	if err != nil {
		return nil, err
	}
	if _, err = c.requireCreatorOrAdmin(chat, false); err != nil {
		c.Logger.Errorf("channels.setEmojiStickers - error: %v", err)
		return nil, err
	}

	key := "chat-emoji:" + strconv.FormatInt(channelId, 10)
	if err = persist.Default.Set(key, stickerSetValue(in.GetStickerset())); err != nil {
		c.Logger.Errorf("channels.setEmojiStickers - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
