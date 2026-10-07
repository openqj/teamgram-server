// Copyright 2026 Teamgram Authors
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
	"encoding/json"
	"fmt"
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// RPCFactChecksServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func factStoreKey(userID int64, peer *mtproto.InputPeer, msgID int32) string {
	var peerUser, peerChat, peerChannel int64
	if peer != nil {
		peerUser = peer.GetUserId()
		peerChat = peer.GetChatId()
		peerChannel = peer.GetChannelId()
	}
	return fmt.Sprintf("fact:%d:%d:%d:%d:%d", userID, peerUser, peerChat, peerChannel, msgID)
}

func factHash(text string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return int64(h.Sum64())
}

func saveFact(key string, text *mtproto.TextWithEntities) error {
	var body string
	if text != nil {
		body = text.GetText()
	}
	fc := mtproto.MakeTLFactCheck(&mtproto.FactCheck{
		Text: text,
		Hash: factHash(body),
	}).To_FactCheck()
	raw, err := json.Marshal(fc)
	if err != nil {
		return err
	}
	return persist.Default.Set(key, string(raw))
}

func loadFact(key string) (*mtproto.FactCheck, error) {
	raw, err := persist.Default.Get(key)
	if err != nil {
		return nil, err
	}
	if raw == "" || raw == "null" {
		return mtproto.MakeTLFactCheck(&mtproto.FactCheck{NeedCheck: true}).To_FactCheck(), nil
	}
	fc := &mtproto.FactCheck{}
	if err := json.Unmarshal([]byte(raw), fc); err != nil {
		return nil, err
	}
	return mtproto.MakeTLFactCheck(fc).To_FactCheck(), nil
}

func factCheckBoxMatches(userID int64, peerType int32, peerID int64, box *mtproto.MessageBox) bool {
	if box == nil || box.GetPeerId() != peerID {
		return false
	}
	if peerType == mtproto.PEER_SELF || (peerType == mtproto.PEER_USER && peerID == userID) {
		return box.GetPeerType() == mtproto.PEER_SELF || box.GetPeerType() == mtproto.PEER_USER
	}
	return box.GetPeerType() == peerType
}

func (c *ApiFullCore) validateFactCheckMessage(userID int64, peer *mtproto.InputPeer, msgID int32) error {
	if peer == nil {
		return mtproto.ErrPeerIdInvalid
	}
	if msgID <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	peerType, peerID := apifullPeerTypeID(userID, peer)
	if peerType <= mtproto.PEER_EMPTY || peerType == mtproto.PEER_UNKNOWN || peerID <= 0 {
		return mtproto.ErrPeerIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		return mtproto.ErrInternalServerError
	}
	box, err := d.PollMessageReader.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{
		UserId: userID,
		Id:     msgID,
	})
	if err != nil {
		return err
	}
	if box == nil || box.GetMessageId() != msgID || !factCheckBoxMatches(userID, peerType, peerID, box) {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

func (c *ApiFullCore) MessagesEditFactCheck(in *mtproto.TLMessagesEditFactCheck) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := c.validateFactCheckMessage(userID, in.GetPeer(), in.GetMsgId()); err != nil {
		return nil, err
	}
	if err := saveFact(factStoreKey(userID, in.GetPeer(), in.GetMsgId()), in.GetText()); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) MessagesDeleteFactCheck(in *mtproto.TLMessagesDeleteFactCheck) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := c.validateFactCheckMessage(userID, in.GetPeer(), in.GetMsgId()); err != nil {
		return nil, err
	}
	if err := persist.Default.Set(factStoreKey(userID, in.GetPeer(), in.GetMsgId()), ""); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) MessagesGetFactCheck(in *mtproto.TLMessagesGetFactCheck) (*mtproto.Vector_FactCheck, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	out := &mtproto.Vector_FactCheck{Datas: []*mtproto.FactCheck{}}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peerType, peerID := apifullPeerTypeID(userID, in.GetPeer())
	if peerType <= mtproto.PEER_EMPTY || peerType == mtproto.PEER_UNKNOWN || peerID <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(in.GetMsgId()) == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	for _, msgID := range in.GetMsgId() {
		if err := c.validateFactCheckMessage(userID, in.GetPeer(), msgID); err != nil {
			return nil, err
		}
		fc, err := loadFact(factStoreKey(userID, in.GetPeer(), msgID))
		if err != nil {
			return nil, err
		}
		out.Datas = append(out.Datas, fc)
	}
	return out, nil
}
