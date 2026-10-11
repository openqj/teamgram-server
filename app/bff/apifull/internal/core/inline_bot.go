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
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
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
	return mtproto.MakeTLMessagesBotResults(&mtproto.Messages_BotResults{
		Gallery:    query.Gallery,
		QueryId:    query.QueryID,
		NextOffset: nextOffset,
		Results:    results,
		CacheTime:  query.CacheTime,
		Users:      []*mtproto.User{},
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
	for _, result := range in.GetResults() {
		if result == nil || strings.TrimSpace(result.GetId()) == "" || strings.TrimSpace(result.GetType()) == "" {
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
		if result.GetPhoto() != nil || result.GetDocument() != nil {
			// InputPhoto/InputDocument and InputRichMessage require their
			// canonical media providers before they can be exposed as output
			// constructors. Do not silently drop those fields.
			return nil, mtproto.ErrMethodNotImpl
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
	}
	// setInlineBotResults carries only query_id. Resolve the pending query
	// atomically across bot partitions instead of treating the authenticated
	// caller as the bot identity.
	if err = persist.SetInlineBotResultsByQuery(in.GetQueryId(), in.GetGallery(), in.GetPrivate(), in.GetCacheTime(), nextOffset, encoded); err != nil {
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
	if selected.GetPhoto() != nil || selected.GetDocument() != nil || selected.GetSendMessage() == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	send := selected.GetSendMessage()
	if err = validateInlineMessagePredicate(send.GetPredicateName()); err != nil {
		return nil, err
	}
	// The message service owns media hydration and inline callback state. This
	// boundary currently supports only text/media-auto result messages; media
	// and rich-message constructors stay fail-closed until their providers are
	// available there.
	if send.GetPredicateName() != "" &&
		send.GetPredicateName() != mtproto.Predicate_inputBotInlineMessageText &&
		send.GetPredicateName() != mtproto.Predicate_inputBotInlineMessageMediaAuto {
		return nil, mtproto.ErrMethodNotImpl
	}
	if strings.TrimSpace(send.GetMessage()) == "" {
		return nil, mtproto.ErrMessageEmpty
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
	if in == nil || in.GetId() == nil || (in.GetMessage() == nil && in.GetMedia() == nil && in.GetRichMessage() == nil) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var result struct {
		Value *bool `json:"value"`
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
	if err := c.callBusinessProvider("messages.sendBotRequestedPeer", map[string]any{
		"peer": in.GetPeer(), "msg_id_flag": in.GetMsgId_FLAGINT32(), "webapp_req_id": in.GetWebappReqId(), "button_id": in.GetButtonId(), "requested_peers": in.GetRequestedPeers(), "msg_id": in.GetMsgId_INT32(), "requested_peer": in.GetRequestedPeer(),
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
	if input.GetPhoto() != nil || input.GetDocument() != nil {
		return nil, mtproto.ErrMethodNotImpl
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
	return result, nil
}

func validateInlineResultPredicate(predicate string) error {
	switch predicate {
	case "", mtproto.Predicate_inputBotInlineResult:
		return nil
	case mtproto.Predicate_inputBotInlineResultPhoto,
		mtproto.Predicate_inputBotInlineResultDocument,
		mtproto.Predicate_inputBotInlineResultGame:
		return mtproto.ErrMethodNotImpl
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
		return mtproto.ErrMethodNotImpl
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
	name := input.GetPredicateName()
	if strings.HasPrefix(name, "input") {
		name = strings.TrimPrefix(name, "input")
		if name != "" {
			name = strings.ToLower(name[:1]) + name[1:]
		}
	}
	if name == "" {
		name = mtproto.Predicate_botInlineMessageText
	}
	return &mtproto.BotInlineMessage{
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
