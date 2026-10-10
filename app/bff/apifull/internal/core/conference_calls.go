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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func conferenceChainUpdates(record domain.GroupCall, subChainID int32, blocks [][]byte, nextOffset int32) *mtproto.Updates {
	update := mtproto.MakeTLUpdateGroupCallChainBlocks(&mtproto.Update{
		Call_INPUTGROUPCALL: groupCallInput(record),
		SubChainId:          subChainID,
		Blocks:              blocks,
		NextOffset:          nextOffset,
	}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{update},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}).To_Updates()
}

// RPCConferenceCallsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) PhoneCreateConferenceCall7D0444BB(in *mtproto.TLPhoneCreateConferenceCall7D0444BB) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in.GetRandomId() == 0 || len(in.GetPublicKey()) == 0 || len(in.GetBlock()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	accessHash, err := newGroupCallAccessHash()
	if err != nil {
		return nil, err
	}
	participants, err := json.Marshal([]int64{uid})
	if err != nil {
		return nil, err
	}
	callID := int64(in.GetRandomId())
	if callID < 0 {
		callID = -callID
	}
	if callID == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if existing, found, loadErr := domain.LoadGroupCallRecord(callID); loadErr != nil {
		return nil, loadErr
	} else if found {
		if existing.Creator != uid || !existing.Conference {
			return nil, mtproto.ErrRandomIdDuplicate
		}
		return groupCallUpdates(existing.ID, existing.AccessHash, true), nil
	}
	if err = domain.SaveConferenceCallWithMetadata(callID, accessHash, uid, 0, "conference", string(participants), nil, false); err != nil {
		return nil, err
	}
	params := ""
	if in.GetParams() != nil {
		params = in.GetParams().GetData()
	}
	if err = domain.SaveConferenceCallControl(callID, in.GetPublicKey(), in.GetBlock(), params); err != nil {
		return nil, err
	}
	if err = domain.SaveGroupCallSettings(domain.GroupCallSettings{CallID: callID, MessagesEnabled: true}); err != nil {
		return nil, err
	}
	if in.GetMuted() || in.GetVideoStopped() {
		if err = domain.SaveGroupCallParticipant(domain.GroupCallParticipantState{CallID: callID, UserID: uid, Muted: in.GetMuted(), VideoStopped: in.GetVideoStopped()}); err != nil {
			return nil, err
		}
	}
	return groupCallUpdates(callID, accessHash, true), nil
}

func (c *ApiFullCore) PhoneDeleteConferenceCallParticipants(in *mtproto.TLPhoneDeleteConferenceCallParticipants) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadConferenceCreator(in.GetCall())
	if err != nil {
		return nil, err
	}
	if len(in.GetIds()) == 0 {
		return nil, mtproto.ErrParticipantsTooFew
	}
	remove := make(map[int64]struct{}, len(in.GetIds()))
	for _, id := range in.GetIds() {
		if id <= 0 || id == record.Creator {
			return nil, mtproto.ErrGroupCallParticipantInvalid
		}
		remove[id] = struct{}{}
	}
	next := make([]int64, 0, len(ids))
	removed := make([]int64, 0, len(remove))
	for _, id := range ids {
		if _, ok := remove[id]; ok {
			removed = append(removed, id)
			continue
		}
		next = append(next, id)
	}
	if len(removed) == 0 {
		return nil, mtproto.ErrGroupcallNotModified
	}
	if err = saveGroupUserIDs(record.ID, record.AccessHash, record.Creator, next); err != nil {
		return nil, err
	}
	for _, id := range removed {
		if err = domain.DeleteGroupCallParticipant(record.ID, id); err != nil {
			return nil, err
		}
	}
	return groupCallParticipantUpdates(record, next, uid)
}

func (c *ApiFullCore) PhoneSendConferenceCallBroadcast(in *mtproto.TLPhoneSendConferenceCallBroadcast) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, record, _, err := c.loadConferenceCreator(in.GetCall())
	if err != nil {
		return nil, err
	}
	if len(in.GetBlock()) == 0 || len(in.GetBlock()) > 1<<20 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = domain.UpdateConferenceCallBlock(record.ID, in.GetBlock()); err != nil {
		return nil, err
	}
	return conferenceChainUpdates(record, 0, [][]byte{append([]byte(nil), in.GetBlock()...)}, 0), nil
}

func (c *ApiFullCore) PhoneInviteConferenceCallParticipant(in *mtproto.TLPhoneInviteConferenceCallParticipant) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetCall() == nil || in.GetCall().GetId() == 0 || in.GetUserId() == nil {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadConferenceCreator(in.GetCall())
	if err != nil {
		return nil, err
	}
	targets, err := c.resolveGroupCallInviteeIDs(uid, []*mtproto.InputUser{in.GetUserId()})
	if err != nil {
		return nil, err
	}
	if len(targets) != 1 || targets[0] <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	target := targets[0]
	if groupCallHasUser(ids, target) {
		return nil, mtproto.ErrGroupcallNotModified
	}
	ids = append(ids, target)
	if err = saveGroupUserIDs(record.ID, record.AccessHash, record.Creator, ids); err != nil {
		return nil, err
	}
	return groupCallParticipantUpdates(record, ids, uid)
}

func (c *ApiFullCore) PhoneDeclineConferenceCallInvite(in *mtproto.TLPhoneDeclineConferenceCallInvite) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		return nil, mtproto.ErrInternalServerError
	}
	box, err := d.PollMessageReader.MessageGetUserMessage(callContext(c), &messagepb.TLMessageGetUserMessage{
		UserId: uid,
		Id:     in.GetMsgId(),
	})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetMessageId() != in.GetMsgId() || box.GetMessage() == nil {
		return nil, mtproto.ErrMsgIdInvalid
	}
	action := box.GetMessage().GetAction()
	if action == nil || action.GetPredicateName() != mtproto.Predicate_messageActionConferenceCall {
		return nil, mtproto.ErrMsgIdInvalid
	}
	call := action.GetCall()
	if call == nil || call.GetId() <= 0 || call.GetAccessHash() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	record, ok, err := domain.LoadGroupCallRecord(call.GetId())
	if err != nil {
		return nil, err
	}
	if !ok || !record.Conference || record.AccessHash != call.GetAccessHash() {
		return nil, mtproto.ErrGroupCallInvalid
	}
	if uid == record.Creator {
		return nil, mtproto.ErrGroupcallForbidden
	}
	ids := decodeGroupUserIDs(record.Participants)
	if !groupCallHasUser(ids, uid) {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	next := make([]int64, 0, len(ids)-1)
	for _, id := range ids {
		if id != uid {
			next = append(next, id)
		}
	}
	if err = saveGroupUserIDs(record.ID, record.AccessHash, record.Creator, next); err != nil {
		return nil, err
	}
	if err = domain.DeleteGroupCallParticipant(record.ID, uid); err != nil {
		return nil, err
	}
	return groupCallParticipantUpdates(record, next, uid)
}

func (c *ApiFullCore) PhoneGetGroupCallChainBlocks(in *mtproto.TLPhoneGetGroupCallChainBlocks) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	_, record, _, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if !record.Conference {
		return nil, mtproto.ErrGroupCallInvalid
	}
	if in.GetSubChainId() < 0 || in.GetOffset() < 0 || in.GetLimit() < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	blocks := make([][]byte, 0, 1)
	if in.GetSubChainId() == 0 && in.GetOffset() == 0 && (in.GetLimit() == 0 || in.GetLimit() >= 1) {
		if block, found, loadErr := domain.LoadConferenceCallBlock(record.ID); loadErr != nil {
			return nil, loadErr
		} else if found {
			blocks = append(blocks, block)
		}
	}
	return conferenceChainUpdates(record, in.GetSubChainId(), blocks, 0), nil
}

func (c *ApiFullCore) PhoneSendGroupCallEncryptedMessage(in *mtproto.TLPhoneSendGroupCallEncryptedMessage) (*mtproto.Bool, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	if len(in.GetEncryptedMessage()) == 0 || len(in.GetEncryptedMessage()) > 1<<20 {
		return nil, mtproto.ErrEncryptedMessageInvalid
	}
	uid, record, _, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if err = domain.SaveGroupCallEncryptedMessage(record.ID, uid, in.GetEncryptedMessage()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PhoneCreateConferenceCallDFC909AB(in *mtproto.TLPhoneCreateConferenceCallDFC909AB) (*mtproto.Phone_PhoneCall, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) loadConferenceCreator(call *mtproto.InputGroupCall) (int64, domain.GroupCall, []int64, error) {
	uid, record, ids, err := c.loadCreatorGroupCall(call)
	if err != nil {
		return 0, domain.GroupCall{}, nil, err
	}
	if !record.Conference {
		return 0, domain.GroupCall{}, nil, mtproto.ErrGroupCallInvalid
	}
	return uid, record, ids, nil
}
