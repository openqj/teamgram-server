// Copyright 2025 Teamgram Authors
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
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UsersSuggestBirthday
// users.suggestBirthday#fc533372 id:InputUser birthday:Birthday = Updates;
func (c *UserChannelProfilesCore) UsersSuggestBirthday(in *mtproto.TLUsersSuggestBirthday) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	targetID, err := otherUserPeer(c.MD.UserId, in.GetId())
	if err != nil {
		c.Logger.Errorf("users.suggestBirthday - error: %v", err)
		return nil, err
	}
	if in.GetBirthday() == nil {
		c.Logger.Errorf("users.suggestBirthday - error: empty birthday")
		return nil, mtproto.ErrInputRequestInvalid
	}
	// A configured provider owns deployments that have an external birthday
	// workflow. Otherwise use the message service's durable PostgreSQL path so
	// the recipient receives the canonical service message and inbox fanout.
	if c.svcCtx != nil && c.svcCtx.Config.UserChannelProfilesProviderEndpoint != "" {
		return c.callBirthdaySuggestionProvider(c.ctx, birthdaySuggestionProviderRequest{
			CallerID: c.MD.UserId, TargetID: targetID, Recipient: in.GetId(), Birthday: in.GetBirthday(),
		})
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	peer, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: targetID})
	if err != nil {
		return nil, err
	}
	if peer == nil || peer.GetUser() == nil || peer.Deleted() || peer.GetUser().GetAccessHash() != in.GetId().GetAccessHash() {
		return nil, mtproto.ErrUserIdInvalid
	}
	var randomIDBytes [8]byte
	if _, err = rand.Read(randomIDBytes[:]); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	serviceAction := mtproto.MakeTLMessageActionSuggestBirthday(nil)
	serviceAction.SetBirthday(in.GetBirthday())
	serviceMessage := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(c.MD.UserId),
		PeerId: mtproto.MakePeerUser(targetID),
		Date:   int32(time.Now().Unix()),
		Action: serviceAction.To_MessageAction(),
	}).To_Message()
	updates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_USER,
		PeerId:    targetID,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: int64(binary.LittleEndian.Uint64(randomIDBytes[:])),
			Message:  serviceMessage,
		}).To_OutboxMessage()},
	})
	if err != nil {
		return nil, err
	}
	if updates == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return updates, nil
}
