// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)

package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// RPCPaidMediaServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesGetExtendedMedia(in *mtproto.TLMessagesGetExtendedMedia) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	peerInput := in.GetPeer()
	if peerInput == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := validateExtendedMediaPeer(uid, peerInput); err != nil {
		return nil, err
	}
	if peerInput.GetPredicateName() == mtproto.Predicate_inputPeerChannel {
		return nil, mtproto.ErrChannelInvalid
	}
	peer := mtproto.FromInputPeer2(uid, peerInput)
	if peer == nil || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		return nil, mtproto.ErrChannelInvalid
	}
	if !peer.IsChatOrUser() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	empty := func() *mtproto.Updates {
		return mtproto.MakeTLUpdates(&mtproto.Updates{
			Updates: []*mtproto.Update{},
			Users:   []*mtproto.User{},
			Chats:   []*mtproto.Chat{},
			Date:    int32(time.Now().Unix()),
		}).To_Updates()
	}
	if len(in.GetId()) == 0 {
		return empty(), nil
	}
	for _, id := range in.GetId() {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}
	d := c.apifullDao()
	if d == nil || d.MessageListReader == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	boxes, err := d.MessageGetUserMessageList(c.ctx, &messagepb.TLMessageGetUserMessageList{
		UserId: uid,
		IdList: in.GetId(),
	})
	if err != nil {
		return nil, err
	}
	if boxes == nil {
		return empty(), nil
	}
	updates := make([]*mtproto.Update, 0, len(boxes.GetDatas()))
	matched := make([]*mtproto.Message, 0, len(boxes.GetDatas()))
	for _, box := range boxes.GetDatas() {
		if box == nil || box.GetMessage() == nil || !messageBoxInPeer(peer, box, uid) {
			continue
		}
		media := box.GetMessage().GetMedia()
		if media == nil {
			continue
		}
		ext := media.GetExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA()
		if len(ext) == 0 {
			if one := media.GetExtendedMedia_FLAGMESSAGEEXTENDEDMEDIA(); one != nil {
				ext = []*mtproto.MessageExtendedMedia{one}
			}
		}
		if len(ext) == 0 {
			continue
		}
		matched = append(matched, box.ToMessage(uid))
		updates = append(updates, mtproto.MakeTLUpdateMessageExtendedMedia(&mtproto.Update{
			Peer_PEER:                                peer.ToPeer(),
			MsgId_INT32:                              box.GetMessageId(),
			ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: ext,
		}).To_Update())
	}
	if len(updates) == 0 {
		return empty(), nil
	}
	users, chats, err := c.hydrateThreadMessages(uid, matched)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: updates,
		Users:   users,
		Chats:   chats,
		Date:    int32(time.Now().Unix()),
	}).To_Updates(), nil
}

func validateExtendedMediaPeer(selfID int64, input *mtproto.InputPeer) error {
	if selfID <= 0 || input == nil {
		return mtproto.ErrPeerIdInvalid
	}
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return nil
	case mtproto.Predicate_inputPeerUser:
		if input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
			return mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerChat:
		if input.GetChatId() <= 0 {
			return mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerChannel:
		if input.GetChannelId() <= 0 || input.GetAccessHash() == 0 {
			return mtproto.ErrPeerIdInvalid
		}
	case mtproto.Predicate_inputPeerUserFromMessage:
		if input.GetUserId() <= 0 || input.GetMsgId() <= 0 || input.GetPeer() == nil {
			return mtproto.ErrPeerIdInvalid
		}
	default:
		if input.GetPredicateName() != "" {
			return mtproto.ErrPeerIdInvalid
		}
		if input.GetUserId() > 0 && input.GetAccessHash() == 0 {
			return mtproto.ErrPeerIdInvalid
		}
		if input.GetUserId() <= 0 && input.GetChatId() <= 0 {
			return mtproto.ErrPeerIdInvalid
		}
	}
	return nil
}

func messageBoxInPeer(peer *mtproto.PeerUtil, box *mtproto.MessageBox, selfID int64) bool {
	if peer == nil || box == nil {
		return false
	}
	wantType, wantID := peer.PeerType, peer.PeerId
	if wantType == mtproto.PEER_SELF {
		wantType, wantID = mtproto.PEER_USER, selfID
	}
	gotType, gotID := box.GetPeerType(), box.GetPeerId()
	if gotType == mtproto.PEER_SELF {
		gotType = mtproto.PEER_USER
		if gotID == 0 {
			gotID = selfID
		}
	}
	return gotType == wantType && gotID == wantID
}
