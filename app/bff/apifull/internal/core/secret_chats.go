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
	"encoding/binary"
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

const maxSecretMessageBytes = 1 << 20

func newSecretAccessHash() (int64, error) {
	var raw [8]byte
	for {
		if _, err := rand.Read(raw[:]); err != nil {
			return 0, err
		}
		if accessHash := int64(binary.BigEndian.Uint64(raw[:])); accessHash != 0 {
			return accessHash, nil
		}
	}
}

func secretChatWaiting(chat domain.SecretChat) *mtproto.EncryptedChat {
	return mtproto.MakeTLEncryptedChatWaiting(&mtproto.EncryptedChat{
		Id:            chat.ID,
		AccessHash:    chat.AccessHash,
		Date:          chat.CreatedAt,
		AdminId:       chat.AdminID,
		ParticipantId: chat.ParticipantID,
	}).To_EncryptedChat()
}

func secretChatRequested(chat domain.SecretChat) *mtproto.EncryptedChat {
	return mtproto.MakeTLEncryptedChatRequested(&mtproto.EncryptedChat{
		Id:            chat.ID,
		AccessHash:    chat.AccessHash,
		Date:          chat.CreatedAt,
		AdminId:       chat.AdminID,
		ParticipantId: chat.ParticipantID,
		GA:            chat.GA,
	}).To_EncryptedChat()
}

func secretChatActive(chat domain.SecretChat, viewerID int64) *mtproto.EncryptedChat {
	peerPublicValue := chat.GA
	if viewerID == chat.AdminID {
		peerPublicValue = chat.GB
	}
	return mtproto.MakeTLEncryptedChat(&mtproto.EncryptedChat{
		Id:             chat.ID,
		AccessHash:     chat.AccessHash,
		Date:           chat.CreatedAt,
		AdminId:        chat.AdminID,
		ParticipantId:  chat.ParticipantID,
		GAOrB:          peerPublicValue,
		KeyFingerprint: chat.KeyFingerprint,
	}).To_EncryptedChat()
}

func secretChatDiscarded(chat domain.SecretChat) *mtproto.EncryptedChat {
	return mtproto.MakeTLEncryptedChatDiscarded(&mtproto.EncryptedChat{
		Id:             chat.ID,
		HistoryDeleted: chat.HistoryDeleted,
	}).To_EncryptedChat()
}

func secretEncryptedFile(file *domain.SecretFile) *mtproto.EncryptedFile {
	if file == nil {
		return mtproto.MakeTLEncryptedFileEmpty(nil).To_EncryptedFile()
	}
	return mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
		Id:             file.ID,
		AccessHash:     file.AccessHash,
		Size2_INT32:    int32(file.Size),
		Size2_INT64:    file.Size,
		DcId:           file.DCID,
		KeyFingerprint: file.KeyFingerprint,
	}).To_EncryptedFile()
}

func secretEncryptedMessage(message domain.SecretMessage) *mtproto.EncryptedMessage {
	data := &mtproto.EncryptedMessage{
		RandomId: message.RandomID,
		ChatId:   message.ChatID,
		Date:     message.Date,
		Bytes:    message.Data,
		File:     secretEncryptedFile(message.File),
	}
	if message.Service {
		return mtproto.MakeTLEncryptedMessageService(data).To_EncryptedMessage()
	}
	return mtproto.MakeTLEncryptedMessage(data).To_EncryptedMessage()
}

func (c *ApiFullCore) secretContext() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

// secretDeviceID binds a secret-chat key to the authenticated MTProto device,
// rather than to the user alone. Permanent auth keys are stable across
// sessions; the shorter-lived auth/session ids are fallbacks for test and
// migration traffic that has not populated PermAuthKeyId yet.
func (c *ApiFullCore) secretDeviceID() (int64, error) {
	if c == nil || c.MD == nil {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	for _, id := range []int64{c.MD.GetPermAuthKeyId(), c.MD.GetAuthId(), c.MD.GetSessionId()} {
		// Auth key ids are signed int64 values on the wire and production keys
		// commonly have the high bit set. Only zero means that the field is absent.
		if id != 0 {
			return id, nil
		}
	}
	return 0, mtproto.ErrAuthKeyUnregistered
}

func (c *ApiFullCore) resolveSecretParticipant(callerID int64, input *mtproto.InputUser) (int64, error) {
	if input == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	if c.MD != nil && c.MD.GetIsBot() {
		return 0, mtproto.ErrBotMethodInvalid
	}
	peer := mtproto.FromInputUser(callerID, input)
	if peer == nil || peer.PeerType != mtproto.PEER_USER || peer.PeerId <= 0 || peer.PeerId == callerID || peer.AccessHash == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return 0, mtproto.ErrInternalServerError
	}
	target, err := d.UserClient.UserGetImmutableUserV2(c.secretContext(), &user.TLUserGetImmutableUserV2{Id: peer.PeerId})
	if err != nil {
		return 0, err
	}
	if target == nil || target.GetUser() == nil || target.GetUser().GetId() != peer.PeerId || target.Deleted() {
		return 0, mtproto.ErrUserIdInvalid
	}
	if target.IsBot() {
		return 0, mtproto.ErrUserIsBot
	}
	if target.AccessHash() != peer.AccessHash {
		return 0, mtproto.ErrUserIdInvalid
	}
	return peer.PeerId, nil
}

func (c *ApiFullCore) pushSecretUpdate(userID int64, update *mtproto.Update) error {
	d := c.apifullDao()
	if d == nil || d.SyncClient == nil || userID <= 0 || update == nil {
		return mtproto.ErrInternalServerError
	}
	result, err := d.SyncClient.SyncPushUpdates(c.secretContext(), &sync.TLSyncPushUpdates{
		UserId:  userID,
		Updates: mtproto.MakeUpdatesByUpdates(update),
	})
	if err != nil {
		return err
	}
	if result == nil {
		return mtproto.ErrInternalServerError
	}
	return nil
}

func (c *ApiFullCore) pushSecretEncryption(userID int64, chat *mtproto.EncryptedChat, date int32) error {
	return c.pushSecretUpdate(userID, mtproto.MakeTLUpdateEncryption(&mtproto.Update{
		Chat:       chat,
		Date_INT32: date,
	}).To_Update())
}

func mapSecretError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrSecretChatNotFound), errors.Is(err, domain.ErrSecretChatForbidden):
		return mtproto.ErrEncryptionIdInvalid
	case errors.Is(err, domain.ErrSecretChatAlreadyAccepted):
		return mtproto.ErrEncryptionAlreadyAccepted
	case errors.Is(err, domain.ErrSecretKeyReplay):
		return mtproto.ErrEncryptionAlreadyAccepted
	case errors.Is(err, domain.ErrSecretChatDeclined):
		return mtproto.ErrEncryptionDeclined
	case errors.Is(err, domain.ErrSecretMessageConflict):
		return mtproto.ErrRandomIdDuplicate
	case errors.Is(err, domain.ErrSecretQTSInvalid):
		return mtproto.ErrMaxQtsInvalid
	case errors.Is(err, domain.ErrSecretChatConflict):
		return mtproto.ErrEncryptionDeclined
	default:
		return err
	}
}

func (c *ApiFullCore) MessagesRequestEncryption(in *mtproto.TLMessagesRequestEncryption) (*mtproto.EncryptedChat, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetRandomId() == 0 || len(in.GetGA()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !dhPublicOK(in.GetGA()) {
		return nil, mtproto.ErrDhGAInvalid
	}
	participantID, err := c.resolveSecretParticipant(uid, in.GetUserId())
	if err != nil {
		return nil, err
	}
	accessHash, err := newSecretAccessHash()
	if err != nil {
		return nil, err
	}
	chat, _, err := domain.CreateSecretChat(domain.SecretChat{
		ID:            in.GetRandomId(),
		AccessHash:    accessHash,
		AdminID:       uid,
		ParticipantID: participantID,
		GA:            append([]byte(nil), in.GetGA()...),
	})
	if err != nil {
		if errors.Is(err, domain.ErrSecretChatConflict) {
			return nil, mtproto.ErrRandomIdDuplicate
		}
		return nil, mapSecretError(err)
	}
	deviceID, err := c.secretDeviceID()
	if err != nil {
		return nil, err
	}
	if err = domain.SaveSecretDeviceKey(domain.SecretDeviceKey{
		ChatID: chat.ID, UserID: uid, DeviceID: deviceID, Epoch: 1,
		PublicKey: append([]byte(nil), chat.GA...),
	}); err != nil {
		return nil, mapSecretError(err)
	}
	if err = c.pushSecretEncryption(uid, secretChatWaiting(chat), chat.CreatedAt); err != nil {
		return nil, err
	}
	if err = c.pushSecretEncryption(participantID, secretChatRequested(chat), chat.CreatedAt); err != nil {
		return nil, err
	}
	return secretChatWaiting(chat), nil
}

func (c *ApiFullCore) MessagesAcceptEncryption(in *mtproto.TLMessagesAcceptEncryption) (*mtproto.EncryptedChat, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetChatId() == 0 || in.GetPeer().GetAccessHash() == 0 || len(in.GetGB()) == 0 || in.GetKeyFingerprint() == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !dhPublicOK(in.GetGB()) {
		return nil, mtproto.ErrDhGAInvalid
	}
	deviceID, err := c.secretDeviceID()
	if err != nil {
		return nil, err
	}
	chat, _, err := domain.AcceptSecretChatOnDevice(in.GetPeer().GetChatId(), in.GetPeer().GetAccessHash(), uid, deviceID, 1, append([]byte(nil), in.GetGB()...), in.GetKeyFingerprint())
	if err != nil {
		return nil, mapSecretError(err)
	}
	date := chat.AcceptedAt
	if date == 0 {
		date = int32(time.Now().Unix())
	}
	if err = c.pushSecretEncryption(chat.AdminID, secretChatActive(chat, chat.AdminID), date); err != nil {
		return nil, err
	}
	if err = c.pushSecretEncryption(chat.ParticipantID, secretChatActive(chat, chat.ParticipantID), date); err != nil {
		return nil, err
	}
	return secretChatActive(chat, uid), nil
}

func (c *ApiFullCore) MessagesDiscardEncryption(in *mtproto.TLMessagesDiscardEncryption) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChatId() == 0 {
		return nil, mtproto.ErrEncryptionIdInvalid
	}
	chat, _, err := domain.DiscardSecretChat(in.GetChatId(), uid, in.GetDeleteHistory())
	if err != nil {
		return nil, mapSecretError(err)
	}
	date := chat.DiscardedAt
	if date == 0 {
		date = int32(time.Now().Unix())
	}
	if err = c.pushSecretEncryption(chat.AdminID, secretChatDiscarded(chat), date); err != nil {
		return nil, err
	}
	if err = c.pushSecretEncryption(chat.ParticipantID, secretChatDiscarded(chat), date); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesSetEncryptedTyping(in *mtproto.TLMessagesSetEncryptedTyping) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetChatId() == 0 || in.GetPeer().GetAccessHash() == 0 || in.GetTyping() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	chat, err := domain.AuthorizeSecretChat(in.GetPeer().GetChatId(), in.GetPeer().GetAccessHash(), uid, true)
	if err != nil {
		return nil, mapSecretError(err)
	}
	if in.GetTyping().GetPredicateName() == mtproto.Predicate_boolTrue {
		peerID := chat.AdminID
		if uid == chat.AdminID {
			peerID = chat.ParticipantID
		}
		if err = c.pushSecretUpdate(peerID, mtproto.MakeTLUpdateEncryptedChatTyping(&mtproto.Update{ChatId_INT32: chat.ID}).To_Update()); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesReadEncryptedHistory(in *mtproto.TLMessagesReadEncryptedHistory) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetChatId() == 0 || in.GetPeer().GetAccessHash() == 0 || in.GetMaxDate() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	chat, peerID, err := domain.ReadSecretHistory(in.GetPeer().GetChatId(), in.GetPeer().GetAccessHash(), uid, in.GetMaxDate())
	if err != nil {
		return nil, mapSecretError(err)
	}
	if err = c.pushSecretUpdate(peerID, mtproto.MakeTLUpdateEncryptedMessagesRead(&mtproto.Update{ChatId_INT32: chat.ID, MaxDate: in.GetMaxDate(), Date_INT32: int32(time.Now().Unix())}).To_Update()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) saveAndPushSecretMessage(uid int64, peer *mtproto.InputEncryptedChat, randomID int64, data []byte, service bool, file *domain.SecretFile) (domain.SecretMessage, error) {
	if peer == nil || peer.GetChatId() == 0 || peer.GetAccessHash() == 0 {
		return domain.SecretMessage{}, mtproto.ErrEncryptionIdInvalid
	}
	if randomID == 0 {
		return domain.SecretMessage{}, mtproto.ErrRandomIdEmpty
	}
	if len(data) == 0 {
		return domain.SecretMessage{}, mtproto.ErrDataInvalid
	}
	if len(data) > maxSecretMessageBytes {
		return domain.SecretMessage{}, mtproto.ErrDataTooLong
	}
	message, _, err := domain.SaveSecretMessage(peer.GetChatId(), peer.GetAccessHash(), uid, randomID, data, service, file)
	if err != nil {
		return domain.SecretMessage{}, mapSecretError(err)
	}
	if err = c.pushSecretUpdate(message.RecipientID, mtproto.MakeTLUpdateNewEncryptedMessage(&mtproto.Update{Message_ENCRYPTEDMESSAGE: secretEncryptedMessage(message), Qts: message.QTS}).To_Update()); err != nil {
		return domain.SecretMessage{}, err
	}
	return message, nil
}

func sentEncryptedMessage(message domain.SecretMessage) *mtproto.Messages_SentEncryptedMessage {
	return mtproto.MakeTLMessagesSentEncryptedMessage(&mtproto.Messages_SentEncryptedMessage{Date: message.Date}).To_Messages_SentEncryptedMessage()
}

func (c *ApiFullCore) MessagesSendEncrypted(in *mtproto.TLMessagesSendEncrypted) (*mtproto.Messages_SentEncryptedMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	message, err := c.saveAndPushSecretMessage(uid, in.GetPeer(), in.GetRandomId(), in.GetData(), false, nil)
	if err != nil {
		return nil, err
	}
	return sentEncryptedMessage(message), nil
}

func (c *ApiFullCore) uploadSecretFile(input *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
	if input == nil || input.GetId() == 0 {
		return nil, mtproto.ErrFileIdInvalid
	}
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputEncryptedFileUploaded, mtproto.Predicate_inputEncryptedFileBigUploaded:
		if input.GetParts() <= 0 || input.GetKeyFingerprint() == 0 {
			return nil, mtproto.ErrFileIdInvalid
		}
	case mtproto.Predicate_inputEncryptedFile:
		if input.GetAccessHash() == 0 {
			return nil, mtproto.ErrFileIdInvalid
		}
	default:
		return nil, mtproto.ErrFileIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.DfsClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	file, err := d.DfsClient.DfsUploadEncryptedFileV2(c.secretContext(), &dfs.TLDfsUploadEncryptedFileV2{Creator: c.MD.GetPermAuthKeyId(), File: input})
	if err != nil {
		return nil, err
	}
	if file == nil || file.GetId() == 0 || file.GetAccessHash() == 0 || file.GetDcId() <= 0 || file.GetSize2_INT64() <= 0 {
		return nil, mtproto.ErrFileIdInvalid
	}
	if file.GetKeyFingerprint() == 0 {
		file.KeyFingerprint = input.GetKeyFingerprint()
	}
	return file, nil
}

func (c *ApiFullCore) MessagesSendEncryptedFile(in *mtproto.TLMessagesSendEncryptedFile) (*mtproto.Messages_SentEncryptedMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if _, err = domain.AuthorizeSecretChat(in.GetPeer().GetChatId(), in.GetPeer().GetAccessHash(), uid, true); err != nil {
		return nil, mapSecretError(err)
	}
	uploaded, err := c.uploadSecretFile(in.GetFile())
	if err != nil {
		return nil, err
	}
	file := &domain.SecretFile{ID: uploaded.GetId(), AccessHash: uploaded.GetAccessHash(), Size: uploaded.GetSize2_INT64(), DCID: uploaded.GetDcId(), KeyFingerprint: uploaded.GetKeyFingerprint()}
	message, err := c.saveAndPushSecretMessage(uid, in.GetPeer(), in.GetRandomId(), in.GetData(), false, file)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesSentEncryptedFile(&mtproto.Messages_SentEncryptedMessage{Date: message.Date, File: secretEncryptedFile(file)}).To_Messages_SentEncryptedMessage(), nil
}

func (c *ApiFullCore) MessagesSendEncryptedService(in *mtproto.TLMessagesSendEncryptedService) (*mtproto.Messages_SentEncryptedMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	message, err := c.saveAndPushSecretMessage(uid, in.GetPeer(), in.GetRandomId(), in.GetData(), true, nil)
	if err != nil {
		return nil, err
	}
	return sentEncryptedMessage(message), nil
}

func (c *ApiFullCore) MessagesReceivedQueue(in *mtproto.TLMessagesReceivedQueue) (*mtproto.Vector_Long, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetMaxQts() <= 0 {
		return nil, mtproto.ErrMaxQtsInvalid
	}
	ids, err := domain.ConfirmSecretQueue(uid, in.GetMaxQts())
	if err != nil {
		return nil, mapSecretError(err)
	}
	return &mtproto.Vector_Long{Datas: ids}, nil
}
