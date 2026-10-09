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
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCEphemeralServer: Layer 229 ephemeral messages are short-lived protocol
// records. They are kept in the same PostgreSQL-backed atomic KV used by the
// rest of APIFull state so retries cannot create duplicate random IDs.

type ephemeralRecord struct {
	Message  *mtproto.EphemeralMessage `json:"message"`
	RandomID int64                     `json:"random_id,omitempty"`
}

func ephemeralRecords(raw string) ([]ephemeralRecord, error) {
	if raw == "" {
		return nil, nil
	}
	var records []ephemeralRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, fmt.Errorf("decode ephemeral state: %w", err)
	}
	return records, nil
}

func ephemeralPeer(peer *mtproto.InputPeer, userID int64) (*mtproto.Peer, string, int64, error) {
	if peer == nil {
		return nil, "", 0, mtproto.ErrPeerIdInvalid
	}
	name := peer.GetPredicateName()
	if name == "" {
		switch {
		case peer.GetChannelId() > 0:
			name = mtproto.Predicate_inputPeerChannel
		case peer.GetChatId() > 0:
			name = mtproto.Predicate_inputPeerChat
		case peer.GetUserId() > 0:
			name = mtproto.Predicate_inputPeerUser
		default:
			name = mtproto.Predicate_inputPeerSelf
		}
	}
	switch name {
	case mtproto.Predicate_inputPeerSelf:
		if peer.GetUserId() != 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 {
			return nil, "", 0, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerUser(userID), "user", userID, nil
	case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
		if peer.GetUserId() <= 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 {
			return nil, "", 0, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerUser(peer.GetUserId()), "user", peer.GetUserId(), nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 || peer.GetUserId() != 0 || peer.GetChannelId() != 0 {
			return nil, "", 0, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerChat(peer.GetChatId()), "chat", peer.GetChatId(), nil
	case mtproto.Predicate_inputPeerChannel, mtproto.Predicate_inputPeerChannelFromMessage:
		if peer.GetChannelId() <= 0 || peer.GetUserId() != 0 || peer.GetChatId() != 0 {
			return nil, "", 0, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerChannel(peer.GetChannelId()), "channel", peer.GetChannelId(), nil
	default:
		return nil, "", 0, mtproto.ErrPeerIdInvalid
	}
}

func ephemeralStoreKey(userID int64, peer *mtproto.InputPeer, receiverID int64) (*mtproto.Peer, string, error) {
	messagePeer, peerType, peerID, err := ephemeralPeer(peer, userID)
	if err != nil {
		return nil, "", err
	}
	if receiverID < 0 {
		return nil, "", mtproto.ErrUserIdInvalid
	}
	// User conversations are shared by both participants. Group and channel
	// conversations are keyed by their peer and retain their normal identity.
	if peerType == "user" {
		selfPeer := peer.GetPredicateName() == mtproto.Predicate_inputPeerSelf ||
			(peer.GetPredicateName() == "" && peer.GetUserId() == 0 && peer.GetChatId() == 0 && peer.GetChannelId() == 0)
		if receiverID > 0 && receiverID != peerID && receiverID != userID && !selfPeer {
			return nil, "", mtproto.ErrPeerIdInvalid
		}
		other := peerID
		if other == userID && receiverID > 0 {
			other = receiverID
		}
		left, right := userID, other
		if left > right {
			left, right = right, left
		}
		return messagePeer, "ephemeral:user:" + strconv.FormatInt(left, 10) + ":" + strconv.FormatInt(right, 10), nil
	}
	return messagePeer, "ephemeral:" + peerType + ":" + strconv.FormatInt(peerID, 10), nil
}

func (c *ApiFullCore) EphemeralSendMessage(in *mtproto.TLEphemeralSendMessage) (*mtproto.Updates, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetMessage() == "" && in.GetMedia() == nil && in.GetRichMessage() == nil {
		return nil, mtproto.ErrMessageEmpty
	}
	// APIFull has no media/rich-message resolver for ephemeral records. Do not
	// persist an incomplete object while claiming success.
	if in.GetMedia() != nil || in.GetRichMessage() != nil {
		return nil, mtproto.ErrMediaInvalid
	}
	receiverID := int64(0)
	if in.GetReceiverId() != nil {
		receiverID = in.GetReceiverId().GetUserId()
		if receiverID <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	}
	peer, key, err := ephemeralStoreKey(userID, in.GetPeer(), receiverID)
	if err != nil {
		return nil, err
	}
	if receiverID == 0 && peer.GetUserId() > 0 {
		receiverID = peer.GetUserId()
	}
	if receiverID > 0 && peer.GetUserId() == userID &&
		(in.GetPeer().GetPredicateName() == mtproto.Predicate_inputPeerSelf ||
			(in.GetPeer().GetPredicateName() == "" && in.GetPeer().GetUserId() == 0 && in.GetPeer().GetChatId() == 0 && in.GetPeer().GetChannelId() == 0)) {
		peer = mtproto.MakePeerUser(receiverID)
	}
	var message *mtproto.EphemeralMessage
	err = persist.Update(key, func(raw string) (string, error) {
		records, err := ephemeralRecords(raw)
		if err != nil {
			return "", err
		}
		if in.GetRandomId() != 0 {
			for _, record := range records {
				if record.RandomID == in.GetRandomId() && record.Message != nil {
					message = record.Message
					return raw, nil
				}
			}
		}
		var nextID int32 = 1
		for _, record := range records {
			if record.Message != nil && record.Message.GetId() >= nextID {
				nextID = record.Message.GetId() + 1
			}
		}
		if nextID <= 0 {
			return "", mtproto.ErrMessageIdInvalid
		}
		message = &mtproto.EphemeralMessage{
			PredicateName: mtproto.Predicate_ephemeralMessage,
			Constructor:   mtproto.TLConstructor_CRC32_ephemeralMessage,
			Out:           true,
			Id:            nextID,
			FromId:        mtproto.MakePeerUser(userID),
			PeerId:        peer,
			ReceiverId:    receiverID,
			Date:          int32(time.Now().Unix()),
			Message:       in.GetMessage(),
			Entities:      in.GetEntities(),
			ReplyMarkup:   in.GetReplyMarkup(),
		}
		records = append(records, ephemeralRecord{Message: message, RandomID: in.GetRandomId()})
		encoded, err := json.Marshal(records)
		return string(encoded), err
	})
	if err != nil {
		return nil, err
	}
	update := mtproto.MakeTLUpdateNewEphemeralMessage(&mtproto.Update{})
	update.SetMessage_EPHEMERALMESSAGE(message)
	return mtproto.MakeUpdatesByUpdates(update.To_Update()), nil
}

func (c *ApiFullCore) EphemeralDeleteMessage(in *mtproto.TLEphemeralDeleteMessage) (*mtproto.Bool, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	receiverID := int64(0)
	if in.GetReceiverId() != nil {
		receiverID = in.GetReceiverId().GetUserId()
		if receiverID <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	}
	_, key, err := ephemeralStoreKey(userID, in.GetPeer(), receiverID)
	if err != nil {
		return nil, err
	}
	deleted := false
	err = persist.Update(key, func(raw string) (string, error) {
		records, err := ephemeralRecords(raw)
		if err != nil {
			return "", err
		}
		kept := records[:0]
		for _, record := range records {
			if record.Message != nil && record.Message.GetId() == in.GetId() {
				deleted = true
				continue
			}
			kept = append(kept, record)
		}
		encoded, err := json.Marshal(kept)
		return string(encoded), err
	})
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) EphemeralReportMessage(in *mtproto.TLEphemeralReportMessage) (*mtproto.ReportResult, error) {
	userID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	target, err := reportPeerTarget(userID, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(userID, "ephemeral.reportMessage", target, in); err != nil {
		return nil, err
	}
	return mtproto.MakeTLReportResultReported(nil).To_ReportResult(), nil
}

func (c *ApiFullCore) EphemeralGetCallbackAnswer(in *mtproto.TLEphemeralGetCallbackAnswer) (*mtproto.Messages_BotCallbackAnswer, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
