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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCGroupCallsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) gcallPut(op string, in any) error {
	uid, err := c.requireUserId()
	if err != nil {
		return err
	}
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set("gcall:"+strconv.FormatInt(uid, 10)+":"+op, string(b))
}

func (c *ApiFullCore) storeCallID(prefix string) (int64, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, err
	}
	id := time.Now().UnixNano()
	if id <= 0 {
		id = 1
	}
	if err = persist.Default.Set(prefix+strconv.FormatInt(uid, 10), strconv.FormatInt(id, 10)); err != nil {
		return 0, err
	}
	return id, nil
}

func newGroupCallAccessHash() (int64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	accessHash := int64(binary.LittleEndian.Uint64(raw[:]) & uint64(^uint64(0)>>1))
	if accessHash == 0 {
		accessHash = 1
	}
	return accessHash, nil
}

func loadGroupUserIDs(id int64) ([]int64, bool, error) {
	if id == 0 {
		return nil, false, nil
	}
	raw, ok, err := domain.LoadGroupCall(id)
	if err != nil || !ok {
		return nil, ok, err
	}
	var ids []int64
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return []int64{}, true, nil
	}
	return ids, true, nil
}

func saveGroupUserIDs(id, access, creator int64, ids []int64) error {
	if ids == nil {
		ids = []int64{}
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, ok, err := domain.LoadGroupCallRecord(id)
	if err != nil {
		return err
	}
	if !ok {
		return domain.SaveGroupCall(id, access, creator, 0, "", string(raw))
	}
	updated, err := domain.UpdateGroupCallParticipants(id, string(raw))
	if err != nil {
		return err
	}
	if !updated {
		return mtproto.ErrGroupCallInvalid
	}
	return nil
}

func (c *ApiFullCore) resolveGroupCallID(call *mtproto.InputGroupCall) (int64, error) {
	if call != nil && call.GetId() != 0 {
		return call.GetId(), nil
	}
	uid, err := c.requireUserId()
	if err != nil {
		return 0, err
	}
	raw, err := persist.Default.Get("gcall:" + strconv.FormatInt(uid, 10))
	if err != nil {
		return 0, err
	}
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id, nil
}

func groupCallHasUser(ids []int64, userID int64) bool {
	for _, id := range ids {
		if id == userID {
			return true
		}
	}
	return false
}

func decodeGroupUserIDs(raw string) []int64 {
	var ids []int64
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return []int64{}
	}
	return ids
}

func (c *ApiFullCore) loadAuthorizedGroupCall(call *mtproto.InputGroupCall) (int64, domain.GroupCall, []int64, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, domain.GroupCall{}, nil, err
	}
	id, err := c.resolveGroupCallID(call)
	if err != nil || id == 0 {
		if err != nil {
			return 0, domain.GroupCall{}, nil, err
		}
		return 0, domain.GroupCall{}, nil, mtproto.ErrGroupCallInvalid
	}
	record, ok, err := domain.LoadGroupCallRecord(id)
	if err != nil {
		return 0, domain.GroupCall{}, nil, err
	}
	if !ok || (call != nil && call.GetId() != 0 && call.GetAccessHash() != record.AccessHash) {
		return 0, domain.GroupCall{}, nil, mtproto.ErrGroupCallInvalid
	}
	ids := decodeGroupUserIDs(record.Participants)
	if record.ChannelID != 0 {
		if err = c.requireGroupCallChannelMember(uid, record.ChannelID); err != nil {
			return 0, domain.GroupCall{}, nil, err
		}
	} else if uid != record.Creator && !groupCallHasUser(ids, uid) {
		return 0, domain.GroupCall{}, nil, mtproto.ErrGroupcallForbidden
	}
	return uid, record, ids, nil
}

// Channel creators are not stored in apifull_channel_member, but they retain
// access to calls attached to their channel.
func (c *ApiFullCore) requireGroupCallChannelMember(userID, channelID int64) error {
	channel, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return c.mapChannelMemberError(err)
	}
	if !ok {
		return mtproto.ErrChannelInvalid
	}
	if channel.Creator == userID {
		return nil
	}
	return c.requireChannelMember(userID, channelID)
}

func (c *ApiFullCore) loadCreatorGroupCall(call *mtproto.InputGroupCall) (int64, domain.GroupCall, []int64, error) {
	uid, record, ids, err := c.loadAuthorizedGroupCall(call)
	if err != nil {
		return 0, domain.GroupCall{}, nil, err
	}
	if uid != record.Creator {
		return 0, domain.GroupCall{}, nil, mtproto.ErrChatAdminRequired
	}
	return uid, record, ids, nil
}

func (c *ApiFullCore) resolveGroupCallInviteeIDs(caller int64, users []*mtproto.InputUser) ([]int64, error) {
	if len(users) == 0 {
		return nil, mtproto.ErrUsersTooFew
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ids := make([]int64, 0, len(users))
	seen := make(map[int64]struct{}, len(users))
	for _, input := range users {
		if input == nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		peer := mtproto.FromInputUser(caller, input)
		if peer == nil || (peer.PeerType != mtproto.PEER_SELF && peer.PeerType != mtproto.PEER_USER) || peer.PeerId <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		target, err := d.UserClient.UserGetImmutableUserV2(ctx, &userpb.TLUserGetImmutableUserV2{Id: peer.PeerId})
		if err != nil {
			return nil, err
		}
		if target == nil || target.GetUser() == nil || target.GetUser().GetId() != peer.PeerId {
			return nil, mtproto.ErrUserIdInvalid
		}
		if target.GetUser().GetDeleted() {
			return nil, mtproto.ErrInputUserDeactivated
		}
		if peer.PeerType == mtproto.PEER_USER && (input.GetAccessHash() == 0 || input.GetAccessHash() != target.GetUser().GetAccessHash()) {
			return nil, mtproto.ErrUserIdInvalid
		}
		if _, ok := seen[peer.PeerId]; ok {
			continue
		}
		seen[peer.PeerId] = struct{}{}
		ids = append(ids, peer.PeerId)
	}
	return ids, nil
}

func (c *ApiFullCore) groupCallChannelID(userID int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil || peer.GetChannelId() == 0 {
		return 0, nil
	}
	if peer.GetPredicateName() != mtproto.Predicate_inputPeerChannel || peer.GetUserId() != 0 || peer.GetChatId() != 0 {
		return 0, mtproto.ErrPeerIdInvalid
	}
	if peer.GetAccessHash() == 0 {
		return 0, mtproto.ErrChannelInvalid
	}
	channel, ok, err := domain.LoadChannel(peer.GetChannelId())
	if err != nil {
		return 0, err
	}
	if !ok || channel.AccessHash != peer.GetAccessHash() {
		return 0, mtproto.ErrChannelInvalid
	}
	if err = c.requireGroupCallChannelMember(userID, channel.ID); err != nil {
		return 0, err
	}
	return channel.ID, nil
}

func participantSource(id int64) (int32, bool) {
	if id <= 0 || id > 2147483647 {
		return 0, false
	}
	return int32(id), true
}

func filterGroupUserIDs(ids []int64, want []*mtproto.InputPeer, sources []int32) []int64 {
	if len(want) > 0 {
		set := make(map[int64]struct{}, len(want))
		for _, p := range want {
			if p != nil && p.GetUserId() != 0 {
				set[p.GetUserId()] = struct{}{}
			}
		}
		next := make([]int64, 0, len(ids))
		for _, id := range ids {
			if _, ok := set[id]; ok {
				next = append(next, id)
			}
		}
		ids = next
	}
	if len(sources) > 0 {
		set := make(map[int32]struct{}, len(sources))
		for _, s := range sources {
			set[s] = struct{}{}
		}
		next := make([]int64, 0, len(ids))
		for _, id := range ids {
			src, ok := participantSource(id)
			if !ok {
				continue
			}
			if _, hit := set[src]; hit {
				next = append(next, id)
			}
		}
		ids = next
	}
	return ids
}

func pageGroupUserIDs(ids []int64, offset string, limit int32) ([]int64, string) {
	start := 0
	if offset != "" {
		if n, err := strconv.Atoi(offset); err == nil && n > 0 {
			if n > len(ids) {
				n = len(ids)
			}
			start = n
		}
	}
	rest := ids[start:]
	if limit > 0 && int(limit) < len(rest) {
		rest = rest[:int(limit)]
		return rest, strconv.Itoa(start + len(rest))
	}
	return rest, ""
}

func groupParticipantViews(ids []int64, self, callID int64) ([]*mtproto.GroupCallParticipant, []*mtproto.User, error) {
	parts := make([]*mtproto.GroupCallParticipant, 0, len(ids))
	users := make([]*mtproto.User, 0, len(ids))
	for _, id := range ids {
		state, ok, err := domain.LoadGroupCallParticipant(callID, id)
		if err != nil {
			return nil, nil, err
		}
		muted := ok && state.Muted
		p := &mtproto.GroupCallParticipant{
			Peer:          mtproto.MakePeerUser(id),
			Self:          id == self,
			CanSelfUnmute: !muted,
			Muted:         muted,
		}
		if src, ok := participantSource(id); ok {
			p.Source = src
		}
		if ok && state.Volume != nil {
			p.Volume = wrapperspb.Int32(*state.Volume)
		}
		if ok && state.RaiseHand {
			p.RaiseHandRating = wrapperspb.Int64(1)
		}
		if ok && state.PresentationActive {
			// The media provider owns endpoint/source details. Expose only the
			// persisted active/paused state until that provider is configured.
			p.Presentation = &mtproto.GroupCallParticipantVideo{Paused: state.PresentationPaused}
		}
		parts = append(parts, mtproto.MakeTLGroupCallParticipant(p).To_GroupCallParticipant())
		users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: id}).To_User())
	}
	return parts, users, nil
}

func loadCreateForCall(callID, creatorID int64) (mtproto.TLPhoneCreateGroupCall, bool, error) {
	var zero mtproto.TLPhoneCreateGroupCall
	raw, err := persist.Default.Get("gcall:" + strconv.FormatInt(creatorID, 10))
	if err != nil {
		return zero, false, err
	}
	stored, _ := strconv.ParseInt(raw, 10, 64)
	if stored != callID {
		return zero, false, nil
	}
	blob, err := persist.Default.Get("gcall:" + strconv.FormatInt(creatorID, 10) + ":PhoneCreateGroupCall")
	if err != nil {
		return zero, false, err
	}
	if blob == "" || blob == "null" || json.Unmarshal([]byte(blob), &zero) != nil {
		return zero, false, nil
	}
	return zero, true, nil
}

func sameInputPeer(a, b *mtproto.InputPeer) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.GetPredicateName() == b.GetPredicateName() &&
		a.GetUserId() == b.GetUserId() && a.GetChatId() == b.GetChatId() &&
		a.GetChannelId() == b.GetChannelId() && a.GetAccessHash() == b.GetAccessHash()
}

func groupCallInputPeer(p *mtproto.InputPeer) *mtproto.Peer {
	if p == nil {
		return nil
	}
	if id := p.GetUserId(); id != 0 {
		return mtproto.MakePeerUser(id)
	}
	if id := p.GetChatId(); id != 0 {
		return mtproto.MakePeerChat(id)
	}
	if id := p.GetChannelId(); id != 0 {
		return mtproto.MakePeerChannel(id)
	}
	return nil
}

func peerListed(peers []*mtproto.Peer, p *mtproto.Peer) bool {
	if p == nil {
		return true
	}
	for _, have := range peers {
		if have.GetUserId() == p.GetUserId() && have.GetChatId() == p.GetChatId() && have.GetChannelId() == p.GetChannelId() &&
			(have.GetUserId() != 0 || have.GetChatId() != 0 || have.GetChannelId() != 0) {
			return true
		}
	}
	return false
}

func groupCallUpdates(id, accessHash int64, conference bool) *mtproto.Updates {
	call := mtproto.MakeTLGroupCall(&mtproto.GroupCall{
		Id:         id,
		AccessHash: accessHash,
		Conference: conference,
	}).To_GroupCall()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdateGroupCall(&mtproto.Update{Call_GROUPCALL: call}).To_Update(),
		},
		Users: []*mtproto.User{},
		Chats: []*mtproto.Chat{},
	}).To_Updates()
}

func groupCallInput(record domain.GroupCall) *mtproto.InputGroupCall {
	return mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{
		Id:         record.ID,
		AccessHash: record.AccessHash,
	}).To_InputGroupCall()
}

func groupCallParticipantUpdates(record domain.GroupCall, ids []int64, self int64) (*mtproto.Updates, error) {
	parts, users, err := groupParticipantViews(ids, self, record.ID)
	if err != nil {
		return nil, err
	}
	update := mtproto.MakeTLUpdateGroupCallParticipants(&mtproto.Update{
		Call_INPUTGROUPCALL:                     groupCallInput(record),
		Participants_VECTORGROUPCALLPARTICIPANT: parts,
		Version:                                 int32(len(ids)),
	}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{update},
		Users:   users,
		Chats:   []*mtproto.Chat{},
	}).To_Updates(), nil
}

func groupCallPeerUserID(caller int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil {
		return 0, mtproto.ErrGroupCallParticipantInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if peer.GetUserId() != 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 || peer.GetAccessHash() != 0 {
			return 0, mtproto.ErrGroupCallParticipantInvalid
		}
		return caller, nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 {
			return 0, mtproto.ErrGroupCallParticipantInvalid
		}
		return peer.GetUserId(), nil
	default:
		return 0, mtproto.ErrGroupCallParticipantInvalid
	}
}

func boolValue(value *mtproto.Bool) bool {
	return value != nil && mtproto.FromBool(value)
}

func groupCallMessageUpdates(record domain.GroupCall, message domain.GroupCallMessage, text *mtproto.TextWithEntities, senderID int64) *mtproto.Updates {
	update := mtproto.MakeTLUpdateGroupCallMessage(&mtproto.Update{
		Call_INPUTGROUPCALL: groupCallInput(record),
		Message_GROUPCALLMESSAGE: mtproto.MakeTLGroupCallMessage(&mtproto.GroupCallMessage{
			Id:      int32(message.ID),
			FromId:  mtproto.MakePeerUser(senderID),
			Date:    int32(message.Date),
			Message: text,
		}).To_GroupCallMessage(),
		FromId:                   mtproto.MakePeerUser(senderID),
		RandomId:                 message.RandomID,
		Message_TEXTWITHENTITIES: text,
	}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{update},
		Users:   []*mtproto.User{mtproto.MakeTLUser(&mtproto.User{Id: senderID}).To_User()},
		Chats:   []*mtproto.Chat{},
	}).To_Updates()
}

func groupCallDeleteUpdates(record domain.GroupCall, ids []int32) *mtproto.Updates {
	update := mtproto.MakeTLUpdateDeleteGroupCallMessages(&mtproto.Update{
		Call_INPUTGROUPCALL: groupCallInput(record),
		Messages:            ids,
	}).To_Update()
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{update},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}).To_Updates()
}

func (c *ApiFullCore) validateGroupCallSendAs(uid int64, ids []int64, sendAs *mtproto.InputPeer) error {
	if sendAs == nil {
		return nil
	}
	if !validDefaultGroupCallPeer(sendAs) {
		return mtproto.ErrPeerIdInvalid
	}
	switch sendAs.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return nil
	case mtproto.Predicate_inputPeerUser:
		if !groupCallHasUser(ids, sendAs.GetUserId()) {
			return mtproto.ErrGroupCallParticipantInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerChannel:
		if _, err := c.groupCallChannelID(uid, sendAs); err != nil {
			return err
		}
		return nil
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

func (c *ApiFullCore) persistGroupCallMessage(record domain.GroupCall, uid int64, ids []int64, randomID int64, message *mtproto.TextWithEntities, allowPaid *wrapperspb.Int64Value, sendAs *mtproto.InputPeer) (domain.GroupCallMessage, error) {
	if randomID == 0 {
		return domain.GroupCallMessage{}, mtproto.ErrRandomIdEmpty
	}
	if message == nil || message.GetText() == "" {
		return domain.GroupCallMessage{}, mtproto.ErrMessageEmpty
	}
	if allowPaid != nil && allowPaid.GetValue() > 0 {
		return domain.GroupCallMessage{}, mtproto.ErrMethodNotImpl
	}
	settings, err := domain.LoadGroupCallSettings(record.ID)
	if err != nil {
		return domain.GroupCallMessage{}, err
	}
	if !settings.MessagesEnabled {
		return domain.GroupCallMessage{}, mtproto.ErrChatWriteForbidden
	}
	if err = c.validateGroupCallSendAs(uid, ids, sendAs); err != nil {
		return domain.GroupCallMessage{}, err
	}
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return domain.GroupCallMessage{}, err
	}
	sendAsJSON := ""
	if sendAs != nil {
		blob, marshalErr := json.Marshal(sendAs)
		if marshalErr != nil {
			return domain.GroupCallMessage{}, marshalErr
		}
		sendAsJSON = string(blob)
	} else if saved, ok, loadErr := domain.LoadGroupCallSendAs(record.ID, uid); loadErr != nil {
		return domain.GroupCallMessage{}, loadErr
	} else if ok {
		sendAsJSON = saved
	}
	stored, _, err := domain.CreateGroupCallMessage(record.ID, uid, randomID, string(messageJSON), sendAsJSON, nil)
	if err != nil {
		return domain.GroupCallMessage{}, err
	}
	if stored.SenderID != uid {
		return domain.GroupCallMessage{}, mtproto.ErrRandomIdDuplicate
	}
	return stored, nil
}

func (c *ApiFullCore) PhoneCreateGroupCall(in *mtproto.TLPhoneCreateGroupCall) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	var peer *mtproto.InputPeer
	if in != nil {
		peer = in.GetPeer()
	}
	channelID, err := c.groupCallChannelID(uid, peer)
	if err != nil {
		return nil, err
	}
	if err = c.gcallPut("PhoneCreateGroupCall", in); err != nil {
		return nil, err
	}
	id, err := c.storeCallID("gcall:")
	if err != nil {
		return nil, err
	}
	accessHash, err := newGroupCallAccessHash()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal([]int64{uid})
	if err != nil {
		return nil, err
	}
	var scheduleDate *int32
	if in.GetScheduleDate() != nil {
		value := in.GetScheduleDate().GetValue()
		scheduleDate = &value
	}
	if err = domain.SaveGroupCallWithMetadata(id, accessHash, uid, channelID, in.GetTitle().GetValue(), string(raw), scheduleDate, in.GetRtmpStream()); err != nil {
		return nil, err
	}
	if err = domain.SaveGroupCallSettings(domain.GroupCallSettings{CallID: id, MessagesEnabled: true}); err != nil {
		return nil, err
	}
	return groupCallUpdates(id, accessHash, false), nil
}

func (c *ApiFullCore) PhoneJoinGroupCall(in *mtproto.TLPhoneJoinGroupCall) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	var uid int64
	var record domain.GroupCall
	var ids []int64
	var invitedSelfUnmute bool
	usedInvite := false
	var err error
	if invite := in.GetInviteHash(); invite != nil && strings.TrimSpace(invite.GetValue()) != "" {
		uid, err = c.requireUserId()
		if err != nil {
			return nil, err
		}
		loaded, ok, loadErr := domain.LoadGroupCallRecord(in.GetCall().GetId())
		if loadErr != nil {
			return nil, loadErr
		}
		if !ok || loaded.AccessHash != in.GetCall().GetAccessHash() {
			return nil, mtproto.ErrGroupCallInvalid
		}
		record = loaded
		valid, canSelfUnmute, checkErr := domain.CheckGroupCallInvite(record.ID, invite.GetValue())
		if checkErr != nil {
			return nil, checkErr
		}
		if !valid {
			return nil, mtproto.ErrGroupCallInvalid
		}
		invitedSelfUnmute = canSelfUnmute
		usedInvite = true
		ids = decodeGroupUserIDs(record.Participants)
	} else {
		uid, record, ids, err = c.loadAuthorizedGroupCall(in.GetCall())
		if err != nil {
			return nil, err
		}
	}
	if !groupCallHasUser(ids, uid) {
		ids = append(ids, uid)
		if err := saveGroupUserIDs(record.ID, record.AccessHash, record.Creator, ids); err != nil {
			return nil, err
		}
	}
	settings, err := domain.LoadGroupCallSettings(record.ID)
	if err != nil {
		return nil, err
	}
	muted := in.GetMuted()
	if settings.JoinMuted {
		muted = true
	}
	if usedInvite && !invitedSelfUnmute {
		muted = true
	}
	if err := domain.SaveGroupCallParticipant(domain.GroupCallParticipantState{
		CallID:       record.ID,
		UserID:       uid,
		Muted:        muted,
		VideoStopped: in.GetVideoStopped(),
		JoinParams:   in.GetParams().GetData(),
	}); err != nil {
		return nil, err
	}
	if err := c.gcallPut("PhoneJoinGroupCall", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PhoneLeaveGroupCall(in *mtproto.TLPhoneLeaveGroupCall) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if !groupCallHasUser(ids, uid) {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	if _, joined, loadErr := domain.LoadGroupCallParticipant(record.ID, uid); loadErr != nil {
		return nil, loadErr
	} else if !joined {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	next := make([]int64, 0, len(ids)-1)
	for _, id := range ids {
		if id != uid {
			next = append(next, id)
		}
	}
	if err := saveGroupUserIDs(record.ID, record.AccessHash, record.Creator, next); err != nil {
		return nil, err
	}
	if err := domain.DeleteGroupCallParticipant(record.ID, uid); err != nil {
		return nil, err
	}
	if err = c.gcallPut("PhoneLeaveGroupCall", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PhoneInviteToGroupCall(in *mtproto.TLPhoneInviteToGroupCall) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	invitees, err := c.resolveGroupCallInviteeIDs(uid, in.GetUsers())
	if err != nil {
		return nil, err
	}
	for _, invitee := range invitees {
		if !groupCallHasUser(ids, invitee) {
			ids = append(ids, invitee)
		}
	}
	if err = saveGroupUserIDs(record.ID, record.AccessHash, record.Creator, ids); err != nil {
		return nil, err
	}
	if err := c.gcallPut("PhoneInviteToGroupCall", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PhoneDiscardGroupCall(in *mtproto.TLPhoneDiscardGroupCall) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	_, record, _, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	deleted, err := domain.DeleteGroupCall(record.ID)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, mtproto.ErrGroupCallInvalid
	}
	if err := c.gcallPut("PhoneDiscardGroupCall", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PhoneToggleGroupCallSettings(in *mtproto.TLPhoneToggleGroupCallSettings) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	_, record, _, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if in.GetResetInviteHash() {
		if err = domain.RevokeGroupCallInvite(record.ID, record.Creator); err != nil {
			return nil, err
		}
	}
	settings, err := domain.LoadGroupCallSettings(record.ID)
	if err != nil {
		return nil, err
	}
	changed := in.GetResetInviteHash()
	if in.GetJoinMuted() != nil {
		value := boolValue(in.GetJoinMuted())
		changed = changed || settings.JoinMuted != value
		settings.JoinMuted = value
	}
	if in.GetMessagesEnabled() != nil {
		value := boolValue(in.GetMessagesEnabled())
		changed = changed || settings.MessagesEnabled != value
		settings.MessagesEnabled = value
	}
	if in.GetSendPaidMessagesStars() != nil {
		if in.GetSendPaidMessagesStars().GetValue() < 0 {
			return nil, mtproto.ErrCurrencyTotalAmountInvalid
		}
		value := in.GetSendPaidMessagesStars().GetValue()
		changed = changed || settings.SendPaidMessagesStars == nil || *settings.SendPaidMessagesStars != value
		settings.SendPaidMessagesStars = &value
	}
	if !changed {
		return nil, mtproto.ErrGroupcallNotModified
	}
	if err = domain.SaveGroupCallSettings(settings); err != nil {
		return nil, err
	}
	return groupCallUpdates(record.ID, record.AccessHash, false), nil
}

func (c *ApiFullCore) PhoneGetGroupCall(in *mtproto.TLPhoneGetGroupCall) (*mtproto.Phone_GroupCall, error) {
	var call *mtproto.InputGroupCall
	var limit int32
	if in != nil {
		call = in.GetCall()
		limit = in.GetLimit()
	}
	uid, record, all, err := c.loadAuthorizedGroupCall(call)
	if err != nil {
		return nil, err
	}
	shown := all
	if limit > 0 && int(limit) < len(shown) {
		shown = shown[:int(limit)]
	}
	parts, users, err := groupParticipantViews(shown, uid, record.ID)
	if err != nil {
		return nil, err
	}
	settings, err := domain.LoadGroupCallSettings(record.ID)
	if err != nil {
		return nil, err
	}
	gc := &mtproto.GroupCall{
		Id:                      record.ID,
		AccessHash:              record.AccessHash,
		Conference:              record.Conference,
		JoinMuted:               settings.JoinMuted,
		CanChangeJoinMuted:      record.Creator == uid,
		ScheduleStartSubscribed: false,
		CanStartVideo:           record.Creator == uid,
		RecordVideoActive:       settings.RecordActive && settings.RecordVideo,
		ParticipantsCount:       int32(len(all)),
		Creator:                 record.Creator == uid,
		RtmpStream:              record.RtmpStream,
		ScheduleDate: func() *wrapperspb.Int32Value {
			if record.ScheduleDate == nil {
				return nil
			}
			return wrapperspb.Int32(*record.ScheduleDate)
		}(),
		MessagesEnabled:          settings.MessagesEnabled,
		CanChangeMessagesEnabled: record.Creator == uid,
	}
	if subscribed, subErr := domain.LoadGroupCallSubscription(record.ID, uid); subErr != nil {
		return nil, subErr
	} else {
		gc.ScheduleStartSubscribed = subscribed
	}
	if settings.SendPaidMessagesStars != nil {
		gc.SendPaidMessagesStars = wrapperspb.Int64(*settings.SendPaidMessagesStars)
	}
	if sendAs, ok, sendErr := domain.LoadGroupCallSendAs(record.ID, uid); sendErr != nil {
		return nil, sendErr
	} else if ok {
		var input mtproto.InputPeer
		if json.Unmarshal([]byte(sendAs), &input) == nil {
			if input.GetPredicateName() == mtproto.Predicate_inputPeerSelf {
				gc.DefaultSendAs = mtproto.MakePeerUser(uid)
			} else {
				gc.DefaultSendAs = groupCallInputPeer(&input)
			}
		}
	}
	if record.Title != "" {
		gc.Title = wrapperspb.String(record.Title)
	}
	return mtproto.MakeTLPhoneGroupCall(&mtproto.Phone_GroupCall{
		Call:         mtproto.MakeTLGroupCall(gc).To_GroupCall(),
		Participants: parts,
		Chats:        []*mtproto.Chat{},
		Users:        users,
	}).To_Phone_GroupCall(), nil
}

func (c *ApiFullCore) PhoneGetGroupParticipants(in *mtproto.TLPhoneGetGroupParticipants) (*mtproto.Phone_GroupParticipants, error) {
	var call *mtproto.InputGroupCall
	var want []*mtproto.InputPeer
	var sources []int32
	var offset string
	var limit int32
	if in != nil {
		call = in.GetCall()
		want = in.GetIds()
		sources = in.GetSources()
		offset = in.GetOffset()
		limit = in.GetLimit()
	}
	uid, record, all, err := c.loadAuthorizedGroupCall(call)
	if err != nil {
		return nil, err
	}
	matched := filterGroupUserIDs(all, want, sources)
	page, next := pageGroupUserIDs(matched, offset, limit)
	parts, users, err := groupParticipantViews(page, uid, record.ID)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLPhoneGroupParticipants(&mtproto.Phone_GroupParticipants{
		Count:        int32(len(matched)),
		Participants: parts,
		NextOffset:   next,
		Chats:        []*mtproto.Chat{},
		Users:        users,
		Version:      int32(len(all)),
	}).To_Phone_GroupParticipants(), nil
}

func (c *ApiFullCore) PhoneCheckGroupCall(in *mtproto.TLPhoneCheckGroupCall) (*mtproto.Vector_Int, error) {
	var call *mtproto.InputGroupCall
	var sources []int32
	if in != nil {
		call = in.GetCall()
		sources = in.GetSources()
	}
	_, _, ids, err := c.loadAuthorizedGroupCall(call)
	if err != nil {
		return nil, err
	}
	present := make(map[int32]struct{}, len(ids))
	for _, userID := range ids {
		if src, ok := participantSource(userID); ok {
			present[src] = struct{}{}
		}
	}
	out := make([]int32, 0, len(sources))
	for _, s := range sources {
		if _, ok := present[s]; ok {
			out = append(out, s)
		}
	}
	return &mtproto.Vector_Int{Datas: out}, nil
}

func (c *ApiFullCore) PhoneToggleGroupCallRecord(in *mtproto.TLPhoneToggleGroupCallRecord) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	_, record, _, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	settings, err := domain.LoadGroupCallSettings(record.ID)
	if err != nil {
		return nil, err
	}
	changed := settings.RecordActive != in.GetStart() || settings.RecordVideo != in.GetVideo()
	settings.RecordActive = in.GetStart()
	settings.RecordVideo = in.GetVideo()
	if in.GetTitle() != nil {
		value := in.GetTitle().GetValue()
		changed = changed || settings.RecordTitle != value
		settings.RecordTitle = value
	}
	if in.GetVideoPortrait() != nil {
		value := boolValue(in.GetVideoPortrait())
		changed = changed || settings.RecordVideoPortrait != value
		settings.RecordVideoPortrait = value
	}
	if !changed {
		return nil, mtproto.ErrGroupcallNotModified
	}
	// This persists the authoritative recording control state. A media
	// recorder/segment store is still required to produce an actual recording.
	if err = domain.SaveGroupCallSettings(settings); err != nil {
		return nil, err
	}
	return groupCallUpdates(record.ID, record.AccessHash, false), nil
}

func (c *ApiFullCore) PhoneEditGroupCallParticipant(in *mtproto.TLPhoneEditGroupCallParticipant) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	target, err := groupCallPeerUserID(uid, in.GetParticipant())
	if err != nil {
		return nil, err
	}
	if !groupCallHasUser(ids, target) {
		return nil, mtproto.ErrGroupCallParticipantInvalid
	}
	if _, joined, loadErr := domain.LoadGroupCallParticipant(record.ID, target); loadErr != nil {
		return nil, loadErr
	} else if !joined {
		return nil, mtproto.ErrGroupCallParticipantInvalid
	}
	if uid != record.Creator && target != uid {
		return nil, mtproto.ErrChatAdminRequired
	}
	if uid != record.Creator && target == uid && in.GetVolume() != nil {
		return nil, mtproto.ErrChatAdminRequired
	}
	state, _, err := domain.LoadGroupCallParticipant(record.ID, target)
	if err != nil {
		return nil, err
	}
	state.CallID, state.UserID = record.ID, target
	changed := false
	if in.GetMuted() != nil {
		value := boolValue(in.GetMuted())
		changed = changed || state.Muted != value
		state.Muted = value
	}
	if in.GetVolume() != nil {
		if in.GetVolume().GetValue() < 0 {
			return nil, mtproto.ErrGroupCallParticipantInvalid
		}
		value := in.GetVolume().GetValue()
		changed = changed || state.Volume == nil || *state.Volume != value
		state.Volume = &value
	}
	if in.GetRaiseHand() != nil {
		value := boolValue(in.GetRaiseHand())
		changed = changed || state.RaiseHand != value
		state.RaiseHand = value
	}
	if in.GetVideoStopped() != nil {
		value := boolValue(in.GetVideoStopped())
		changed = changed || state.VideoStopped != value
		state.VideoStopped = value
	}
	if in.GetVideoPaused() != nil {
		value := boolValue(in.GetVideoPaused())
		changed = changed || state.VideoPaused != value
		state.VideoPaused = value
	}
	if in.GetPresentationPaused() != nil {
		value := boolValue(in.GetPresentationPaused())
		if value && !state.PresentationActive {
			return nil, mtproto.ErrGroupCallParticipantInvalid
		}
		changed = changed || state.PresentationPaused != value
		state.PresentationPaused = value
	}
	if !changed {
		return nil, mtproto.ErrGroupcallNotModified
	}
	if err = domain.SaveGroupCallParticipant(state); err != nil {
		return nil, err
	}
	return groupCallParticipantUpdates(record, []int64{target}, uid)
}

func (c *ApiFullCore) PhoneEditGroupCallTitle(in *mtproto.TLPhoneEditGroupCallTitle) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	_, record, _, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	updated, err := domain.UpdateGroupCallTitle(record.ID, in.GetTitle())
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, mtproto.ErrGroupCallInvalid
	}
	if err := c.gcallPut("PhoneEditGroupCallTitle", in); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) PhoneGetGroupCallJoinAs(in *mtproto.TLPhoneGetGroupCallJoinAs) (*mtproto.Phone_JoinAsPeers, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	want := in.GetPeer()
	if err = c.validateGroupCallJoinAsPeer(uid, want); err != nil {
		return nil, err
	}
	peers := []*mtproto.Peer{mtproto.MakePeerUser(uid)}
	users := []*mtproto.User{mtproto.MakeTLUser(&mtproto.User{Id: uid}).To_User()}
	chats := []*mtproto.Chat{}
	add := func(p *mtproto.InputPeer) {
		if p == nil || c.validateGroupCallJoinAsPeer(uid, p) != nil {
			return
		}
		peer := groupCallInputPeer(p)
		if peer == nil || peerListed(peers, peer) {
			return
		}
		peers = append(peers, peer)
		switch {
		case p.GetUserId() != 0:
			users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: p.GetUserId()}).To_User())
		case p.GetChatId() != 0:
			chats = append(chats, mtproto.MakeTLChat(&mtproto.Chat{Id: p.GetChatId()}).To_Chat())
		case p.GetChannelId() != 0:
			chats = append(chats, chanChat(p.GetChannelId(), ""))
		}
	}
	raw, err := persist.Default.Get("gcall:" + strconv.FormatInt(uid, 10) + ":PhoneSaveDefaultGroupCallJoinAs")
	if err != nil {
		return nil, err
	}
	if raw != "" && raw != "null" {
		var saved mtproto.TLPhoneSaveDefaultGroupCallJoinAs
		if json.Unmarshal([]byte(raw), &saved) == nil && sameInputPeer(want, saved.GetPeer()) {
			add(saved.GetJoinAs())
		}
	}
	raw, err = persist.Default.Get("gcall:" + strconv.FormatInt(uid, 10) + ":PhoneJoinGroupCall")
	if err != nil {
		return nil, err
	}
	if raw != "" && raw != "null" {
		var joined mtproto.TLPhoneJoinGroupCall
		if json.Unmarshal([]byte(raw), &joined) == nil {
			add(joined.GetJoinAs())
		}
	}
	return mtproto.MakeTLPhoneJoinAsPeers(&mtproto.Phone_JoinAsPeers{
		Peers: peers,
		Chats: chats,
		Users: users,
	}).To_Phone_JoinAsPeers(), nil
}

func (c *ApiFullCore) PhoneExportGroupCallInvite(in *mtproto.TLPhoneExportGroupCallInvite) (*mtproto.Phone_ExportedGroupCallInvite, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, _, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	if err = domain.SaveGroupCallInvite(record.ID, uid, token, in.GetCanSelfUnmute()); err != nil {
		return nil, err
	}
	return mtproto.MakeTLPhoneExportedGroupCallInvite(&mtproto.Phone_ExportedGroupCallInvite{
		Link: fmt.Sprintf("https://t.me/call/%d?invite_hash=%s", record.ID, token),
	}).To_Phone_ExportedGroupCallInvite(), nil
}

func (c *ApiFullCore) PhoneToggleGroupCallStartSubscription(in *mtproto.TLPhoneToggleGroupCallStartSubscription) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, _, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if in.GetSubscribed() == nil {
		return nil, mtproto.ErrGroupCallInvalid
	}
	if err = domain.SaveGroupCallSubscription(record.ID, uid, boolValue(in.GetSubscribed())); err != nil {
		return nil, err
	}
	return groupCallUpdates(record.ID, record.AccessHash, false), nil
}

func (c *ApiFullCore) PhoneStartScheduledGroupCall(in *mtproto.TLPhoneStartScheduledGroupCall) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	_, record, _, err := c.loadCreatorGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if record.ScheduleDate == nil || *record.ScheduleDate <= 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	settings, err := domain.LoadGroupCallSettings(record.ID)
	if err != nil {
		return nil, err
	}
	if settings.ScheduledStarted {
		return nil, mtproto.ErrGroupcallAlreadyStarted
	}
	settings.ScheduledStarted = true
	if err = domain.SaveGroupCallSettings(settings); err != nil {
		return nil, err
	}
	return groupCallUpdates(record.ID, record.AccessHash, false), nil
}

func validDefaultGroupCallPeer(peer *mtproto.InputPeer) bool {
	if peer == nil {
		return false
	}
	userID, chatID, channelID := peer.GetUserId(), peer.GetChatId(), peer.GetChannelId()
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return userID == 0 && chatID == 0 && channelID == 0
	case mtproto.Predicate_inputPeerUser:
		return userID > 0 && chatID == 0 && channelID == 0
	case mtproto.Predicate_inputPeerChat:
		return userID == 0 && chatID > 0 && channelID == 0
	case mtproto.Predicate_inputPeerChannel:
		return userID == 0 && chatID == 0 && channelID > 0
	default:
		return false
	}
}

// validateGroupCallJoinAsPeer verifies that a peer can actually be used by
// this user. Chat peers have no authoritative APIFull roster, so they are
// rejected until one is available instead of being returned as a guess.
func (c *ApiFullCore) validateGroupCallJoinAsPeer(userID int64, peer *mtproto.InputPeer) error {
	if !validDefaultGroupCallPeer(peer) {
		return mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if peer.GetAccessHash() != 0 {
			return mtproto.ErrPeerIdInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() != userID {
			return mtproto.ErrPeerIdInvalid
		}
		if peer.GetAccessHash() == 0 {
			return mtproto.ErrUserIdInvalid
		}
		d := c.apifullDao()
		if d == nil || d.UserClient == nil {
			return mtproto.ErrInternalServerError
		}
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		immutable, err := d.UserClient.UserGetImmutableUserV2(ctx, &userpb.TLUserGetImmutableUserV2{Id: peer.GetUserId()})
		if err != nil {
			return err
		}
		if immutable == nil || immutable.GetUser() == nil || immutable.GetUser().GetId() != peer.GetUserId() || immutable.GetUser().GetDeleted() {
			return mtproto.ErrUserIdInvalid
		}
		if immutable.GetUser().GetAccessHash() != peer.GetAccessHash() {
			return mtproto.ErrUserIdInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerChannel:
		channelID, err := c.groupCallChannelID(userID, peer)
		if err != nil {
			return err
		}
		if channelID == 0 {
			return mtproto.ErrChannelInvalid
		}
		return nil
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

func (c *ApiFullCore) PhoneSaveDefaultGroupCallJoinAs(in *mtproto.TLPhoneSaveDefaultGroupCallJoinAs) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || !validDefaultGroupCallPeer(in.GetPeer()) || !validDefaultGroupCallPeer(in.GetJoinAs()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err = c.validateGroupCallJoinAsPeer(uid, in.GetPeer()); err != nil {
		return nil, err
	}
	if err = c.validateGroupCallJoinAsPeer(uid, in.GetJoinAs()); err != nil {
		return nil, err
	}
	if err := c.gcallPut("PhoneSaveDefaultGroupCallJoinAs", in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PhoneJoinGroupCallPresentation(in *mtproto.TLPhoneJoinGroupCallPresentation) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 || in.GetParams() == nil || in.GetParams().GetData() == "" {
		return nil, mtproto.ErrGroupCallParticipantInvalid
	}
	if !json.Valid([]byte(strings.TrimSpace(in.GetParams().GetData()))) {
		return nil, mtproto.ErrDataJsonInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if !groupCallHasUser(ids, uid) {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	state, joined, err := domain.LoadGroupCallParticipant(record.ID, uid)
	if err != nil {
		return nil, err
	}
	if !joined {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	state.CallID, state.UserID = record.ID, uid
	state.PresentationActive = true
	state.PresentationPaused = false
	state.PresentationParams = in.GetParams().GetData()
	if err = domain.SaveGroupCallParticipant(state); err != nil {
		return nil, err
	}
	return groupCallParticipantUpdates(record, []int64{uid}, uid)
}

func (c *ApiFullCore) PhoneLeaveGroupCallPresentation(in *mtproto.TLPhoneLeaveGroupCallPresentation) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if !groupCallHasUser(ids, uid) {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	state, joined, err := domain.LoadGroupCallParticipant(record.ID, uid)
	if err != nil {
		return nil, err
	}
	if !joined {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	state.CallID, state.UserID = record.ID, uid
	state.PresentationActive = false
	state.PresentationPaused = false
	state.PresentationParams = ""
	if err = domain.SaveGroupCallParticipant(state); err != nil {
		return nil, err
	}
	return groupCallParticipantUpdates(record, []int64{uid}, uid)
}

func (c *ApiFullCore) PhoneGetGroupCallStreamChannels(in *mtproto.TLPhoneGetGroupCallStreamChannels) (*mtproto.Phone_GroupCallStreamChannels, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	// Participant records do not represent a media stream. A media controller
	// must provide active sources and timestamps.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PhoneGetGroupCallStreamRtmpUrl(in *mtproto.TLPhoneGetGroupCallStreamRtmpUrl) (*mtproto.Phone_GroupCallStreamRtmpUrl, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	// There is no relay authorization service to issue or revoke stream keys.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PhoneSendGroupCallMessageB1D11410(in *mtproto.TLPhoneSendGroupCallMessageB1D11410) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	stored, err := c.persistGroupCallMessage(record, uid, ids, in.GetRandomId(), in.GetMessage(), in.GetAllowPaidStars(), in.GetSendAs())
	if err != nil {
		return nil, err
	}
	var text mtproto.TextWithEntities
	if err = json.Unmarshal([]byte(stored.Message), &text); err != nil {
		return nil, err
	}
	return groupCallMessageUpdates(record, stored, &text, stored.SenderID), nil
}

func (c *ApiFullCore) PhoneDeleteGroupCallMessages(in *mtproto.TLPhoneDeleteGroupCallMessages) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, _, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if len(in.GetMessages()) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	if uid != record.Creator {
		for _, id := range in.GetMessages() {
			message, ok, loadErr := domain.LoadGroupCallMessageByID(record.ID, int64(id))
			if loadErr != nil {
				return nil, loadErr
			}
			if ok && message.SenderID != uid {
				return nil, mtproto.ErrMessageDeleteForbidden
			}
		}
	}
	deleted, err := domain.DeleteGroupCallMessages(record.ID, in.GetMessages())
	if err != nil {
		return nil, err
	}
	if len(deleted) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	return groupCallDeleteUpdates(record, deleted), nil
}

func (c *ApiFullCore) PhoneDeleteGroupCallParticipantMessages(in *mtproto.TLPhoneDeleteGroupCallParticipantMessages) (*mtproto.Updates, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, _, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	target, err := groupCallPeerUserID(uid, in.GetParticipant())
	if err != nil {
		return nil, err
	}
	if target != uid && uid != record.Creator {
		return nil, mtproto.ErrMessageDeleteForbidden
	}
	deleted, err := domain.DeleteGroupCallParticipantMessages(record.ID, target)
	if err != nil {
		return nil, err
	}
	if len(deleted) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	return groupCallDeleteUpdates(record, deleted), nil
}

func (c *ApiFullCore) PhoneGetGroupCallStars(in *mtproto.TLPhoneGetGroupCallStars) (*mtproto.Phone_GroupCallStars, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	// Group membership is not a donation ledger and cannot identify donors.
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) PhoneSaveDefaultSendAs(in *mtproto.TLPhoneSaveDefaultSendAs) (*mtproto.Bool, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if in.GetSendAs() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err = c.validateGroupCallSendAs(uid, ids, in.GetSendAs()); err != nil {
		return nil, err
	}
	blob, err := json.Marshal(in.GetSendAs())
	if err != nil {
		return nil, err
	}
	if err = domain.SaveGroupCallSendAs(record.ID, uid, string(blob)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PhoneSendGroupCallMessage87893014(in *mtproto.TLPhoneSendGroupCallMessage87893014) (*mtproto.Bool, error) {
	if in == nil || in.GetCall() == nil || in.GetCall().GetId() == 0 {
		return nil, mtproto.ErrGroupCallInvalid
	}
	uid, record, ids, err := c.loadAuthorizedGroupCall(in.GetCall())
	if err != nil {
		return nil, err
	}
	if _, err = c.persistGroupCallMessage(record, uid, ids, in.GetRandomId(), in.GetMessage(), nil, nil); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
