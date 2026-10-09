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
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func miniAppKey(userID int64) string {
	return "miniapp:" + strconv.FormatInt(userID, 10)
}

func miniAppPut(userID int64, method string, in any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return persist.Default.Set(miniAppKey(userID)+":"+method, string(b))
}

func miniBotID(input *mtproto.InputUser) (int64, error) {
	if input == nil {
		return 0, mtproto.ErrInputRequestInvalid
	}
	if input.GetPredicateName() != mtproto.Predicate_inputUser || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return input.GetUserId(), nil
}

func miniUserID(caller int64, input *mtproto.InputUser) (int64, error) {
	if input == nil {
		return 0, mtproto.ErrInputRequestInvalid
	}
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		return caller, nil
	case mtproto.Predicate_inputUser:
		if input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return input.GetUserId(), nil
	default:
		return 0, mtproto.ErrInputConstructorInvalid
	}
}

func (c *ApiFullCore) verifyMiniBot(input *mtproto.InputUser) (int64, error) {
	botID, err := miniBotID(input)
	if err != nil {
		return 0, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: botID})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return 0, mtproto.ErrBotInvalid
	}
	if profile.GetUser().GetAccessHash() != input.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return botID, nil
}

func (c *ApiFullCore) verifyMiniUser(caller int64, input *mtproto.InputUser) (int64, error) {
	userID, err := miniUserID(caller, input)
	if err != nil {
		return 0, err
	}
	if input.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		return userID, nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.Deleted() {
		return 0, mtproto.ErrUserIdInvalid
	}
	if profile.GetUser().GetAccessHash() != input.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return userID, nil
}

func miniWebViewURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", mtproto.ErrInputRequestInvalid
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", mtproto.ErrWebdocumentUrlInvalid
	}
	return raw, nil
}

func miniRequestID() (string, int64, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", 0, err
	}
	queryID := int64(0)
	for _, b := range raw[:8] {
		queryID = (queryID << 8) | int64(b)
	}
	queryID &= 0x7fffffffffffffff
	if queryID == 0 {
		queryID = 1
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), queryID, nil
}

func miniWebViewPayload(kind, rawURL, platform string, queryID int64, request any) (string, error) {
	payload := map[string]any{
		"kind":       kind,
		"url":        rawURL,
		"platform":   platform,
		"query_id":   strconv.FormatInt(queryID, 10),
		"request":    request,
		"created_at": time.Now().UTC().Unix(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *ApiFullCore) requestMiniWebView(uid int64, bot *mtproto.InputUser, rawURL, kind, platform string, fullsize, fullscreen bool, request any) (*mtproto.WebViewResult, error) {
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := c.verifyMiniBot(bot)
	if err != nil {
		return nil, err
	}
	rawURL, err = miniWebViewURL(rawURL)
	if err != nil {
		return nil, err
	}
	requestID, queryID, err := miniRequestID()
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	payload, err := miniWebViewPayload(kind, rawURL, platform, queryID, request)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = persist.PutWebViewRequest(persist.WebViewRequest{
		RequestID: requestID,
		UserID:    uid,
		BotID:     botID,
		Kind:      kind,
		Payload:   payload,
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLWebViewResultUrl(&mtproto.WebViewResult{
		Fullsize:          fullsize,
		Fullscreen:        fullscreen,
		SameOrigin:        false,
		QueryId_FLAGINT64: wrapperspb.Int64(queryID),
		Url:               rawURL,
	}).To_WebViewResult(), nil
}

func (c *ApiFullCore) requestMiniSimpleWebView(uid int64, bot *mtproto.InputUser, rawURL, kind, platform string, request any) (*mtproto.SimpleWebViewResult, error) {
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := c.verifyMiniBot(bot)
	if err != nil {
		return nil, err
	}
	rawURL, err = miniWebViewURL(rawURL)
	if err != nil {
		return nil, err
	}
	requestID, queryID, err := miniRequestID()
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	payload, err := miniWebViewPayload(kind, rawURL, platform, queryID, request)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = persist.PutWebViewRequest(persist.WebViewRequest{
		RequestID: requestID,
		UserID:    uid,
		BotID:     botID,
		Kind:      kind,
		Payload:   payload,
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLSimpleWebViewResultUrl(&mtproto.SimpleWebViewResult{Url: rawURL}).To_SimpleWebViewResult(), nil
}

// RPCMiniBotAppsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesRequestWebView(in *mtproto.TLMessagesRequestWebView) (*mtproto.WebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetUrl() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.requestMiniWebView(uid, in.GetBot(), in.GetUrl().GetValue(), "requestWebView", in.GetPlatform(), !in.GetCompact(), in.GetFullscreen(), in)
}

func (c *ApiFullCore) MessagesProlongWebView(in *mtproto.TLMessagesProlongWebView) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetQueryId() <= 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	prolonged, err := persist.ProlongWebViewQuery(uid, in.GetQueryId(), 5*time.Minute)
	if err != nil {
		return nil, err
	}
	if !prolonged {
		return nil, mtproto.ErrQueryIdInvalid
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) MessagesRequestSimpleWebView413A3E73(in *mtproto.TLMessagesRequestSimpleWebView413A3E73) (*mtproto.WebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetUrl() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.requestMiniWebView(uid, in.GetBot(), in.GetUrl().GetValue(), "requestSimpleWebView", in.GetPlatform(), !in.GetCompact(), in.GetFullscreen(), in)
}

func (c *ApiFullCore) MessagesSendWebViewResultMessage(in *mtproto.TLMessagesSendWebViewResultMessage) (*mtproto.WebViewMessageSent, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesSendWebViewData(in *mtproto.TLMessagesSendWebViewData) (*mtproto.Updates, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesGetBotApp(in *mtproto.TLMessagesGetBotApp) (*mtproto.Messages_BotApp, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesRequestAppWebView53618BCE(in *mtproto.TLMessagesRequestAppWebView53618BCE) (*mtproto.WebViewResult, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesRequestChatJoinWebView(in *mtproto.TLMessagesRequestChatJoinWebView) (*mtproto.WebViewResult, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsCanSendMessage(in *mtproto.TLBotsCanSendMessage) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	allowed, err := persist.GetMiniBotPermission(uid, botID)
	if err != nil {
		return nil, err
	}
	if allowed {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (c *ApiFullCore) BotsAllowSendMessage(in *mtproto.TLBotsAllowSendMessage) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	if err = persist.SetMiniBotPermission(uid, botID, true); err != nil {
		return nil, err
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{}, Users: []*mtproto.User{}, Chats: []*mtproto.Chat{}, Date: int32(time.Now().Unix()),
	}).To_Updates(), nil
}

func (c *ApiFullCore) BotsInvokeWebViewCustomMethod(in *mtproto.TLBotsInvokeWebViewCustomMethod) (*mtproto.DataJSON, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsCheckDownloadFileParams(in *mtproto.TLBotsCheckDownloadFileParams) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsRequestWebViewButton(in *mtproto.TLBotsRequestWebViewButton) (*mtproto.Bots_RequestedButton, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	userID, err := c.verifyMiniUser(uid, in.GetUserId())
	if err != nil {
		return nil, err
	}
	button := in.GetButton()
	if button == nil || (button.GetPredicateName() != mtproto.Predicate_keyboardButtonWebView && button.GetPredicateName() != mtproto.Predicate_keyboardButtonSimpleWebView) {
		return nil, mtproto.ErrButtonDataInvalid
	}
	if _, err = miniWebViewURL(button.GetUrl()); err != nil {
		return nil, err
	}
	requestID, _, err := miniRequestID()
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	payload, err := json.Marshal(button)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = persist.PutWebViewRequest(persist.WebViewRequest{
		RequestID: requestID, UserID: uid, BotID: userID, Kind: "requestWebViewButton",
		Payload: string(payload), ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLBotsRequestedButton(&mtproto.Bots_RequestedButton{WebappReqId: requestID}).To_Bots_RequestedButton(), nil
}

func (c *ApiFullCore) BotsGetRequestedWebViewButton(in *mtproto.TLBotsGetRequestedWebViewButton) (*mtproto.KeyboardButton, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.GetWebappReqId()) == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	request, err := persist.GetWebViewRequest(uid, in.GetWebappReqId())
	if err != nil {
		return nil, err
	}
	if request == nil || request.Kind != "requestWebViewButton" || request.BotID != botID {
		return nil, mtproto.ErrQueryIdInvalid
	}
	var button mtproto.KeyboardButton
	if err = json.Unmarshal([]byte(request.Payload), &button); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	return &button, nil
}

func (c *ApiFullCore) MessagesRequestSimpleWebView1A46500A(in *mtproto.TLMessagesRequestSimpleWebView1A46500A) (*mtproto.SimpleWebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil || in.GetUrl() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.requestMiniSimpleWebView(uid, in.GetBot(), in.GetUrl().GetValue(), "requestSimpleWebView", in.GetPlatform(), in)
}

func (c *ApiFullCore) MessagesRequestAppWebView8C5A3B3C(in *mtproto.TLMessagesRequestAppWebView8C5A3B3C) (*mtproto.AppWebViewResult, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) MessagesRequestSimpleWebView299BEC8E(in *mtproto.TLMessagesRequestSimpleWebView299BEC8E) (*mtproto.SimpleWebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.requestMiniSimpleWebView(uid, in.GetBot(), in.GetUrl(), "requestSimpleWebView", in.GetPlatform(), in)
}

func (c *ApiFullCore) MessagesRequestSimpleWebView6ABB2F73(in *mtproto.TLMessagesRequestSimpleWebView6ABB2F73) (*mtproto.SimpleWebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return c.requestMiniSimpleWebView(uid, in.GetBot(), in.GetUrl(), "requestSimpleWebView", "", in)
}
