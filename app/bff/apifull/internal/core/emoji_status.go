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
	updateRecent := false
	var recentFallback []*mtproto.EmojiStatus
	if status.GetPredicateName() != mtproto.Predicate_emojiStatusEmpty {
		b, mErr := json.Marshal(status)
		if mErr != nil {
			return nil, mErr
		}
		payload = string(b)
		recentFallback, err = loadStoredRecentEmojiStatuses(uid)
		if err != nil {
			return nil, err
		}
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
		if err = updateRecentEmojiStatuses(uid, status, recentFallback); err != nil {
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

// updateRecentEmojiStatuses performs the recent-status read/modify/write in a
// single shared-store transaction. PostgreSQL locks the owner key so concurrent
// status updates cannot overwrite one another's history.
func updateRecentEmojiStatuses(userID int64, status *mtproto.EmojiStatus, fallback []*mtproto.EmojiStatus) error {
	if userID <= 0 || status == nil {
		return mtproto.ErrAuthKeyUnregistered
	}
	return persist.Update(recentEmojiStatusKey(userID), func(raw string) (string, error) {
		var statuses []*mtproto.EmojiStatus
		if raw == "" {
			// Preserve the one-time migration from the current status key. The
			// fallback is read before entering this transaction so the callback
			// never opens a second connection while PostgreSQL holds its lock.
			statuses = fallback
		} else if err := json.Unmarshal([]byte(raw), &statuses); err != nil {
			return "", err
		}
		statuses = prependRecentEmojiStatus(status, statuses)
		b, err := json.Marshal(statuses)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
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
	return emojiStatusesReplyWithHash(hash, recentEmojiStatusHash(statuses), statuses)
}

func emojiStatusesReplyWithHash(requestedHash, currentHash int64, statuses []*mtproto.EmojiStatus) *mtproto.Account_EmojiStatuses {
	if requestedHash != 0 && requestedHash == currentHash {
		return mtproto.MakeTLAccountEmojiStatusesNotModified(&mtproto.Account_EmojiStatuses{}).To_Account_EmojiStatuses()
	}
	return mtproto.MakeTLAccountEmojiStatuses(&mtproto.Account_EmojiStatuses{
		Hash: currentHash, Statuses: statuses,
	}).To_Account_EmojiStatuses()
}

func collectibleEmojiStatusHash(statuses []*mtproto.EmojiStatus) int64 {
	hash := recentEmojiStatusHash(statuses)
	for _, status := range statuses {
		if status == nil || status.GetPredicateName() != mtproto.Predicate_emojiStatusCollectible {
			continue
		}
		for _, value := range []string{status.GetTitle(), status.GetSlug()} {
			for _, b := range []byte(value) {
				hash = hash*31 + int64(b)
			}
		}
		hash = hash*31 + status.GetPatternDocumentId()
		hash = hash*31 + int64(status.GetCenterColor())
		hash = hash*31 + int64(status.GetEdgeColor())
		hash = hash*31 + int64(status.GetPatternColor())
		hash = hash*31 + int64(status.GetTextColor())
	}
	if hash < 0 {
		return -hash
	}
	return hash
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	rows, err := persist.LoadCollectibleEmojiStatuses(stickerRequestContext(c), uid)
	if err != nil {
		return nil, customEmojiProviderError(c, err)
	}
	if len(rows) == 0 {
		// Do not return an empty success: without provider-owned rows the
		// client cannot distinguish "no inventory" from an unavailable
		// collectible provider and would cache a synthetic catalogue.
		return nil, stickersProviderUnavailable(c)
	}
	statuses := make([]*mtproto.EmojiStatus, 0, len(rows))
	for _, row := range rows {
		if row.CollectibleID <= 0 || row.DocumentID <= 0 {
			return nil, mtproto.ErrInternalServerError
		}
		statuses = append(statuses, mtproto.MakeTLEmojiStatusCollectible(&mtproto.EmojiStatus{
			CollectibleId:     row.CollectibleID,
			DocumentId:        row.DocumentID,
			Title:             row.Title,
			Slug:              row.Slug,
			PatternDocumentId: row.PatternDocumentID,
			CenterColor:       row.CenterColor,
			EdgeColor:         row.EdgeColor,
			PatternColor:      row.PatternColor,
			TextColor:         row.TextColor,
		}).To_EmojiStatus())
	}
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return emojiStatusesReplyWithHash(hash, collectibleEmojiStatusHash(statuses), statuses), nil
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
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetUserId() == nil || in.GetEmojiStatus() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	target := in.GetUserId()
	switch target.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
	case mtproto.Predicate_inputUser:
		if target.GetUserId() <= 0 || target.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	status := in.GetEmojiStatus()
	switch status.GetPredicateName() {
	case mtproto.Predicate_emojiStatusEmpty,
		mtproto.Predicate_emojiStatus,
		mtproto.Predicate_emojiStatusUntil:
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.businessProviderConfigured() {
		var result struct {
			Value *bool `json:"value"`
		}
		if err := c.callBusinessProvider("bots.updateUserEmojiStatus", map[string]any{
			"caller_user_id": uid, "user_id": target, "emoji_status": status,
		}, &result); err != nil {
			return nil, err
		}
		if result.Value == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if *result.Value {
			return mtproto.BoolTrue, nil
		}
		return mtproto.BoolFalse, nil
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	targetID, err := c.resolveEmojiStatusUser(target)
	if err != nil {
		return nil, err
	}
	if _, err = c.requireEmojiStatusBot(uid); err != nil {
		return nil, err
	}
	if targetID != uid {
		enabled, permissionErr := persist.UserEmojiStatusPermission(c.requestContext(), targetID, uid)
		if permissionErr != nil {
			return nil, permissionErr
		}
		if !enabled {
			return nil, mtproto.ErrBotMethodInvalid
		}
	}
	documentID, until := emojiStatusFields(status)
	if documentID > 0 {
		found, catalogErr := persist.EmojiDocumentInCatalog(stickerRequestContext(c), documentID, "status")
		if catalogErr != nil {
			return nil, customEmojiProviderError(c, catalogErr)
		}
		if !found {
			return nil, mtproto.ErrDocumentInvalid
		}
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	updated, err := d.UserClient.UserUpdateEmojiStatus(c.requestContext(), &userpb.TLUserUpdateEmojiStatus{
		UserId: targetID, EmojiStatusDocumentId: documentID, EmojiStatusUntil: until,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || !mtproto.FromBool(updated) {
		return nil, mtproto.ErrInternalServerError
	}
	payload := "null"
	if status.GetPredicateName() != mtproto.Predicate_emojiStatusEmpty {
		encoded, marshalErr := json.Marshal(status)
		if marshalErr != nil {
			return nil, marshalErr
		}
		payload = string(encoded)
	}
	if err = persist.Default.Set(emojiStatusKey(targetID), payload); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsToggleUserEmojiStatusPermission(in *mtproto.TLBotsToggleUserEmojiStatusPermission) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetEnabled() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	bot := in.GetBot()
	switch bot.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
	case mtproto.Predicate_inputUser:
		if bot.GetUserId() <= 0 || bot.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.businessProviderConfigured() {
		var result struct {
			Value *bool `json:"value"`
		}
		if err := c.callBusinessProvider("bots.toggleUserEmojiStatusPermission", map[string]any{
			"caller_user_id": uid, "bot": bot, "enabled": in.GetEnabled(),
		}, &result); err != nil {
			return nil, err
		}
		if result.Value == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if *result.Value {
			return mtproto.BoolTrue, nil
		}
		return mtproto.BoolFalse, nil
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := c.resolveEmojiStatusUser(bot)
	if err != nil {
		return nil, err
	}
	if _, err = c.requireEmojiStatusBot(botID); err != nil {
		return nil, err
	}
	if err = persist.SetUserEmojiStatusPermission(c.requestContext(), uid, botID, mtproto.FromBool(in.GetEnabled())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) requestContext() context.Context {
	if c != nil && c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func emojiStatusFields(status *mtproto.EmojiStatus) (int64, int32) {
	if status == nil || status.GetPredicateName() == mtproto.Predicate_emojiStatusEmpty {
		return 0, 0
	}
	if status.GetPredicateName() == mtproto.Predicate_emojiStatusUntil {
		return status.GetDocumentId(), status.GetUntil_INT32()
	}
	until := int32(0)
	if status.GetUntil_FLAGINT32() != nil {
		until = status.GetUntil_FLAGINT32().GetValue()
	}
	return status.GetDocumentId(), until
}

func (c *ApiFullCore) resolveEmojiStatusUser(in *mtproto.InputUser) (int64, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	if in.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return uid, nil
	}
	if in.GetPredicateName() != mtproto.Predicate_inputUser || in.GetUserId() <= 0 || in.GetAccessHash() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	profile, err := c.loadEmojiStatusUser(in.GetUserId())
	if err != nil {
		return 0, err
	}
	if profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetAccessHash() != in.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return in.GetUserId(), nil
}

func (c *ApiFullCore) requireEmojiStatusBot(userID int64) (*mtproto.ImmutableUser, error) {
	profile, err := c.loadEmojiStatusUser(userID)
	if err != nil {
		return nil, err
	}
	if profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetBot() == nil {
		return nil, mtproto.ErrBotInvalid
	}
	return profile, nil
}

func (c *ApiFullCore) loadEmojiStatusUser(userID int64) (*mtproto.ImmutableUser, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.requestContext(), &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	return profile, nil
}
