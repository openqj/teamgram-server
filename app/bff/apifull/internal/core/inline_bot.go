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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/paymentinvoice"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	dfspb "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func b7Key(userId int64, part string) string {
	return "b7:" + strconv.FormatInt(userId, 10) + ":" + part
}

// RPCInlineBotServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesGetInlineBotResults(in *mtproto.TLMessagesGetInlineBotResults) (*mtproto.Messages_BotResults, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetBot() == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = c.validateInlineQueryPeer(uid, in.GetPeer()); err != nil {
		return nil, err
	}
	peerKey, err := inlineQueryPeerKey(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if geoKey, geoErr := inlineQueryGeoKey(in.GetGeoPoint()); geoErr != nil {
		return nil, geoErr
	} else if geoKey != "" {
		peerKey += "|geo:" + geoKey
	}
	botID, err := c.verifyInlineBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	query, err := persist.FindInlineBotQuery(botID, uid, peerKey, in.GetQuery(), in.GetOffset())
	if err != nil {
		return nil, err
	}
	if query == nil {
		queryID, err := newInlineQueryID()
		if err != nil {
			return nil, mtproto.ErrInternalServerError
		}
		query = &persist.InlineBotQuery{
			QueryID:     queryID,
			BotID:       botID,
			RequesterID: uid,
			PeerKey:     peerKey,
			Query:       in.GetQuery(),
			Offset:      in.GetOffset(),
			ExpiresAt:   0,
			Results:     []json.RawMessage{},
		}
		if err = persist.CreateInlineBotQuery(*query); err != nil {
			return nil, err
		}
	}
	results, err := decodeInlineBotResults(query.Results)
	if err != nil {
		return nil, err
	}
	var nextOffset *wrapperspb.StringValue
	if query.NextOffset != "" {
		nextOffset = wrapperspb.String(query.NextOffset)
	}
	var switchPM *mtproto.InlineBotSwitchPM
	if query.SwitchPM != nil {
		switchPM = mtproto.MakeTLInlineBotSwitchPM(&mtproto.InlineBotSwitchPM{
			Text: query.SwitchPM.Text, StartParam: query.SwitchPM.StartParam,
		}).To_InlineBotSwitchPM()
	}
	var switchWebview *mtproto.InlineBotWebView
	if query.SwitchWebview != nil {
		switchWebview = mtproto.MakeTLInlineBotWebView(&mtproto.InlineBotWebView{
			Text: query.SwitchWebview.Text, Url: query.SwitchWebview.URL,
		}).To_InlineBotWebView()
	}
	users := []*mtproto.User{}
	if c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil {
		if profile, profileErr := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID}); profileErr == nil && profile != nil && profile.GetUser() != nil && !profile.Deleted() {
			users = append(users, profile.ToUser(uid))
		}
	}
	return mtproto.MakeTLMessagesBotResults(&mtproto.Messages_BotResults{
		Gallery:       query.Gallery,
		QueryId:       query.QueryID,
		NextOffset:    nextOffset,
		SwitchPm:      switchPM,
		SwitchWebview: switchWebview,
		Results:       results,
		CacheTime:     query.CacheTime,
		Users:         users,
	}).To_Messages_BotResults(), nil
}

// validateInlineQueryPeer checks the peer envelope before it becomes part of
// an inline-query record. InputPeerUser carries an access hash and must be
// checked against the authoritative User service; accepting only its numeric
// ID would let a stale or forged peer address another account. Chat peers do
// not carry an access hash, while channel peers do. The *FromMessage variants
// require message-service resolution that APIFull does not own, so they fail
// closed instead of being reduced to an ID-only peer.
func (c *ApiFullCore) validateInlineQueryPeer(callerID int64, peer *mtproto.InputPeer) error {
	if callerID <= 0 || peer == nil {
		return mtproto.ErrInputRequestInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 || peer.GetAccessHash() == 0 {
			return mtproto.ErrUserIdInvalid
		}
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
			return mtproto.ErrMethodNotImpl
		}
		profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: peer.GetUserId()})
		if err != nil {
			return err
		}
		if profile == nil || profile.GetUser() == nil || profile.Deleted() {
			return mtproto.ErrUserIdInvalid
		}
		if profile.GetUser().GetAccessHash() != peer.GetAccessHash() {
			return mtproto.ErrUserIdInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 {
			return mtproto.ErrPeerIdInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 || peer.GetAccessHash() == 0 {
			return mtproto.ErrChannelInvalid
		}
		if !domain.Ready() {
			return mtproto.ErrMethodNotImpl
		}
		channel, ok, err := domain.LoadChannel(peer.GetChannelId())
		if err != nil {
			return mtproto.ErrInternalServerError
		}
		if !ok || channel.AccessHash != peer.GetAccessHash() {
			return mtproto.ErrChannelInvalid
		}
		member, err := domain.ChannelIsMember(peer.GetChannelId(), callerID)
		if err != nil {
			return mtproto.ErrInternalServerError
		}
		if !member {
			return mtproto.ErrUserNotParticipant
		}
		return nil
	case mtproto.Predicate_inputPeerUserFromMessage, mtproto.Predicate_inputPeerChannelFromMessage:
		return mtproto.ErrMethodNotImpl
	default:
		return mtproto.ErrInputConstructorInvalid
	}
}

func inlineQueryPeerKey(callerID int64, peer *mtproto.InputPeer) (string, error) {
	if callerID <= 0 || peer == nil {
		return "", mtproto.ErrInputRequestInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return "self:" + strconv.FormatInt(callerID, 10), nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 || peer.GetAccessHash() == 0 {
			return "", mtproto.ErrUserIdInvalid
		}
		return "user:" + strconv.FormatInt(peer.GetUserId(), 10) + ":" + strconv.FormatInt(peer.GetAccessHash(), 10), nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 {
			return "", mtproto.ErrPeerIdInvalid
		}
		return "chat:" + strconv.FormatInt(peer.GetChatId(), 10), nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 || peer.GetAccessHash() == 0 {
			return "", mtproto.ErrChannelInvalid
		}
		return "channel:" + strconv.FormatInt(peer.GetChannelId(), 10) + ":" + strconv.FormatInt(peer.GetAccessHash(), 10), nil
	default:
		return "", mtproto.ErrMethodNotImpl
	}
}

func inlineQueryGeoKey(point *mtproto.InputGeoPoint) (string, error) {
	if point == nil {
		return "", nil
	}
	lat, long := point.GetLat(), point.GetLong()
	if math.IsNaN(lat) || math.IsNaN(long) || math.IsInf(lat, 0) || math.IsInf(long, 0) || lat < -90 || lat > 90 || long < -180 || long > 180 {
		return "", mtproto.ErrGeoPointInvalid
	}
	accuracy := int32(0)
	if point.GetAccuracyRadius() != nil {
		accuracy = point.GetAccuracyRadius().GetValue()
		if accuracy < 0 {
			return "", mtproto.ErrGeoPointInvalid
		}
	}
	return fmt.Sprintf("%.7f,%.7f,%d", lat, long, accuracy), nil
}

func (c *ApiFullCore) MessagesSetInlineBotResults(in *mtproto.TLMessagesSetInlineBotResults) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetQueryId() <= 0 || in.GetCacheTime() < 0 || len(in.GetResults()) > 50 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = c.verifyInlineBotCaller(uid); err != nil {
		return nil, err
	}
	encoded := make([]json.RawMessage, 0, len(in.GetResults()))
	seenResultIDs := make(map[string]struct{}, len(in.GetResults()))
	for _, result := range in.GetResults() {
		resultID := strings.TrimSpace(result.GetId())
		if result == nil || resultID == "" || len(resultID) > 64 || strings.TrimSpace(result.GetType()) == "" {
			return nil, mtproto.ErrInputRequestInvalid
		}
		if _, exists := seenResultIDs[resultID]; exists {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seenResultIDs[resultID] = struct{}{}
		if !validInlineResultType(result.GetType()) {
			return nil, mtproto.ErrInputRequestInvalid
		}
		if err = validateInlineResultPredicate(result.GetPredicateName()); err != nil {
			return nil, err
		}
		if result.GetSendMessage() != nil {
			if err = validateInlineMessagePredicate(result.GetSendMessage().GetPredicateName()); err != nil {
				return nil, err
			}
		}
		switch result.GetPredicateName() {
		case mtproto.Predicate_inputBotInlineResultPhoto:
			if result.GetPhoto() == nil {
				return nil, mtproto.ErrInputRequestInvalid
			}
		case mtproto.Predicate_inputBotInlineResultDocument:
			if result.GetDocument() == nil {
				return nil, mtproto.ErrInputRequestInvalid
			}
		case mtproto.Predicate_inputBotInlineResultGame:
			if strings.TrimSpace(result.GetShortName()) == "" {
				return nil, mtproto.ErrInputRequestInvalid
			}
		}
		raw, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		encoded = append(encoded, raw)
	}
	nextOffset := ""
	if in.GetNextOffset() != nil {
		nextOffset = in.GetNextOffset().GetValue()
		if len(nextOffset) > 64 {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	if err = validateInlineResultSwitches(in.GetSwitchPm(), in.GetSwitchWebview()); err != nil {
		return nil, err
	}
	// setInlineBotResults carries only query_id. Resolve the pending query
	// atomically across bot partitions, while requiring the authenticated bot
	// to match the bot that created the query.
	var switchPM *persist.InlineBotSwitchPM
	if value := in.GetSwitchPm(); value != nil {
		switchPM = &persist.InlineBotSwitchPM{Text: value.GetText(), StartParam: value.GetStartParam()}
	}
	var switchWebview *persist.InlineBotWebView
	if value := in.GetSwitchWebview(); value != nil {
		switchWebview = &persist.InlineBotWebView{Text: value.GetText(), URL: value.GetUrl()}
	}
	if err = persist.SetInlineBotResultsByQueryForBotWithSwitches(uid, in.GetQueryId(), in.GetGallery(), in.GetPrivate(), in.GetCacheTime(), nextOffset, switchPM, switchWebview, encoded); err != nil {
		if errors.Is(err, persist.ErrInlineQueryNotFound) {
			return nil, mtproto.ErrQueryIdInvalid
		}
		if errors.Is(err, persist.ErrInlineQueryConflict) {
			return nil, mtproto.ErrQueryIdInvalid
		}
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesSendInlineBotResult(in *mtproto.TLMessagesSendInlineBotResult) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetPeer() == nil || in.GetQueryId() <= 0 || strings.TrimSpace(in.GetId()) == "" || in.GetRandomId() == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = c.validateInlineQueryPeer(uid, in.GetPeer()); err != nil {
		return nil, err
	}
	query, err := persist.FindInlineBotQueryByID(in.GetQueryId(), uid)
	if err != nil {
		if errors.Is(err, persist.ErrInlineQueryNotFound) {
			return nil, mtproto.ErrQueryIdInvalid
		}
		return nil, err
	}
	if query == nil || !query.Ready {
		return nil, mtproto.ErrQueryIdInvalid
	}
	var selected *mtproto.InputBotInlineResult
	for _, raw := range query.Results {
		var candidate mtproto.InputBotInlineResult
		if err = json.Unmarshal(raw, &candidate); err != nil {
			return nil, mtproto.ErrInternalServerError
		}
		if candidate.GetId() == in.GetId() {
			selected = &candidate
			break
		}
	}
	if selected == nil {
		return nil, mtproto.ErrResultIdInvalid
	}
	if selected.GetSendMessage() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	send := selected.GetSendMessage()
	if err = validateInlineMessagePredicate(send.GetPredicateName()); err != nil {
		return nil, err
	}
	peer := mtproto.FromInputPeer2(uid, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsSelfUser(uid) {
		peer.PeerType = mtproto.PEER_USER
	}
	if c.apifullDao() == nil || c.apifullDao().ScheduledMessageSender == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:         true,
		FromId:      mtproto.MakePeerUser(uid),
		PeerId:      peer.ToPeer(),
		ViaBotId:    wrapperspb.Int64(query.BotID),
		Silent:      in.GetSilent(),
		InvertMedia: send.GetInvertMedia(),
		Date:        int32(time.Now().Unix()),
		Message:     send.GetMessage(),
		ReplyMarkup: send.GetReplyMarkup(),
		Entities:    send.GetEntities(),
	}).To_Message()
	media, richMessage, mediaErr := c.inlineInputToMessageMedia(selected, send)
	if mediaErr != nil {
		return nil, mediaErr
	}
	out.Media = media
	out.RichMessage = richMessage
	if strings.TrimSpace(send.GetMessage()) == "" && media == nil && richMessage == nil {
		return nil, mtproto.ErrMessageEmpty
	}
	if in.GetReplyToMsgId() != nil {
		replyID := in.GetReplyToMsgId().GetValue()
		if replyID <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		out.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId:           replyID,
			ReplyToMsgId_INT32:     replyID,
			ReplyToMsgId_FLAGINT32: in.GetReplyToMsgId(),
		}).To_MessageReplyHeader()
	}
	updates, err := c.apifullDao().ScheduledMessageSender.MsgSendMessageV2(callContext(c), &msgpb.TLMsgSendMessageV2{
		UserId:    uid,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			NoWebpage:    send.GetNoWebpage(),
			Background:   in.GetBackground(),
			RandomId:     in.GetRandomId(),
			Message:      out,
			ScheduleDate: in.GetScheduleDate(),
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

func (c *ApiFullCore) MessagesEditInlineBotMessage(in *mtproto.TLMessagesEditInlineBotMessage) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetId() == nil || (in.GetMessage() == nil && in.GetMedia() == nil && in.GetRichMessage() == nil && in.GetReplyMarkup() == nil) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateInputBotInlineMessageIDInput(in.GetId()); err != nil {
		return nil, err
	}
	var result struct {
		Value *bool `json:"value"`
	}
	inlineID := in.GetId()
	inlineKey := fmt.Sprintf("inline:%d:%d:%d:%d", inlineID.GetDcId(), inlineID.GetId_INT64(), inlineID.GetId_INT32(), inlineID.GetAccessHash())
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.editInlineBotMessage", inlineKey, &result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := c.requireBotCaller(c.MD.GetUserId()); err != nil {
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
	if err := c.callBusinessProvider("messages.editInlineBotMessage", map[string]any{
		"no_webpage": in.GetNoWebpage(), "invert_media": in.GetInvertMedia(), "id": in.GetId(), "message": in.GetMessage(), "media": in.GetMedia(), "reply_markup": in.GetReplyMarkup(), "entities": in.GetEntities(), "rich_message": in.GetRichMessage(),
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

func (c *ApiFullCore) MessagesGetBotCallbackAnswer(in *mtproto.TLMessagesGetBotCallbackAnswer) (*mtproto.Messages_BotCallbackAnswer, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetMsgId() <= 0 || len(in.GetData()) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.Messages_BotCallbackAnswer)
	scopeKey := fmt.Sprintf("%s:msg:%d:%s", businessControlPeerKey(c.MD.GetUserId(), in.GetPeer()), in.GetMsgId(), businessControlHash("data", in.GetData()))
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.getBotCallbackAnswer", scopeKey, result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := c.validateInlineQueryPeer(c.MD.GetUserId(), in.GetPeer()); err != nil {
			return nil, err
		}
		if result.GetPredicateName() != mtproto.Predicate_messages_botCallbackAnswer || result.GetCacheTime() < 0 {
			return nil, mtproto.ErrInternalServerError
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.getBotCallbackAnswer", map[string]any{
		"game": in.GetGame(), "peer": in.GetPeer(), "msg_id": in.GetMsgId(), "data": in.GetData(), "password": in.GetPassword(),
	}, result); err != nil {
		return nil, err
	}
	if result.GetPredicateName() != mtproto.Predicate_messages_botCallbackAnswer || result.GetCacheTime() < 0 {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) MessagesSetBotCallbackAnswer(in *mtproto.TLMessagesSetBotCallbackAnswer) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() <= 0 || in.GetCacheTime() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var result struct {
		Value *bool `json:"value"`
	}
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.setBotCallbackAnswer", businessControlQueryKey(in.GetQueryId()), &result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := c.requireBotCaller(c.MD.GetUserId()); err != nil {
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
	if err := c.callBusinessProvider("messages.setBotCallbackAnswer", map[string]any{
		"alert": in.GetAlert(), "query_id": in.GetQueryId(), "message": in.GetMessage(), "url": in.GetUrl(), "cache_time": in.GetCacheTime(),
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

func (c *ApiFullCore) MessagesSendBotRequestedPeer(in *mtproto.TLMessagesSendBotRequestedPeer) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || (len(in.GetRequestedPeers()) == 0 && in.GetRequestedPeer() == nil) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.Updates)
	msgID := in.GetMsgId_INT32()
	if msgID == 0 && in.GetMsgId_FLAGINT32() != nil {
		msgID = in.GetMsgId_FLAGINT32().GetValue()
	}
	scopeKey := ""
	if value := in.GetWebappReqId(); value != nil {
		scopeKey = strings.TrimSpace(value.GetValue())
	}
	if scopeKey == "" {
		scopeKey = fmt.Sprintf("%s:msg:%d:button:%d", businessControlPeerKey(c.MD.GetUserId(), in.GetPeer()), msgID, in.GetButtonId())
	}
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.sendBotRequestedPeer", scopeKey, result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := c.validateInlineQueryPeer(c.MD.GetUserId(), in.GetPeer()); err != nil {
			return nil, err
		}
		for _, peer := range in.GetRequestedPeers() {
			if err := c.validateInlineQueryPeer(c.MD.GetUserId(), peer); err != nil {
				return nil, err
			}
		}
		if in.GetRequestedPeer() != nil {
			if err := c.validateInlineQueryPeer(c.MD.GetUserId(), in.GetRequestedPeer()); err != nil {
				return nil, err
			}
		}
		if err := validateUpdatesResult(result); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.sendBotRequestedPeer", map[string]any{
		"scope_key": scopeKey, "peer": in.GetPeer(), "msg_id_flag": in.GetMsgId_FLAGINT32(), "webapp_req_id": in.GetWebappReqId(), "button_id": in.GetButtonId(), "requested_peers": in.GetRequestedPeers(), "msg_id": msgID, "requested_peer": in.GetRequestedPeer(),
	}, result); err != nil {
		return nil, err
	}
	if err := validateUpdatesResult(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ApiFullCore) MessagesSetBotGuestChatResult(in *mtproto.TLMessagesSetBotGuestChatResult) (*mtproto.InputBotInlineMessageID, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() <= 0 || in.GetResult() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.InputBotInlineMessageID)
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.setBotGuestChatResult", businessControlQueryKey(in.GetQueryId()), result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := c.requireBotCaller(c.MD.GetUserId()); err != nil {
			return nil, err
		}
		if err := validateInputBotInlineMessageID(result); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.setBotGuestChatResult", map[string]any{
		"query_id": in.GetQueryId(), "result": in.GetResult(),
	}, result); err != nil {
		return nil, err
	}
	if err := validateInputBotInlineMessageID(result); err != nil {
		return nil, err
	}
	return result, nil
}

func newInlineQueryID() (int64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	id := int64(binary.BigEndian.Uint64(raw[:]) & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id, nil
}

func (c *ApiFullCore) verifyInlineBot(input *mtproto.InputUser) (int64, error) {
	if input == nil || input.GetPredicateName() != mtproto.Predicate_inputUser || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: input.GetUserId()})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return 0, mtproto.ErrBotInvalid
	}
	if profile.GetUser().GetAccessHash() != input.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return input.GetUserId(), nil
}

func (c *ApiFullCore) verifyInlineBotCaller(userID int64) error {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return mtproto.ErrBotInvalid
	}
	return nil
}

func decodeInlineBotResults(raw []json.RawMessage) ([]*mtproto.BotInlineResult, error) {
	results := make([]*mtproto.BotInlineResult, 0, len(raw))
	for _, item := range raw {
		var input mtproto.InputBotInlineResult
		if err := json.Unmarshal(item, &input); err != nil {
			return nil, mtproto.ErrInternalServerError
		}
		result, err := inputInlineResultToOutput(&input)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func inputInlineResultToOutput(input *mtproto.InputBotInlineResult) (*mtproto.BotInlineResult, error) {
	if input == nil || strings.TrimSpace(input.GetId()) == "" || strings.TrimSpace(input.GetType()) == "" {
		return nil, mtproto.ErrInternalServerError
	}
	if err := validateInlineResultPredicate(input.GetPredicateName()); err != nil {
		return nil, err
	}
	if !validInlineResultType(input.GetType()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if input.GetPredicateName() == mtproto.Predicate_inputBotInlineResultPhoto && input.GetPhoto() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if input.GetPredicateName() == mtproto.Predicate_inputBotInlineResultDocument && input.GetDocument() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if input.GetPredicateName() == mtproto.Predicate_inputBotInlineResultGame && strings.TrimSpace(input.GetShortName()) == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if input.GetSendMessage() != nil {
		if err := validateInlineMessagePredicate(input.GetSendMessage().GetPredicateName()); err != nil {
			return nil, err
		}
	}
	result := &mtproto.BotInlineResult{
		PredicateName: mtproto.Predicate_botInlineResult,
		Id:            input.GetId(),
		Type:          input.GetType(),
		Title:         input.GetTitle(),
		Description:   input.GetDescription(),
		Url:           input.GetUrl(),
	}
	if input.GetThumb() != nil {
		result.Thumb = inputWebDocumentToOutput(input.GetThumb())
	}
	if input.GetContent() != nil {
		result.Content = inputWebDocumentToOutput(input.GetContent())
	}
	if input.GetSendMessage() != nil {
		result.SendMessage = inputInlineMessageToOutput(input.GetSendMessage())
	}
	if input.GetPhoto() != nil {
		result.Photo = inputPhotoToOutput(input.GetPhoto())
		return mtproto.MakeTLBotInlineMediaResult(result).To_BotInlineResult(), nil
	}
	if input.GetDocument() != nil {
		result.Document = inputDocumentToOutput(input.GetDocument())
		return mtproto.MakeTLBotInlineMediaResult(result).To_BotInlineResult(), nil
	}
	return mtproto.MakeTLBotInlineResult(result).To_BotInlineResult(), nil
}

func validInlineResultType(value string) bool {
	switch strings.TrimSpace(value) {
	case "article", "photo", "gif", "mpeg4_gif", "video", "audio", "voice", "document",
		"location", "venue", "contact", "game", "invoice", "sticker":
		return true
	default:
		return false
	}
}

func validateInlineResultSwitches(switchPM *mtproto.InlineBotSwitchPM, switchWebview *mtproto.InlineBotWebView) error {
	if switchPM != nil {
		if utf8.RuneCountInString(strings.TrimSpace(switchPM.GetText())) == 0 || utf8.RuneCountInString(switchPM.GetText()) > 64 || !validInlineStartParameter(switchPM.GetStartParam()) {
			return mtproto.ErrInputRequestInvalid
		}
	}
	if switchWebview != nil {
		if utf8.RuneCountInString(strings.TrimSpace(switchWebview.GetText())) == 0 || utf8.RuneCountInString(switchWebview.GetText()) > 64 {
			return mtproto.ErrInputRequestInvalid
		}
		parsed, err := url.ParseRequestURI(strings.TrimSpace(switchWebview.GetUrl()))
		if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
			return mtproto.ErrInputRequestInvalid
		}
	}
	if switchPM != nil && switchWebview != nil {
		return mtproto.ErrInputRequestInvalid
	}
	return nil
}

func validInlineStartParameter(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validateInlineResultPredicate(predicate string) error {
	switch predicate {
	case "", mtproto.Predicate_inputBotInlineResult:
		return nil
	case mtproto.Predicate_inputBotInlineResultPhoto,
		mtproto.Predicate_inputBotInlineResultDocument,
		mtproto.Predicate_inputBotInlineResultGame:
		return nil
	default:
		return mtproto.ErrInputConstructorInvalid
	}
}

func validateInlineMessagePredicate(predicate string) error {
	switch predicate {
	case "", mtproto.Predicate_inputBotInlineMessageMediaAuto,
		mtproto.Predicate_inputBotInlineMessageText,
		mtproto.Predicate_inputBotInlineMessageMediaGeo,
		mtproto.Predicate_inputBotInlineMessageMediaVenue,
		mtproto.Predicate_inputBotInlineMessageMediaContact,
		mtproto.Predicate_inputBotInlineMessageMediaWebPage:
		return nil
	case mtproto.Predicate_inputBotInlineMessageGame,
		mtproto.Predicate_inputBotInlineMessageMediaInvoice,
		mtproto.Predicate_inputBotInlineMessageRichMessage:
		return nil
	default:
		return mtproto.ErrInputConstructorInvalid
	}
}

func inputWebDocumentToOutput(input *mtproto.InputWebDocument) *mtproto.WebDocument {
	if input == nil {
		return nil
	}
	return &mtproto.WebDocument{
		PredicateName: mtproto.Predicate_webDocument,
		Url:           input.GetUrl(),
		Size2:         input.GetSize2(),
		MimeType:      input.GetMimeType(),
		Attributes:    input.GetAttributes(),
	}
}

func inputInlineMessageToOutput(input *mtproto.InputBotInlineMessage) *mtproto.BotInlineMessage {
	if input == nil {
		return nil
	}
	name := inlineOutputMessagePredicate(input.GetPredicateName())
	result := &mtproto.BotInlineMessage{
		PredicateName:               name,
		InvertMedia:                 input.GetInvertMedia(),
		Message:                     input.GetMessage(),
		Entities:                    input.GetEntities(),
		ReplyMarkup:                 input.GetReplyMarkup(),
		NoWebpage:                   input.GetNoWebpage(),
		Geo:                         inputGeoPointToOutput(input.GetGeoPoint()),
		Heading:                     input.GetHeading(),
		Period:                      input.GetPeriod(),
		ProximityNotificationRadius: input.GetProximityNotificationRadius(),
		Title:                       input.GetTitle(),
		Address:                     input.GetAddress(),
		Provider:                    input.GetProvider(),
		VenueId:                     input.GetVenueId(),
		VenueType:                   input.GetVenueType(),
		PhoneNumber:                 input.GetPhoneNumber(),
		FirstName:                   input.GetFirstName(),
		LastName:                    input.GetLastName(),
		Vcard:                       input.GetVcard(),
		Description:                 input.GetDescription(),
		ForceLargeMedia:             input.GetForceLargeMedia(),
		ForceSmallMedia:             input.GetForceSmallMedia(),
		Url:                         input.GetUrl(),
	}
	if input.GetPhoto() != nil {
		result.Photo = inputWebDocumentToOutput(input.GetPhoto())
	}
	if invoice := input.GetInvoice(); invoice != nil {
		result.ShippingAddressRequested = invoice.GetShippingAddressRequested()
		result.Test = invoice.GetTest()
		result.Currency = invoice.GetCurrency()
		result.TotalAmount = inlineInvoiceTotal(invoice)
	}
	if input.GetRichMessage() != nil {
		result.RichMessage = inputRichMessageToOutput(input.GetRichMessage())
	}
	if name == mtproto.Predicate_botInlineMessageMediaInvoice {
		return mtproto.MakeTLBotInlineMessageMediaInvoice(result).To_BotInlineMessage()
	}
	if name == mtproto.Predicate_botInlineMessageRichMessage {
		return mtproto.MakeTLBotInlineMessageRichMessage(result).To_BotInlineMessage()
	}
	if name == mtproto.Predicate_botInlineMessageMediaWebPage {
		return mtproto.MakeTLBotInlineMessageMediaWebPage(result).To_BotInlineMessage()
	}
	if name == mtproto.Predicate_botInlineMessageMediaGeo {
		return mtproto.MakeTLBotInlineMessageMediaGeo(result).To_BotInlineMessage()
	}
	if name == mtproto.Predicate_botInlineMessageMediaVenue {
		return mtproto.MakeTLBotInlineMessageMediaVenue(result).To_BotInlineMessage()
	}
	if name == mtproto.Predicate_botInlineMessageMediaContact {
		return mtproto.MakeTLBotInlineMessageMediaContact(result).To_BotInlineMessage()
	}
	if name == mtproto.Predicate_botInlineMessageText {
		return mtproto.MakeTLBotInlineMessageText(result).To_BotInlineMessage()
	}
	return mtproto.MakeTLBotInlineMessageMediaAuto(result).To_BotInlineMessage()
}

func inlineOutputMessagePredicate(predicate string) string {
	switch predicate {
	case mtproto.Predicate_inputBotInlineMessageMediaAuto:
		return mtproto.Predicate_botInlineMessageMediaAuto
	case mtproto.Predicate_inputBotInlineMessageText:
		return mtproto.Predicate_botInlineMessageText
	case mtproto.Predicate_inputBotInlineMessageMediaGeo:
		return mtproto.Predicate_botInlineMessageMediaGeo
	case mtproto.Predicate_inputBotInlineMessageMediaVenue:
		return mtproto.Predicate_botInlineMessageMediaVenue
	case mtproto.Predicate_inputBotInlineMessageMediaContact:
		return mtproto.Predicate_botInlineMessageMediaContact
	case mtproto.Predicate_inputBotInlineMessageMediaInvoice:
		return mtproto.Predicate_botInlineMessageMediaInvoice
	case mtproto.Predicate_inputBotInlineMessageMediaWebPage:
		return mtproto.Predicate_botInlineMessageMediaWebPage
	case mtproto.Predicate_inputBotInlineMessageRichMessage:
		return mtproto.Predicate_botInlineMessageRichMessage
	case mtproto.Predicate_inputBotInlineMessageGame:
		return mtproto.Predicate_botInlineMessageMediaAuto
	default:
		return mtproto.Predicate_botInlineMessageText
	}
}

func inputPhotoToOutput(input *mtproto.InputPhoto) *mtproto.Photo {
	if input == nil {
		return nil
	}
	return mtproto.MakeTLPhoto(&mtproto.Photo{Id: input.GetId(), AccessHash: input.GetAccessHash(), FileReference: input.GetFileReference()}).To_Photo()
}

func inputDocumentToOutput(input *mtproto.InputDocument) *mtproto.Document {
	if input == nil {
		return nil
	}
	return mtproto.MakeTLDocument(&mtproto.Document{Id: input.GetId(), AccessHash: input.GetAccessHash(), FileReference: input.GetFileReference()}).To_Document()
}

func inputRichMessageToOutput(input *mtproto.InputRichMessage) *mtproto.RichMessage {
	if input == nil {
		return nil
	}
	photos := make([]*mtproto.Photo, 0, len(input.GetPhotos()))
	for _, photo := range input.GetPhotos() {
		if converted := inputPhotoToOutput(photo); converted != nil {
			photos = append(photos, converted)
		}
	}
	documents := make([]*mtproto.Document, 0, len(input.GetDocuments()))
	for _, document := range input.GetDocuments() {
		if converted := inputDocumentToOutput(document); converted != nil {
			documents = append(documents, converted)
		}
	}
	return mtproto.MakeTLRichMessage(&mtproto.RichMessage{
		Blocks: blocksOrEmpty(input.GetBlocks()), Photos: photos, Documents: documents, Rtl: input.GetRtl(),
	}).To_RichMessage()
}

func blocksOrEmpty(blocks []*mtproto.PageBlock) []*mtproto.PageBlock {
	if blocks == nil {
		return []*mtproto.PageBlock{}
	}
	return blocks
}

func inlineInvoiceTotal(invoice *mtproto.Invoice) int64 {
	var total int64
	if invoice == nil {
		return 0
	}
	for _, price := range invoice.GetPrices() {
		if price != nil {
			total += price.GetAmount()
		}
	}
	return total
}

func (c *ApiFullCore) inlineInputToMessageMedia(result *mtproto.InputBotInlineResult, message *mtproto.InputBotInlineMessage) (*mtproto.MessageMedia, *mtproto.RichMessage, error) {
	if result == nil || message == nil {
		return nil, nil, mtproto.ErrInputRequestInvalid
	}
	switch result.GetPredicateName() {
	case mtproto.Predicate_inputBotInlineResultPhoto:
		if result.GetPhoto() == nil {
			return nil, nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{Photo_FLAGPHOTO: inputPhotoToOutput(result.GetPhoto())}).To_MessageMedia(), nil, nil
	case mtproto.Predicate_inputBotInlineResultDocument:
		if result.GetDocument() == nil {
			return nil, nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{Document: inputDocumentToOutput(result.GetDocument())}).To_MessageMedia(), nil, nil
	case mtproto.Predicate_inputBotInlineResultGame:
		if strings.TrimSpace(result.GetShortName()) == "" {
			return nil, nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLMessageMediaGame(&mtproto.MessageMedia{Game: &mtproto.Game{ShortName: result.GetShortName()}}).To_MessageMedia(), nil, nil
	}
	switch message.GetPredicateName() {
	case mtproto.Predicate_inputBotInlineMessageMediaGeo:
		geo, err := inlineInputGeoToMessageMedia(message)
		if err != nil {
			return nil, nil, err
		}
		return geo, nil, nil
	case mtproto.Predicate_inputBotInlineMessageMediaVenue:
		geo, err := inlineMessageGeoPoint(message.GetGeoPoint())
		if err != nil || strings.TrimSpace(message.GetTitle()) == "" || strings.TrimSpace(message.GetAddress()) == "" {
			return nil, nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLMessageMediaVenue(&mtproto.MessageMedia{
			Geo:       geo,
			Title:     message.GetTitle(),
			Address:   message.GetAddress(),
			Provider:  message.GetProvider(),
			VenueId:   message.GetVenueId(),
			VenueType: message.GetVenueType(),
		}).To_MessageMedia(), nil, nil
	case mtproto.Predicate_inputBotInlineMessageMediaContact:
		if strings.TrimSpace(message.GetPhoneNumber()) == "" || strings.TrimSpace(message.GetFirstName()) == "" {
			return nil, nil, mtproto.ErrInputRequestInvalid
		}
		return mtproto.MakeTLMessageMediaContact(&mtproto.MessageMedia{
			PhoneNumber: message.GetPhoneNumber(),
			FirstName:   message.GetFirstName(),
			LastName:    message.GetLastName(),
			Vcard:       message.GetVcard(),
		}).To_MessageMedia(), nil, nil
	case mtproto.Predicate_inputBotInlineMessageMediaWebPage:
		return nil, nil, mtproto.ErrMethodNotImpl
	}
	if message.GetPredicateName() == mtproto.Predicate_inputBotInlineMessageMediaInvoice {
		media := &mtproto.InputMedia{
			Title: message.GetTitle(), Description: message.GetDescription(), Photo: message.GetPhoto(), Invoice: message.GetInvoice(),
			Payload_BYTES: message.GetPayload(), Provider_STRING: message.GetProvider(), ProviderData: message.GetProviderData(),
		}
		if media.GetInvoice() == nil {
			return nil, nil, mtproto.ErrInputRequestInvalid
		}
		if media.GetProviderData() == nil {
			media.ProviderData = mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: "{}"}).To_DataJSON()
		}
		if message.GetUrl() != "" {
			media.StartParam = wrapperspb.String(message.GetUrl())
		}
		invoiceMedia, err := paymentinvoice.FromInputMedia(mtproto.MakeTLInputMediaInvoice(media).To_InputMedia())
		return invoiceMedia, nil, err
	}
	if message.GetPredicateName() == mtproto.Predicate_inputBotInlineMessageRichMessage {
		return nil, inputRichMessageToOutput(message.GetRichMessage()), nil
	}
	if content := result.GetContent(); content != nil {
		return c.inlineRemoteMedia(result.GetType(), content)
	}
	// A result without a media constructor is a text-only inline message.
	// Returning nil here is intentional; the caller still rejects a message
	// that has neither text nor media.
	return nil, nil, nil
}

const (
	inlineRemoteMediaMaxBytes = 20 << 20
	inlineRemoteMediaPartSize = 512 << 10
)

func (c *ApiFullCore) inlineRemoteMedia(resultType string, content *mtproto.InputWebDocument) (*mtproto.MessageMedia, *mtproto.RichMessage, error) {
	if content == nil || strings.TrimSpace(content.GetUrl()) == "" {
		return nil, nil, mtproto.ErrInputRequestInvalid
	}
	remoteURL, err := parseInlineRemoteURL(content.GetUrl())
	if err != nil {
		return nil, nil, mtproto.ErrMediaInvalid
	}
	data, responseType, name, err := fetchInlineRemoteMedia(c.ctx, remoteURL)
	if err != nil {
		return nil, nil, err
	}
	mimeType := responseType
	if mimeType == "" {
		mimeType = strings.TrimSpace(content.GetMimeType())
	}
	if parsed, _, parseErr := mime.ParseMediaType(mimeType); parseErr == nil {
		mimeType = parsed
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if resultType == "photo" {
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			return nil, nil, mtproto.ErrMediaInvalid
		}
		photo, uploadErr := c.uploadInlinePhoto(data, name)
		if uploadErr != nil {
			return nil, nil, uploadErr
		}
		if photo == nil || photo.GetId() <= 0 {
			return nil, nil, mtproto.ErrMediaInvalid
		}
		return mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{Photo_FLAGPHOTO: photo}).To_MessageMedia(), nil, nil
	}
	attributes := content.GetAttributes()
	media := mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		MimeType:   mimeType,
		Attributes: attributes,
		ForceFile:  resultType == "document",
	}).To_InputMedia()
	document, uploadErr := c.uploadInlineDocument(data, name, media)
	if uploadErr != nil {
		return nil, nil, uploadErr
	}
	if document == nil || document.GetId() <= 0 {
		return nil, nil, mtproto.ErrMediaInvalid
	}
	return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{Document: document}).To_MessageMedia(), nil, nil
}

func parseInlineRemoteURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("inline remote URL is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return nil, errors.New("inline remote URL scheme is invalid")
	}
	if err = rejectPrivateInlineHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func rejectPrivateInlineHost(host string) error {
	if strings.TrimSpace(host) == "" {
		return errors.New("inline remote host is empty")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return errors.New("inline remote host cannot be resolved")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("inline remote host is not public")
		}
	}
	return nil
}

func fetchInlineRemoteMedia(parent context.Context, remoteURL *url.URL) ([]byte, string, string, error) {
	if remoteURL == nil {
		return nil, "", "", mtproto.ErrMediaInvalid
	}
	requestContext, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	transport := &http.Transport{
		// Inline URLs are supplied by third-party bots. Do not route them
		// through an ambient proxy, which could bypass the private-address
		// check in DialContext.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil {
				return nil, splitErr
			}
			if err := rejectPrivateInlineHost(host); err != nil {
				return nil, err
			}
			ips, err := net.LookupIP(host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("inline remote host cannot be resolved")
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if len(req.URL.String()) > 2048 {
				return errors.New("inline remote redirect is too long")
			}
			return rejectInlineRemoteRedirect(req.URL)
		},
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		return nil, "", "", mtproto.ErrMediaInvalid
	}
	request.Header.Set("Accept", "image/*,audio/*,video/*,application/octet-stream;q=0.8")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", "", mtproto.ErrMediaInvalid
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", "", mtproto.ErrMediaInvalid
	}
	if response.ContentLength > inlineRemoteMediaMaxBytes {
		return nil, "", "", mtproto.ErrFilePartsInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, inlineRemoteMediaMaxBytes+1))
	if err != nil || len(data) == 0 {
		return nil, "", "", mtproto.ErrMediaInvalid
	}
	if len(data) > inlineRemoteMediaMaxBytes {
		return nil, "", "", mtproto.ErrFilePartsInvalid
	}
	name := path.Base(response.Request.URL.Path)
	if name == "." || name == "/" || name == "" {
		name = "inline-media"
	}
	return data, response.Header.Get("Content-Type"), name, nil
}

func rejectInlineRemoteRedirect(target *url.URL) error {
	if target == nil || target.User != nil || (!strings.EqualFold(target.Scheme, "http") && !strings.EqualFold(target.Scheme, "https")) {
		return errors.New("inline remote redirect is invalid")
	}
	return rejectPrivateInlineHost(target.Hostname())
}

func (c *ApiFullCore) uploadInlinePhoto(data []byte, name string) (*mtproto.Photo, error) {
	d := c.apifullDao()
	uid := c.MD.GetUserId()
	if d == nil || d.DfsClient == nil || uid <= 0 {
		return nil, mtproto.ErrMethodNotImpl
	}
	input, err := c.writeInlineRemoteFile(data, name)
	if err != nil {
		return nil, err
	}
	return d.DfsUploadPhotoFileV2(callContext(c), &dfspb.TLDfsUploadPhotoFileV2{Creator: uid, File: input})
}

func (c *ApiFullCore) uploadInlineDocument(data []byte, name string, media *mtproto.InputMedia) (*mtproto.Document, error) {
	d := c.apifullDao()
	uid := c.MD.GetUserId()
	if d == nil || d.DfsClient == nil || uid <= 0 || media == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	input, err := c.writeInlineRemoteFile(data, name)
	if err != nil {
		return nil, err
	}
	media.File = input
	return d.DfsUploadDocumentFileV2(callContext(c), &dfspb.TLDfsUploadDocumentFileV2{Creator: uid, Media: media})
}

func (c *ApiFullCore) writeInlineRemoteFile(data []byte, name string) (*mtproto.InputFile, error) {
	d := c.apifullDao()
	uid := c.MD.GetUserId()
	if d == nil || d.DfsClient == nil || uid <= 0 || len(data) == 0 || len(data) > inlineRemoteMediaMaxBytes {
		return nil, mtproto.ErrMediaInvalid
	}
	parts := (len(data) + inlineRemoteMediaPartSize - 1) / inlineRemoteMediaPartSize
	if parts <= 0 || parts > 3000 {
		return nil, mtproto.ErrFilePartsInvalid
	}
	fileID, err := newInlineQueryID()
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	for part := 0; part < parts; part++ {
		start := part * inlineRemoteMediaPartSize
		end := start + inlineRemoteMediaPartSize
		if end > len(data) {
			end = len(data)
		}
		ok, writeErr := d.DfsWriteFilePartData(callContext(c), &dfspb.TLDfsWriteFilePartData{
			Creator: uid, FileId: fileID, FilePart: int32(part), Bytes: data[start:end],
			Big: len(data) > 10<<20, FileTotalParts: wrapperspb.Int32(int32(parts)),
		})
		if writeErr != nil || !mtproto.FromBool(ok) {
			if writeErr != nil {
				return nil, writeErr
			}
			return nil, mtproto.ErrMediaInvalid
		}
	}
	input := &mtproto.InputFile{Id_INT64: fileID, Parts: int32(parts), Name: name}
	if len(data) > 10<<20 {
		return mtproto.MakeTLInputFileBig(input).To_InputFile(), nil
	}
	return mtproto.MakeTLInputFile(input).To_InputFile(), nil
}

func inlineInputGeoToMessageMedia(message *mtproto.InputBotInlineMessage) (*mtproto.MessageMedia, error) {
	geo, err := inlineMessageGeoPoint(message.GetGeoPoint())
	if err != nil {
		return nil, err
	}
	heading := message.GetHeading()
	if heading != nil && (heading.GetValue() < 1 || heading.GetValue() > 360) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	proximity := message.GetProximityNotificationRadius()
	if proximity != nil && proximity.GetValue() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	period := message.GetPeriod()
	if period != nil && period.GetValue() < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if period != nil && period.GetValue() > 0 {
		return mtproto.MakeTLMessageMediaGeoLive(&mtproto.MessageMedia{
			Geo:                         geo,
			Period:                      period.GetValue(),
			Heading:                     heading,
			ProximityNotificationRadius: proximity,
		}).To_MessageMedia(), nil
	}
	if heading != nil || proximity != nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return mtproto.MakeTLMessageMediaGeo(&mtproto.MessageMedia{Geo: geo}).To_MessageMedia(), nil
}

func inlineMessageGeoPoint(input *mtproto.InputGeoPoint) (*mtproto.GeoPoint, error) {
	if input == nil || math.IsNaN(input.GetLat()) || math.IsNaN(input.GetLong()) ||
		math.IsInf(input.GetLat(), 0) || math.IsInf(input.GetLong(), 0) ||
		input.GetLat() < -90 || input.GetLat() > 90 || input.GetLong() < -180 || input.GetLong() > 180 {
		return nil, mtproto.ErrGeoPointInvalid
	}
	if accuracy := input.GetAccuracyRadius(); accuracy != nil && accuracy.GetValue() < 0 {
		return nil, mtproto.ErrGeoPointInvalid
	}
	return inputGeoPointToOutput(input), nil
}

func inputGeoPointToOutput(input *mtproto.InputGeoPoint) *mtproto.GeoPoint {
	if input == nil {
		return nil
	}
	return &mtproto.GeoPoint{
		PredicateName:  mtproto.Predicate_geoPoint,
		Long:           input.GetLong(),
		Lat:            input.GetLat(),
		AccuracyRadius: input.GetAccuracyRadius(),
	}
}
