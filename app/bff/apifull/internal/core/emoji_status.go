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
	"encoding/json"
	"errors"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// RPCEmojiStatusServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func emojiStatusKey(userId int64) string {
	return "emojistatus:" + strconv.FormatInt(userId, 10)
}

func recentEmojiStatusKey(userId int64) string {
	return "emojistatus:recent:" + strconv.FormatInt(userId, 10)
}

func (c *ApiFullCore) AccountUpdateEmojiStatus(in *mtproto.TLAccountUpdateEmojiStatus) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetEmojiStatus() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil || d.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	status := in.GetEmojiStatus()
	var documentID int64
	var until int32
	switch status.GetPredicateName() {
	case mtproto.Predicate_emojiStatusEmpty:
	case mtproto.Predicate_emojiStatus:
		documentID = status.GetDocumentId()
		if status.GetUntil_FLAGINT32() != nil {
			until = status.GetUntil_FLAGINT32().GetValue()
		} else {
			until = status.GetUntil_INT32()
		}
	case mtproto.Predicate_emojiStatusUntil:
		documentID = status.GetDocumentId()
		until = status.GetUntil_INT32()
	default:
		// The user service stores only a document ID and expiry; collectible
		// statuses cannot be represented without losing their ownership data.
		return nil, mtproto.ErrMethodNotImpl
	}
	if status.GetPredicateName() != mtproto.Predicate_emojiStatusEmpty && documentID == 0 {
		return nil, mtproto.ErrDocumentInvalid
	}
	payload := "null"
	var recent []*mtproto.EmojiStatus
	updateRecent := false
	if status.GetPredicateName() != mtproto.Predicate_emojiStatusEmpty {
		b, mErr := json.Marshal(status)
		if mErr != nil {
			return nil, mErr
		}
		payload = string(b)
		recent, err = loadStoredRecentEmojiStatuses(uid)
		if err != nil {
			return nil, err
		}
		recent = prependRecentEmojiStatus(status, recent)
		updateRecent = true
	}
	updated, err := d.UserClient.UserUpdateEmojiStatus(ctx, &userpb.TLUserUpdateEmojiStatus{
		UserId:                uid,
		EmojiStatusDocumentId: documentID,
		EmojiStatusUntil:      until,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || !mtproto.FromBool(updated) {
		return nil, mtproto.ErrInternalServerError
	}
	if err = persist.Default.Set(emojiStatusKey(uid), payload); err != nil {
		return nil, err
	}
	if updateRecent {
		if err = saveRecentEmojiStatuses(uid, recent); err != nil {
			return nil, err
		}
	}
	updates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateUserEmojiStatus(&mtproto.Update{
		UserId:      uid,
		EmojiStatus: status,
	}).To_Update())
	if _, err = d.SyncClient.SyncUpdatesNotMe(ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        uid,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       updates,
	}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func loadStoredEmojiStatus(userID int64) ([]*mtproto.EmojiStatus, error) {
	raw, err := persist.Default.Get(emojiStatusKey(userID))
	if err != nil || raw == "" || raw == "null" {
		return []*mtproto.EmojiStatus{}, err
	}
	st := &mtproto.EmojiStatus{}
	if err = json.Unmarshal([]byte(raw), st); err != nil {
		return nil, err
	}
	if st.GetDocumentId() == 0 && st.GetCollectibleId() == 0 {
		return []*mtproto.EmojiStatus{}, nil
	}
	return []*mtproto.EmojiStatus{st}, nil
}

func loadStoredRecentEmojiStatuses(userID int64) ([]*mtproto.EmojiStatus, error) {
	raw, err := persist.Default.Get(recentEmojiStatusKey(userID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		// Preserve the old recent-status view once for users without a separate history.
		statuses, err := loadStoredEmojiStatus(userID)
		if err != nil {
			return nil, err
		}
		if len(statuses) > 0 {
			if err = saveRecentEmojiStatuses(userID, statuses); err != nil {
				return nil, err
			}
		}
		return statuses, nil
	}
	var statuses []*mtproto.EmojiStatus
	if err = json.Unmarshal([]byte(raw), &statuses); err != nil {
		return nil, err
	}
	if statuses == nil {
		statuses = []*mtproto.EmojiStatus{}
	}
	return statuses, nil
}

func prependRecentEmojiStatus(status *mtproto.EmojiStatus, statuses []*mtproto.EmojiStatus) []*mtproto.EmojiStatus {
	if status == nil {
		return statuses
	}
	recent := make([]*mtproto.EmojiStatus, 0, len(statuses)+1)
	recent = append(recent, status)
	for _, existing := range statuses {
		if existing == nil || sameEmojiStatus(status, existing) {
			continue
		}
		recent = append(recent, existing)
	}
	return recent
}

func sameEmojiStatus(a, b *mtproto.EmojiStatus) bool {
	return a != nil && b != nil &&
		a.GetPredicateName() == b.GetPredicateName() &&
		a.GetDocumentId() == b.GetDocumentId() &&
		a.GetCollectibleId() == b.GetCollectibleId()
}

func saveRecentEmojiStatuses(userID int64, statuses []*mtproto.EmojiStatus) error {
	if statuses == nil {
		statuses = []*mtproto.EmojiStatus{}
	}
	b, err := json.Marshal(statuses)
	if err != nil {
		return err
	}
	return persist.Default.Set(recentEmojiStatusKey(userID), string(b))
}

func recentEmojiStatusHash(statuses []*mtproto.EmojiStatus) int64 {
	var hash int64 = 1
	for _, status := range statuses {
		if status == nil {
			hash = hash*31 + 1
			continue
		}
		hash = hash*31 + int64(len(status.GetPredicateName()))
		hash = hash*31 + status.GetDocumentId()
		hash = hash*31 + status.GetCollectibleId()
		if until := status.GetUntil_FLAGINT32(); until != nil {
			hash = hash*31 + int64(until.GetValue())
		} else {
			hash = hash*31 + int64(status.GetUntil_INT32())
		}
	}
	if hash < 0 {
		return -hash
	}
	return hash
}

// loadEmojiStatusCatalog returns document-backed status values from the
// PostgreSQL emoji catalogue. The catalogue owns the available documents;
// this boundary only maps their IDs to the Layer 229 status constructor.
func loadEmojiStatusCatalog(c *ApiFullCore, kind string) ([]*mtproto.EmojiStatus, error) {
	ids, err := persist.LoadEmojiDocumentIDs(stickerRequestContext(c), kind)
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	if len(ids) == 0 {
		return nil, stickersProviderUnavailable(c)
	}
	statuses := make([]*mtproto.EmojiStatus, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		statuses = append(statuses, mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{
			DocumentId: id,
		}).To_EmojiStatus())
	}
	return statuses, nil
}

func emojiStatusesReply(hash int64, statuses []*mtproto.EmojiStatus) *mtproto.Account_EmojiStatuses {
	currentHash := recentEmojiStatusHash(statuses)
	if hash != 0 && hash == currentHash {
		return mtproto.MakeTLAccountEmojiStatusesNotModified(&mtproto.Account_EmojiStatuses{}).To_Account_EmojiStatuses()
	}
	return mtproto.MakeTLAccountEmojiStatuses(&mtproto.Account_EmojiStatuses{
		Hash: currentHash, Statuses: statuses,
	}).To_Account_EmojiStatuses()
}

func (c *ApiFullCore) AccountGetDefaultEmojiStatuses(in *mtproto.TLAccountGetDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	statuses, err := loadEmojiStatusCatalog(c, "status")
	if err != nil {
		return nil, err
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return emojiStatusesReply(hash, statuses), nil
}

func (c *ApiFullCore) AccountGetRecentEmojiStatuses(in *mtproto.TLAccountGetRecentEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	statuses, err := loadStoredRecentEmojiStatuses(uid)
	if err != nil {
		return nil, err
	}
	hash := recentEmojiStatusHash(statuses)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLAccountEmojiStatusesNotModified(&mtproto.Account_EmojiStatuses{}).To_Account_EmojiStatuses(), nil
	}
	return mtproto.MakeTLAccountEmojiStatuses(&mtproto.Account_EmojiStatuses{
		Hash:     hash,
		Statuses: statuses,
	}).To_Account_EmojiStatuses(), nil
}

func (c *ApiFullCore) AccountClearRecentEmojiStatuses(in *mtproto.TLAccountClearRecentEmojiStatuses) (*mtproto.Bool, error) {
	_ = in
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = saveRecentEmojiStatuses(uid, []*mtproto.EmojiStatus{}); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) AccountGetChannelDefaultEmojiStatuses(in *mtproto.TLAccountGetChannelDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	statuses, err := loadEmojiStatusCatalog(c, "channel_status")
	if err != nil {
		return nil, err
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return emojiStatusesReply(hash, statuses), nil
}

func (c *ApiFullCore) AccountGetChannelRestrictedStatusEmojis(in *mtproto.TLAccountGetChannelRestrictedStatusEmojis) (*mtproto.EmojiList, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	ids, err := persist.LoadEmojiDocumentIDs(stickerRequestContext(c), "restricted_status")
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	if len(ids) == 0 {
		return nil, stickersProviderUnavailable(c)
	}
	hash := emojiListHash(ids)
	if in != nil && in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{Hash: hash}).To_EmojiList(), nil
	}
	return emojiListReply(ids, hash), nil
}

func (c *ApiFullCore) AccountGetCollectibleEmojiStatuses(in *mtproto.TLAccountGetCollectibleEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	// Collectible status ownership is not persisted by this deployment. Return
	// an empty catalog so clients can complete initialization and cache it.
	_ = in
	return mtproto.MakeTLAccountEmojiStatuses(&mtproto.Account_EmojiStatuses{
		Hash:     0,
		Statuses: []*mtproto.EmojiStatus{},
	}).To_Account_EmojiStatuses(), nil
}

func (c *ApiFullCore) ChannelsUpdateEmojiStatus(in *mtproto.TLChannelsUpdateEmojiStatus) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetChannel() == nil || in.GetEmojiStatus() == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	channelID := in.GetChannel().GetChannelId()
	if channelID == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	if _, err = c.resolveMemberChannel(uid, in.GetChannel(), channelID); err != nil {
		return nil, err
	}
	status := in.GetEmojiStatus()
	documentID := int64(0)
	until := int32(0)
	switch status.GetPredicateName() {
	case mtproto.Predicate_emojiStatusEmpty:
	case mtproto.Predicate_emojiStatus:
		documentID = status.GetDocumentId()
		if status.GetUntil_FLAGINT32() != nil {
			until = status.GetUntil_FLAGINT32().GetValue()
		}
	case mtproto.Predicate_emojiStatusUntil:
		documentID = status.GetDocumentId()
		until = status.GetUntil_INT32()
	case mtproto.Predicate_emojiStatusCollectible:
		return nil, mtproto.ErrMethodNotImpl
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if documentID < 0 || until < 0 {
		return nil, mtproto.ErrDocumentInvalid
	}
	if documentID > 0 {
		found, catalogErr := persist.EmojiDocumentInCatalog(stickerRequestContext(c), documentID, "channel_status")
		if catalogErr != nil {
			return nil, customEmojiProviderError(c, catalogErr)
		}
		if !found {
			return nil, mtproto.ErrDocumentInvalid
		}
	}
	if err = domain.UpdateChannelEmojiStatus(uid, channelID, documentID, until); err != nil {
		switch {
		case errors.Is(err, domain.ErrChannelMissing):
			return nil, mtproto.ErrChannelInvalid
		case errors.Is(err, domain.ErrNotCreator):
			return nil, mtproto.ErrChatAdminRequired
		default:
			return nil, err
		}
	}
	channel, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, mtproto.ErrChannelInvalid
	}
	update := mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: channelID}).To_Update()
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{channelview.Chat(channel, channel.Creator == uid)}, update), nil
}

func (c *ApiFullCore) BotsUpdateUserEmojiStatus(in *mtproto.TLBotsUpdateUserEmojiStatus) (*mtproto.Bool, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsToggleUserEmojiStatusPermission(in *mtproto.TLBotsToggleUserEmojiStatusPermission) (*mtproto.Bool, error) {
	_ = in
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
