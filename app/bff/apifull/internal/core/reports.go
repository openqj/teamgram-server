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
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// RPCReportsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

// reportTarget is the canonical target stored with a moderation intake record.
// The request payload keeps the complete protocol object, while the target
// columns allow workers to partition and deduplicate work without decoding TL.
type reportTarget struct {
	typ string
	id  int64
}

func reportPeerTarget(actorID int64, peer *mtproto.InputPeer) (reportTarget, error) {
	if peer == nil {
		return reportTarget{}, mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if peer.GetUserId() != 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 {
			return reportTarget{}, mtproto.ErrPeerIdInvalid
		}
		return reportTarget{typ: "user", id: actorID}, nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 {
			return reportTarget{}, mtproto.ErrPeerIdInvalid
		}
		return reportTarget{typ: "user", id: peer.GetUserId()}, nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 || peer.GetUserId() != 0 || peer.GetChannelId() != 0 {
			return reportTarget{}, mtproto.ErrPeerIdInvalid
		}
		return reportTarget{typ: "chat", id: peer.GetChatId()}, nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 || peer.GetUserId() != 0 || peer.GetChatId() != 0 {
			return reportTarget{}, mtproto.ErrPeerIdInvalid
		}
		return reportTarget{typ: "channel", id: peer.GetChannelId()}, nil
	case "":
		// Unit callers sometimes construct the generated value directly instead
		// of using a TL constructor. Infer only an unambiguous legacy shape.
		switch {
		case peer.GetUserId() > 0 && peer.GetChatId() == 0 && peer.GetChannelId() == 0:
			return reportTarget{typ: "user", id: peer.GetUserId()}, nil
		case peer.GetChatId() > 0 && peer.GetUserId() == 0 && peer.GetChannelId() == 0:
			return reportTarget{typ: "chat", id: peer.GetChatId()}, nil
		case peer.GetChannelId() > 0 && peer.GetUserId() == 0 && peer.GetChatId() == 0:
			return reportTarget{typ: "channel", id: peer.GetChannelId()}, nil
		}
	}
	return reportTarget{}, mtproto.ErrPeerIdInvalid
}

func reportChannelTarget(channel *mtproto.InputChannel) (reportTarget, error) {
	if channel == nil || channel.GetChannelId() <= 0 {
		return reportTarget{}, mtproto.ErrChannelInvalid
	}
	if name := channel.GetPredicateName(); name != "" && name != mtproto.Predicate_inputChannel {
		return reportTarget{}, mtproto.ErrChannelInvalid
	}
	return reportTarget{typ: "channel", id: channel.GetChannelId()}, nil
}

func reportPhotoID(photo *mtproto.InputPhoto) error {
	if photo == nil || photo.GetId() <= 0 {
		return mtproto.ErrPhotoIdInvalid
	}
	if name := photo.GetPredicateName(); name != "" && name != mtproto.Predicate_inputPhoto {
		return mtproto.ErrPhotoIdInvalid
	}
	return nil
}

func reportDocumentID(document *mtproto.InputDocument) error {
	if document == nil || document.GetId() <= 0 {
		return mtproto.ErrDocumentInvalid
	}
	if name := document.GetPredicateName(); name != "" && name != mtproto.Predicate_inputDocument {
		return mtproto.ErrDocumentInvalid
	}
	return nil
}

func reportEncryptedChatID(chat *mtproto.InputEncryptedChat) (reportTarget, error) {
	if chat == nil || chat.GetChatId() <= 0 {
		return reportTarget{}, mtproto.ErrEncryptedChatIdInvalid
	}
	if name := chat.GetPredicateName(); name != "" && name != mtproto.Predicate_inputEncryptedChat {
		return reportTarget{}, mtproto.ErrEncryptedChatIdInvalid
	}
	return reportTarget{typ: "encrypted_chat", id: int64(chat.GetChatId())}, nil
}

func reportReasonValid(reason *mtproto.ReportReason) bool {
	if reason == nil {
		return false
	}
	switch reason.GetPredicateName() {
	case mtproto.Predicate_inputReportReasonSpam,
		mtproto.Predicate_inputReportReasonViolence,
		mtproto.Predicate_inputReportReasonPornography,
		mtproto.Predicate_inputReportReasonChildAbuse,
		mtproto.Predicate_inputReportReasonOther,
		mtproto.Predicate_inputReportReasonCopyright,
		mtproto.Predicate_inputReportReasonGeoIrrelevant,
		mtproto.Predicate_inputReportReasonFake,
		mtproto.Predicate_inputReportReasonIllegalDrugs,
		mtproto.Predicate_inputReportReasonPersonalDetails:
		return true
	default:
		return false
	}
}

func reportMessageIDs(ids []int32) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func reportDedupeKey(actorID int64, kind string, target reportTarget, payload []byte) string {
	h := sha256.New()
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(actorID))
	_, _ = h.Write(n[:])
	for _, part := range []string{kind, target.typ} {
		binary.BigEndian.PutUint64(n[:], uint64(len(part)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(part))
	}
	binary.BigEndian.PutUint64(n[:], uint64(target.id))
	_, _ = h.Write(n[:])
	binary.BigEndian.PutUint64(n[:], uint64(len(payload)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func (c *ApiFullCore) recordReport(actorID int64, kind string, target reportTarget, in proto.Message) error {
	// Keep the old behavior on deployments without the moderation intake DB.
	// A successful protocol response is only emitted after durable insertion.
	if !domain.Ready() {
		return mtproto.ErrMethodNotImpl
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal report request: %w", err)
	}
	if err := domain.SaveReport(actorID, kind, target.typ, target.id, reportDedupeKey(actorID, kind, target, payload), payload); err != nil {
		return err
	}
	return nil
}

func reportAccepted() *mtproto.Bool {
	return mtproto.MakeTLBoolTrue(nil).To_Bool()
}

func (c *ApiFullCore) AccountReportPeer(in *mtproto.TLAccountReportPeer) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || !reportReasonValid(in.GetReason()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "account.reportPeer", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) AccountReportProfilePhoto(in *mtproto.TLAccountReportProfilePhoto) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || !reportReasonValid(in.GetReason()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = reportPhotoID(in.GetPhotoId()); err != nil {
		return nil, err
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "account.reportProfilePhoto", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) MessagesReportSpam(in *mtproto.TLMessagesReportSpam) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "messages.reportSpam", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) MessagesReportFC78AF9B(in *mtproto.TLMessagesReportFC78AF9B) (*mtproto.ReportResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !reportMessageIDs(in.GetId()) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "messages.report", target, in); err != nil {
		return nil, err
	}
	return mtproto.MakeTLReportResultReported(nil).To_ReportResult(), nil
}

func (c *ApiFullCore) MessagesReportEncryptedSpam(in *mtproto.TLMessagesReportEncryptedSpam) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportEncryptedChatID(in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "messages.reportEncryptedSpam", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) MessagesReportReadMetrics(in *mtproto.TLMessagesReportReadMetrics) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || len(in.GetMetrics()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	for _, metric := range in.GetMetrics() {
		if metric == nil || metric.GetMsgId() <= 0 || metric.GetViewId() < 0 || metric.GetTimeInViewMs() < 0 || metric.GetActiveTimeInViewMs() < 0 || metric.GetHeightToViewportRatioPermille() < 0 || metric.GetSeenRangeRatioPermille() < 0 {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "messages.reportReadMetrics", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) MessagesReportMusicListen(in *mtproto.TLMessagesReportMusicListen) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetListenedDuration() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = reportDocumentID(in.GetId()); err != nil {
		return nil, err
	}
	target := reportTarget{typ: "document", id: in.GetId().GetId()}
	if err = c.recordReport(uid, "messages.reportMusicListen", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) ChannelsReportSpam(in *mtproto.TLChannelsReportSpam) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || !reportMessageIDs(in.GetId()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportChannelTarget(in.GetChannel())
	if err != nil {
		return nil, err
	}
	if _, err = reportPeerTarget(uid, in.GetParticipant()); err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "channels.reportSpam", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}

func (c *ApiFullCore) MessagesReport8953AB4E(in *mtproto.TLMessagesReport8953AB4E) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !domain.Ready() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || !reportMessageIDs(in.GetId()) || !reportReasonValid(in.GetReason()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target, err := reportPeerTarget(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = c.recordReport(uid, "messages.report8953AB4E", target, in); err != nil {
		return nil, err
	}
	return reportAccepted(), nil
}
