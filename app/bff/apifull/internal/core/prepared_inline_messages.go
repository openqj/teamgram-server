// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package core

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	preparedInlineTTL       = 24 * time.Hour
	preparedInlineCacheTime = int32(300)
)

// RPCPreparedInlineMessagesServer: Layer 229 prepared inline messages keep
// the complete result envelope server-side and return only an opaque ID to the
// saving bot.

func preparedResultID(in *mtproto.TLMessagesSavePreparedInlineMessage) string {
	if in == nil || in.GetResult() == nil {
		return ""
	}
	return strings.TrimSpace(in.GetResult().GetId())
}

func (c *ApiFullCore) MessagesSavePreparedInlineMessage(in *mtproto.TLMessagesSavePreparedInlineMessage) (*mtproto.Messages_BotPreparedInlineMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetResult() == nil || preparedResultID(in) == "" || len(preparedResultID(in)) > 128 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := in.GetResult()
	// Keep the old ID-only records readable for deployments that created them
	// before complete prepared-inline persistence was introduced.
	if isLegacyPreparedInlineResult(result) {
		if err = persist.Default.Set(b7Key(uid, preparedResultID(in)), preparedResultID(in)); err != nil {
			return nil, err
		}
		return preparedInlineSavedResponse(preparedResultID(in), time.Now().UTC().Add(preparedInlineTTL).Unix()), nil
	}
	if err = c.verifyInlineBotCaller(uid); err != nil {
		return nil, err
	}
	if err = validatePreparedInlineResult(result); err != nil {
		return nil, err
	}
	targetUserID, err := c.validatePreparedInlineTarget(in.GetUserId())
	if err != nil {
		return nil, err
	}
	peerTypes, err := marshalPreparedInlinePeerTypes(in.GetPeerTypes())
	if err != nil {
		return nil, err
	}
	resultRaw, err := json.Marshal(result)
	if err != nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	expiresAt := time.Now().UTC().Add(preparedInlineTTL).Unix()
	if err = persist.SavePreparedInlineMessage(persist.PreparedInlineMessage{
		ID:           preparedResultID(in),
		BotID:        uid,
		TargetUserID: targetUserID,
		Result:       resultRaw,
		PeerTypes:    peerTypes,
		CacheTime:    preparedInlineCacheTime,
		ExpiresAt:    expiresAt,
	}); err != nil {
		if errors.Is(err, persist.ErrPreparedInlineConflict) {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return nil, err
	}
	return preparedInlineSavedResponse(preparedResultID(in), expiresAt), nil
}

func (c *ApiFullCore) MessagesGetPreparedInlineMessage(in *mtproto.TLMessagesGetPreparedInlineMessage) (*mtproto.Messages_PreparedInlineMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetId()) == "" || len(in.GetId()) > 128 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	// The complete path always authenticates the bot and its access hash before
	// loading the opaque record. A nil bot is accepted only for legacy ID-only
	// records written by older versions.
	if in.GetBot() != nil {
		botID, verifyErr := c.verifyInlineBot(in.GetBot())
		if verifyErr != nil {
			return nil, verifyErr
		}
		stored, loadErr := persist.LoadPreparedInlineMessage(botID, in.GetId())
		if loadErr == nil {
			if stored.TargetUserID != 0 && stored.TargetUserID != uid {
				return nil, mtproto.ErrUserIdInvalid
			}
			return c.preparedInlineResponse(stored)
		}
		if !errors.Is(loadErr, persist.ErrPreparedInlineNotFound) {
			return nil, loadErr
		}
		return nil, mtproto.ErrResultIdInvalid
	}

	stored, loadErr := persist.Default.Get(b7Key(uid, in.GetId()))
	if loadErr != nil {
		return nil, loadErr
	}
	if strings.TrimSpace(stored) == "" {
		return nil, mtproto.ErrResultIdInvalid
	}
	return mtproto.MakeTLMessagesPreparedInlineMessage(&mtproto.Messages_PreparedInlineMessage{
		Result: mtproto.MakeTLBotInlineResult(&mtproto.BotInlineResult{
			Id:          stored,
			Type:        "article",
			SendMessage: mtproto.MakeTLBotInlineMessageText(&mtproto.BotInlineMessage{}).To_BotInlineMessage(),
		}).To_BotInlineResult(),
		CacheTime: preparedInlineCacheTime,
		PeerTypes: []*mtproto.InlineQueryPeerType{},
		Users:     []*mtproto.User{},
	}).To_Messages_PreparedInlineMessage(), nil
}

func (c *ApiFullCore) validatePreparedInlineTarget(input *mtproto.InputUser) (int64, error) {
	if input == nil {
		return 0, nil
	}
	if input.GetPredicateName() != mtproto.Predicate_inputUser || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: input.GetUserId()})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetAccessHash() != input.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return input.GetUserId(), nil
}

func isLegacyPreparedInlineResult(result *mtproto.InputBotInlineResult) bool {
	return result != nil && strings.TrimSpace(result.GetPredicateName()) == "" &&
		strings.TrimSpace(result.GetType()) == "" && result.GetTitle() == nil && result.GetDescription() == nil &&
		result.GetUrl() == nil && result.GetThumb() == nil && result.GetContent() == nil && result.GetSendMessage() == nil &&
		result.GetPhoto() == nil && result.GetDocument() == nil && strings.TrimSpace(result.GetShortName()) == ""
}

func validatePreparedInlineResult(result *mtproto.InputBotInlineResult) error {
	if result == nil || strings.TrimSpace(result.GetId()) == "" || strings.TrimSpace(result.GetType()) == "" {
		return mtproto.ErrInputRequestInvalid
	}
	if err := validateInlineResultPredicate(result.GetPredicateName()); err != nil {
		return err
	}
	if !validInlineResultType(result.GetType()) {
		return mtproto.ErrInputRequestInvalid
	}
	if result.GetSendMessage() != nil {
		if err := validateInlineMessagePredicate(result.GetSendMessage().GetPredicateName()); err != nil {
			return err
		}
	}
	switch result.GetPredicateName() {
	case mtproto.Predicate_inputBotInlineResultPhoto:
		if result.GetPhoto() == nil {
			return mtproto.ErrInputRequestInvalid
		}
	case mtproto.Predicate_inputBotInlineResultDocument:
		if result.GetDocument() == nil {
			return mtproto.ErrInputRequestInvalid
		}
	case mtproto.Predicate_inputBotInlineResultGame:
		if strings.TrimSpace(result.GetShortName()) == "" {
			return mtproto.ErrInputRequestInvalid
		}
	}
	return nil
}

func marshalPreparedInlinePeerTypes(peerTypes []*mtproto.InlineQueryPeerType) ([]json.RawMessage, error) {
	if len(peerTypes) > 16 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := make([]json.RawMessage, 0, len(peerTypes))
	seen := make(map[string]struct{}, len(peerTypes))
	for _, peerType := range peerTypes {
		if peerType == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		switch peerType.GetPredicateName() {
		case mtproto.Predicate_inlineQueryPeerTypeSameBotPM,
			mtproto.Predicate_inlineQueryPeerTypePM,
			mtproto.Predicate_inlineQueryPeerTypeChat,
			mtproto.Predicate_inlineQueryPeerTypeMegagroup,
			mtproto.Predicate_inlineQueryPeerTypeBroadcast,
			mtproto.Predicate_inlineQueryPeerTypeBotPM:
		default:
			return nil, mtproto.ErrInputConstructorInvalid
		}
		if _, ok := seen[peerType.GetPredicateName()]; ok {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seen[peerType.GetPredicateName()] = struct{}{}
		raw, err := json.Marshal(peerType)
		if err != nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		result = append(result, raw)
	}
	return result, nil
}

func preparedInlineSavedResponse(id string, expiresAt int64) *mtproto.Messages_BotPreparedInlineMessage {
	return mtproto.MakeTLMessagesBotPreparedInlineMessage(&mtproto.Messages_BotPreparedInlineMessage{
		Id:         id,
		ExpireDate: int32(expiresAt),
	}).To_Messages_BotPreparedInlineMessage()
}

func (c *ApiFullCore) preparedInlineResponse(stored *persist.PreparedInlineMessage) (*mtproto.Messages_PreparedInlineMessage, error) {
	if stored == nil || stored.BotID <= 0 || stored.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, mtproto.ErrResultIdInvalid
	}
	var input mtproto.InputBotInlineResult
	if err := json.Unmarshal(stored.Result, &input); err != nil {
		return nil, mtproto.ErrResultIdInvalid
	}
	result, err := inputInlineResultToOutput(&input)
	if err != nil {
		return nil, err
	}
	peerTypes := make([]*mtproto.InlineQueryPeerType, 0, len(stored.PeerTypes))
	for _, raw := range stored.PeerTypes {
		var peerType mtproto.InlineQueryPeerType
		if err = json.Unmarshal(raw, &peerType); err != nil {
			return nil, mtproto.ErrResultIdInvalid
		}
		converted, convertErr := inlineQueryPeerTypeFromPredicate(peerType.GetPredicateName())
		if convertErr != nil {
			return nil, mtproto.ErrResultIdInvalid
		}
		peerTypes = append(peerTypes, converted)
	}
	users := []*mtproto.User{}
	if c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil {
		profile, profileErr := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: stored.BotID})
		if profileErr != nil {
			return nil, profileErr
		}
		if profile == nil || profile.GetUser() == nil || profile.Deleted() {
			return nil, mtproto.ErrBotInvalid
		}
		users = append(users, profile.ToUser(c.MD.GetUserId()))
	}
	return mtproto.MakeTLMessagesPreparedInlineMessage(&mtproto.Messages_PreparedInlineMessage{
		Result:    result,
		PeerTypes: peerTypes,
		CacheTime: stored.CacheTime,
		Users:     users,
	}).To_Messages_PreparedInlineMessage(), nil
}
