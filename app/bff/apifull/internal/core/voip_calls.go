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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCVoipCallsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func callKey(userID int64, op string) string {
	return "call:" + strconv.FormatInt(userID, 10) + ":" + op
}

func newCallID() int64 {
	id := time.Now().UnixNano()
	if id <= 0 {
		return 1
	}
	return id
}

func saveCall(userID int64, op string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set(callKey(userID, op), string(raw))
}

func phoneCallBox() *mtproto.Phone_PhoneCall {
	return mtproto.MakeTLPhonePhoneCall(&mtproto.Phone_PhoneCall{
		PhoneCall: mtproto.MakeTLPhoneCallEmpty(&mtproto.PhoneCall{}).To_PhoneCall(),
		Users:     []*mtproto.User{},
	}).To_Phone_PhoneCall()
}

func callMaterialKey(id int64, part string) string {
	return fmt.Sprintf("callmat:%d:%s", id, part)
}

func loadCallMaterial(id int64, part string) ([]byte, error) {
	raw, err := persist.Default.Get(callMaterialKey(id, part))
	if err != nil || raw == "" {
		return nil, err
	}
	return hex.DecodeString(raw)
}

func callProtocol(p *mtproto.PhoneCallProtocol) *mtproto.PhoneCallProtocol {
	if p == nil {
		p = &mtproto.PhoneCallProtocol{
			UdpP2P:          true,
			UdpReflector:    true,
			MinLayer:        65,
			MaxLayer:        92,
			LibraryVersions: []string{"4.0.0"},
		}
	} else if p.LibraryVersions == nil {
		p.LibraryVersions = []string{}
	}
	return mtproto.MakeTLPhoneCallProtocol(p).To_PhoneCallProtocol()
}

func phoneCallRequested(call domain.Call, protocol *mtproto.PhoneCallProtocol) *mtproto.Phone_PhoneCall {
	return mtproto.MakeTLPhonePhoneCall(&mtproto.Phone_PhoneCall{
		PhoneCall: mtproto.MakeTLPhoneCallRequested(&mtproto.PhoneCall{
			Id:            call.ID,
			AccessHash:    call.AccessHash,
			AdminId:       call.AdminID,
			ParticipantId: call.ParticipantID,
			Video:         call.Video,
			Date:          int32(call.CreatedAt),
			GAHash:        call.GAHash,
			Protocol:      callProtocol(protocol),
		}).To_PhoneCall(),
		Users: []*mtproto.User{},
	}).To_Phone_PhoneCall()
}

func phoneCallConnectedRecord(call domain.Call, protocol *mtproto.PhoneCallProtocol, viewerID int64) *mtproto.Phone_PhoneCall {
	conn := mtproto.MakeTLPhoneConnectionWebrtc(&mtproto.PhoneConnection{
		Id:       1,
		Ip:       domain.Relay.IP,
		Ipv6:     "",
		Port:     domain.Relay.Port,
		Username: domain.Relay.Username,
		Password: domain.Relay.Password,
		Turn:     true,
		Stun:     true,
	}).To_PhoneConnection()
	peerPublicValue := call.GA
	if viewerID == call.AdminID {
		peerPublicValue = call.GB
	}
	return mtproto.MakeTLPhonePhoneCall(&mtproto.Phone_PhoneCall{
		PhoneCall: mtproto.MakeTLPhoneCall(&mtproto.PhoneCall{
			Id:             call.ID,
			AccessHash:     call.AccessHash,
			AdminId:        call.AdminID,
			ParticipantId:  call.ParticipantID,
			Video:          call.Video,
			Date:           int32(call.CreatedAt),
			Protocol:       callProtocol(protocol),
			Connections:    []*mtproto.PhoneConnection{conn},
			GB:             call.GB,
			GAOrB:          append([]byte(nil), peerPublicValue...),
			KeyFingerprint: call.KeyFingerprint,
			P2PAllowed:     true,
		}).To_PhoneCall(),
		Users: []*mtproto.User{},
	}).To_Phone_PhoneCall()
}

func phoneCallAcceptedRecord(call domain.Call, protocol *mtproto.PhoneCallProtocol) *mtproto.Phone_PhoneCall {
	return mtproto.MakeTLPhonePhoneCall(&mtproto.Phone_PhoneCall{
		PhoneCall: mtproto.MakeTLPhoneCallAccepted(&mtproto.PhoneCall{
			Id:            call.ID,
			AccessHash:    call.AccessHash,
			AdminId:       call.AdminID,
			ParticipantId: call.ParticipantID,
			Video:         call.Video,
			Date:          int32(call.CreatedAt),
			GB:            call.GB,
			Protocol:      callProtocol(protocol),
		}).To_PhoneCall(),
		Users: []*mtproto.User{},
	}).To_Phone_PhoneCall()
}

func phoneCallDiscarded(call domain.Call, reason *mtproto.PhoneCallDiscardReason) *mtproto.PhoneCall {
	return mtproto.MakeTLPhoneCallDiscarded(&mtproto.PhoneCall{
		Id:            call.ID,
		AccessHash:    call.AccessHash,
		AdminId:       call.AdminID,
		ParticipantId: call.ParticipantID,
		Video:         call.Video,
		Date:          int32(call.CreatedAt),
		Reason:        reason,
		Duration:      mtproto.MakeFlagsInt32(call.Duration),
	}).To_PhoneCall()
}

func callContext(c *ApiFullCore) context.Context {
	if c != nil && c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func (c *ApiFullCore) resolveCallParticipant(uid int64, input *mtproto.InputUser) (int64, int64, error) {
	if input == nil || input.GetUserId() == 0 || input.GetUserId() == uid || input.GetAccessHash() == 0 {
		return 0, 0, mtproto.ErrUserIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return 0, 0, mtproto.ErrInternalServerError
	}
	target, err := d.UserClient.UserGetImmutableUserV2(callContext(c), &userpb.TLUserGetImmutableUserV2{Id: input.GetUserId()})
	if err != nil {
		return 0, 0, err
	}
	if target == nil || target.GetUser() == nil || target.GetUser().GetId() != input.GetUserId() || target.Deleted() {
		return 0, 0, mtproto.ErrUserIdInvalid
	}
	if target.IsBot() {
		return 0, 0, mtproto.ErrUserIsBot
	}
	if target.AccessHash() != input.GetAccessHash() {
		return 0, 0, mtproto.ErrUserIdInvalid
	}
	return input.GetUserId(), input.GetAccessHash(), nil
}

func callPeer(in *mtproto.InputPhoneCall) (int64, int64, error) {
	if in == nil || in.GetId() <= 0 || in.GetAccessHash() == 0 {
		return 0, 0, mtproto.ErrCallPeerInvalid
	}
	return in.GetId(), in.GetAccessHash(), nil
}

func mapCallError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrCallNotFound), errors.Is(err, domain.ErrCallForbidden):
		return mtproto.ErrCallPeerInvalid
	case errors.Is(err, domain.ErrCallProtocol):
		return mtproto.ErrCallProtocolFlagsInvalid
	case errors.Is(err, domain.ErrCallInvalidState):
		return mtproto.ErrCallAlreadyDeclined
	default:
		return err
	}
}

func protocolJSON(protocol *mtproto.PhoneCallProtocol) (string, *mtproto.PhoneCallProtocol, error) {
	protocol = callProtocol(protocol)
	raw, err := json.Marshal(protocol)
	return string(raw), protocol, err
}

func (c *ApiFullCore) pushCallUpdate(userID int64, call *mtproto.PhoneCall) error {
	if userID == 0 || call == nil {
		return mtproto.ErrInternalServerError
	}
	d := c.apifullDao()
	if d == nil || d.SyncClient == nil {
		return mtproto.ErrInternalServerError
	}
	_, err := d.SyncClient.SyncPushUpdates(callContext(c), &sync.TLSyncPushUpdates{
		UserId:  userID,
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{PhoneCall: call}).To_Update()),
	})
	return err
}

func (c *ApiFullCore) pushCallSignaling(userID, callID int64, data []byte) error {
	if userID == 0 || callID == 0 || len(data) == 0 {
		return mtproto.ErrInternalServerError
	}
	d := c.apifullDao()
	if d == nil || d.SyncClient == nil {
		return mtproto.ErrInternalServerError
	}
	_, err := d.SyncClient.SyncPushUpdates(callContext(c), &sync.TLSyncPushUpdates{
		UserId: userID,
		Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePhoneCallSignalingData(&mtproto.Update{
			PhoneCallId:    callID,
			Data_FLAGBYTES: append([]byte(nil), data...),
		}).To_Update()),
	})
	return err
}

func callUpdates() *mtproto.Updates {
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}).To_Updates()
}

func (c *ApiFullCore) MessagesDeletePhoneCallHistory(in *mtproto.TLMessagesDeletePhoneCallHistory) (*mtproto.Messages_AffectedFoundMessages, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	d := c.apifullDao()
	if d == nil || d.PhoneCallHistoryMutator == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	result, err := d.PhoneCallHistoryMutator.MsgDeletePhoneCallHistory(callContext(c), &msgpb.TLMsgDeletePhoneCallHistory{
		UserId:    uid,
		AuthKeyId: c.MD.GetPermAuthKeyId(),
		Revoke:    in.GetRevoke(),
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) PhoneGetCallConfig(in *mtproto.TLPhoneGetCallConfig) (*mtproto.DataJSON, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	_ = in
	type turnConfig struct {
		IP       string `json:"ip"`
		Port     int32  `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		PeerTag  []byte `json:"peer_tag"`
	}
	payload := struct {
		AudioFrameSize        int        `json:"audio_frame_size"`
		JitterMinDelay60      int        `json:"jitter_min_delay_60"`
		JitterMaxDelay60      int        `json:"jitter_max_delay_60"`
		VideoCongestionWindow int        `json:"video_congestion_window"`
		Turn                  turnConfig `json:"turn"`
	}{
		AudioFrameSize:        60,
		JitterMinDelay60:      2,
		JitterMaxDelay60:      10,
		VideoCongestionWindow: 1024,
		Turn: turnConfig{
			IP:       domain.Relay.IP,
			Port:     domain.Relay.Port,
			Username: domain.Relay.Username,
			Password: domain.Relay.Password,
			PeerTag:  domain.Relay.PeerTag,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(raw)}).To_DataJSON(), nil
}

func (c *ApiFullCore) PhoneRequestCall(in *mtproto.TLPhoneRequestCall) (*mtproto.Phone_PhoneCall, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetRandomId() == 0 || len(in.GetGAHash()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	participantID, _, err := c.resolveCallParticipant(uid, in.GetUserId())
	if err != nil {
		return nil, err
	}
	protocolRaw, protocol, err := protocolJSON(in.GetProtocol())
	if err != nil {
		return nil, err
	}
	id := newCallID()
	record := domain.Call{
		ID: id, AccessHash: id, AdminID: uid, ParticipantID: participantID,
		State: "requested", Video: in.GetVideo(), GAHash: append([]byte(nil), in.GetGAHash()...),
		Protocol: protocolRaw, CreatedAt: time.Now().Unix(),
	}
	if err = domain.SaveCall(record); err != nil {
		return nil, err
	}
	if err = c.pushCallUpdate(participantID, phoneCallRequested(record, protocol).GetPhoneCall()); err != nil {
		return nil, err
	}
	return phoneCallRequested(record, protocol), nil
}

func (c *ApiFullCore) PhoneAcceptCall(in *mtproto.TLPhoneAcceptCall) (*mtproto.Phone_PhoneCall, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	if len(in.GetGB()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	protocolRaw, protocol, err := protocolJSON(in.GetProtocol())
	if err != nil {
		return nil, err
	}
	record, err := domain.TransitionCall(id, domain.CallTransition{
		ActorID: uid, AccessHash: accessHash, FromStates: []string{"requested", "received"}, ToState: "accepted",
		GB: append([]byte(nil), in.GetGB()...), Protocol: protocolRaw, AcceptedAt: time.Now().Unix(),
	})
	if err != nil {
		return nil, mapCallError(err)
	}
	if err = c.pushCallUpdate(record.AdminID, phoneCallAcceptedRecord(record, protocol).GetPhoneCall()); err != nil {
		return nil, err
	}
	return phoneCallAcceptedRecord(record, protocol), nil
}

func (c *ApiFullCore) PhoneConfirmCall(in *mtproto.TLPhoneConfirmCall) (*mtproto.Phone_PhoneCall, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	if len(in.GetGA()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	protocolRaw, protocol, err := protocolJSON(in.GetProtocol())
	if err != nil {
		return nil, err
	}
	record, err := domain.TransitionCall(id, domain.CallTransition{
		ActorID: uid, AccessHash: accessHash, FromStates: []string{"accepted"}, ToState: "confirmed",
		GA: append([]byte(nil), in.GetGA()...), Protocol: protocolRaw, KeyFingerprint: in.GetKeyFingerprint(),
		ConfirmedAt: time.Now().Unix(),
	})
	if err != nil {
		return nil, mapCallError(err)
	}
	if err = c.pushCallUpdate(record.ParticipantID, phoneCallConnectedRecord(record, protocol, record.ParticipantID).GetPhoneCall()); err != nil {
		return nil, err
	}
	return phoneCallConnectedRecord(record, protocol, record.AdminID), nil
}

func (c *ApiFullCore) PhoneReceivedCall(in *mtproto.TLPhoneReceivedCall) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	if _, err = domain.TransitionCall(id, domain.CallTransition{
		ActorID: uid, AccessHash: accessHash, FromStates: []string{"requested"}, ToState: "received", ReceivedAt: time.Now().Unix(),
	}); err != nil {
		return nil, mapCallError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PhoneDiscardCall(in *mtproto.TLPhoneDiscardCall) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	reason := in.GetReason()
	reasonName := ""
	if reason != nil {
		reasonName = reason.GetPredicateName()
	}
	record, err := domain.TransitionCall(id, domain.CallTransition{
		ActorID: uid, AccessHash: accessHash,
		FromStates: []string{"requested", "received", "accepted", "confirmed"}, ToState: "discarded",
		DiscardedAt: time.Now().Unix(), DiscardedBy: uid, Duration: in.GetDuration(), Reason: reasonName,
	})
	if err != nil {
		return nil, mapCallError(err)
	}
	other := record.AdminID
	if uid == record.AdminID {
		other = record.ParticipantID
	}
	call := phoneCallDiscarded(record, reason)
	if err = c.pushCallUpdate(other, call); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{PhoneCall: call}).To_Update(),
		},
		Users: []*mtproto.User{},
		Chats: []*mtproto.Chat{},
	}).To_Updates(), nil
}

func (c *ApiFullCore) PhoneSetCallRating(in *mtproto.TLPhoneSetCallRating) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetRating() < 1 || in.GetRating() > 5 || len([]rune(in.GetComment())) > 512 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		UserInitiative bool   `json:"user_initiative"`
		Rating         int32  `json:"rating"`
		Comment        string `json:"comment,omitempty"`
	}{in.GetUserInitiative(), in.GetRating(), in.GetComment()})
	if err != nil {
		return nil, err
	}
	if _, err = domain.SaveCallArtifact(id, accessHash, uid, "rating", payload); err != nil {
		return nil, mapCallError(err)
	}
	return callUpdates(), nil
}

func (c *ApiFullCore) PhoneSaveCallDebug(in *mtproto.TLPhoneSaveCallDebug) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetDebug() == nil || len(in.GetDebug().GetData()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	if len(in.GetDebug().GetData()) > 1<<20 {
		return nil, mtproto.ErrDataTooLong
	}
	if _, err = domain.SaveCallArtifact(id, accessHash, uid, "debug", []byte(in.GetDebug().GetData())); err != nil {
		return nil, mapCallError(err)
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PhoneSendSignalingData(in *mtproto.TLPhoneSendSignalingData) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || len(in.GetData()) == 0 || len(in.GetData()) > 1<<20 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	call, err := domain.SaveCallArtifact(id, accessHash, uid, "signaling", in.GetData())
	if err != nil {
		return nil, mapCallError(err)
	}
	other := call.AdminID
	if uid == call.AdminID {
		other = call.ParticipantID
	}
	if err = c.pushCallSignaling(other, call.ID, in.GetData()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) PhoneSaveCallLog(in *mtproto.TLPhoneSaveCallLog) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetFile() == nil || in.GetFile().GetId_INT64() == 0 || in.GetFile().GetParts() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	id, accessHash, err := callPeer(in.GetPeer())
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		ID    int64  `json:"id"`
		Parts int32  `json:"parts"`
		Name  string `json:"name,omitempty"`
		MD5   string `json:"md5,omitempty"`
	}{in.GetFile().GetId_INT64(), in.GetFile().GetParts(), in.GetFile().GetName(), in.GetFile().GetMd5Checksum()})
	if err != nil {
		return nil, err
	}
	if _, err = domain.SaveCallArtifact(id, accessHash, uid, "log", payload); err != nil {
		return nil, mapCallError(err)
	}
	return mtproto.BoolTrue, nil
}
