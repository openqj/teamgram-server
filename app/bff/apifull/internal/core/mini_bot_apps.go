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
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/botapi"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
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

// validateProviderBotInputUser checks the wire shape before a request reaches
// an external business provider. Tests and a few older clients leave the
// generated predicate marker empty, so the marker is only rejected when it is
// present and names a different constructor.
func validateProviderBotInputUser(input *mtproto.InputUser) error {
	if input == nil || input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
		return mtproto.ErrUserIdInvalid
	}
	if predicate := input.GetPredicateName(); predicate != "" && predicate != mtproto.Predicate_inputUser {
		return mtproto.ErrInputConstructorInvalid
	}
	return nil
}

func validateInputBotInlineMessageIDInput(input *mtproto.InputBotInlineMessageID) error {
	if input == nil || input.GetDcId() <= 0 || input.GetAccessHash() == 0 ||
		(input.GetId_INT64() == 0 && input.GetId_INT32() == 0) {
		return mtproto.ErrInputRequestInvalid
	}
	switch predicate := input.GetPredicateName(); predicate {
	case "", mtproto.Predicate_inputBotInlineMessageID, mtproto.Predicate_inputBotInlineMessageID64:
		return nil
	default:
		return mtproto.ErrInputConstructorInvalid
	}
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

func miniWebViewStablePayload(kind, rawURL string, queryID int64, request any) (string, error) {
	payload := map[string]any{
		"kind":     kind,
		"url":      rawURL,
		"query_id": strconv.FormatInt(queryID, 10),
		"request":  request,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func validateWebViewMessageSent(result *mtproto.WebViewMessageSent) error {
	if result == nil || result.GetPredicateName() != mtproto.Predicate_webViewMessageSent {
		return mtproto.ErrInternalServerError
	}
	if result.GetMsgId() != nil {
		id := result.GetMsgId()
		if id.GetPredicateName() == "" || id.GetDcId() <= 0 || (id.GetId_INT64() == 0 && id.GetId_INT32() == 0) {
			return mtproto.ErrInternalServerError
		}
	}
	return nil
}

func validateUpdatesResult(result *mtproto.Updates) error {
	if result == nil {
		return mtproto.ErrInternalServerError
	}
	switch result.GetPredicateName() {
	case mtproto.Predicate_updates, mtproto.Predicate_updatesCombined, mtproto.Predicate_updatesTooLong:
		return nil
	default:
		return mtproto.ErrInternalServerError
	}
}

func validateInputBotInlineMessageID(result *mtproto.InputBotInlineMessageID) error {
	if result == nil || (result.GetPredicateName() != mtproto.Predicate_inputBotInlineMessageID && result.GetPredicateName() != mtproto.Predicate_inputBotInlineMessageID64) || result.GetDcId() <= 0 || result.GetAccessHash() == 0 {
		return mtproto.ErrInternalServerError
	}
	if result.GetId_INT64() == 0 && result.GetId_INT32() == 0 {
		return mtproto.ErrInternalServerError
	}
	return nil
}

func validateInputBotApp(app *mtproto.InputBotApp) error {
	if app == nil || app.GetBotId() == nil {
		return mtproto.ErrInputRequestInvalid
	}
	if app.GetBotId().GetPredicateName() != mtproto.Predicate_inputUser || app.GetBotId().GetUserId() <= 0 || app.GetBotId().GetAccessHash() == 0 {
		return mtproto.ErrUserIdInvalid
	}
	switch app.GetPredicateName() {
	case mtproto.Predicate_inputBotAppID:
		if app.GetId() <= 0 || app.GetAccessHash() == 0 {
			return mtproto.ErrInputRequestInvalid
		}
	case mtproto.Predicate_inputBotAppShortName:
		if strings.TrimSpace(app.GetShortName()) == "" || len(app.GetShortName()) > 64 {
			return mtproto.ErrInputRequestInvalid
		}
	default:
		return mtproto.ErrInputConstructorInvalid
	}
	return nil
}

// validateMiniAppPeer checks the peer before it becomes part of a durable
// WebView scope. Explicit user and channel peers carry access hashes and must
// match the authoritative stores. Message-derived peers have no target hash,
// so their source shape and target existence are checked instead.
func (c *ApiFullCore) validateMiniAppPeer(callerID int64, peer *mtproto.InputPeer) error {
	if callerID <= 0 || peer == nil {
		return mtproto.ErrInputRequestInvalid
	}

	predicate := peer.GetPredicateName()
	if predicate == "" {
		switch {
		case peer.GetPeer() != nil || peer.GetMsgId() != 0:
			if peer.GetChannelId() > 0 {
				predicate = mtproto.Predicate_inputPeerChannelFromMessage
			} else if peer.GetUserId() > 0 {
				predicate = mtproto.Predicate_inputPeerUserFromMessage
			} else {
				return mtproto.ErrPeerIdInvalid
			}
		case peer.GetChannelId() > 0:
			predicate = mtproto.Predicate_inputPeerChannel
		case peer.GetChatId() > 0:
			predicate = mtproto.Predicate_inputPeerChat
		case peer.GetUserId() > 0:
			predicate = mtproto.Predicate_inputPeerUser
		default:
			predicate = mtproto.Predicate_inputPeerSelf
		}
	}

	switch predicate {
	case mtproto.Predicate_inputPeerSelf:
		if peer.GetUserId() != 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 ||
			peer.GetAccessHash() != 0 || peer.GetPeer() != nil || peer.GetMsgId() != 0 || peer.GetUsername() != "" {
			return mtproto.ErrPeerIdInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerUser:
		if peer.GetUserId() <= 0 || peer.GetAccessHash() == 0 || peer.GetChatId() != 0 || peer.GetChannelId() != 0 ||
			peer.GetPeer() != nil || peer.GetMsgId() != 0 || peer.GetUsername() != "" {
			return mtproto.ErrUserIdInvalid
		}
		return c.validateMiniAppUserPeer(peer.GetUserId(), peer.GetAccessHash())
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() <= 0 || peer.GetUserId() != 0 || peer.GetChannelId() != 0 || peer.GetAccessHash() != 0 ||
			peer.GetPeer() != nil || peer.GetMsgId() != 0 || peer.GetUsername() != "" {
			return mtproto.ErrPeerIdInvalid
		}
		return nil
	case mtproto.Predicate_inputPeerChannel:
		if peer.GetChannelId() <= 0 || peer.GetAccessHash() == 0 || peer.GetUserId() != 0 || peer.GetChatId() != 0 ||
			peer.GetPeer() != nil || peer.GetMsgId() != 0 || peer.GetUsername() != "" {
			return mtproto.ErrChannelInvalid
		}
		return c.validateMiniAppChannelPeer(peer.GetChannelId(), peer.GetAccessHash(), true)
	case mtproto.Predicate_inputPeerUserFromMessage:
		if peer.GetUserId() <= 0 || peer.GetMsgId() <= 0 || peer.GetPeer() == nil || peer.GetAccessHash() != 0 ||
			peer.GetChatId() != 0 || peer.GetChannelId() != 0 || peer.GetUsername() != "" {
			return mtproto.ErrPeerIdInvalid
		}
		if err := c.validateMiniAppPeer(callerID, peer.GetPeer()); err != nil {
			return err
		}
		return c.validateMiniAppUserPeer(peer.GetUserId(), 0)
	case mtproto.Predicate_inputPeerChannelFromMessage:
		if peer.GetChannelId() <= 0 || peer.GetMsgId() <= 0 || peer.GetPeer() == nil || peer.GetAccessHash() != 0 ||
			peer.GetUserId() != 0 || peer.GetChatId() != 0 || peer.GetUsername() != "" {
			return mtproto.ErrPeerIdInvalid
		}
		if err := c.validateMiniAppPeer(callerID, peer.GetPeer()); err != nil {
			return err
		}
		return c.validateMiniAppChannelPeer(peer.GetChannelId(), 0, false)
	case mtproto.Predicate_inputPeerUsername:
		if strings.TrimSpace(peer.GetUsername()) == "" || peer.GetUsername() != strings.TrimSpace(peer.GetUsername()) {
			return mtproto.ErrPeerIdInvalid
		}
		return mtproto.ErrMethodNotImpl
	default:
		return mtproto.ErrInputConstructorInvalid
	}
}

func (c *ApiFullCore) validateMiniAppUserPeer(userID, accessHash int64) error {
	if userID <= 0 {
		return mtproto.ErrUserIdInvalid
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	profile, err := d.UserGetImmutableUser(callContext(c), &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetId() != userID || profile.Deleted() {
		return mtproto.ErrUserIdInvalid
	}
	if accessHash != 0 && profile.GetUser().GetAccessHash() != accessHash {
		return mtproto.ErrUserIdInvalid
	}
	return nil
}

func (c *ApiFullCore) validateMiniAppChannelPeer(channelID, accessHash int64, requireAccessHash bool) error {
	if channelID <= 0 || (requireAccessHash && accessHash == 0) {
		return mtproto.ErrChannelInvalid
	}
	if !domain.Ready() {
		return mtproto.ErrMethodNotImpl
	}
	channel, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	if !ok || (requireAccessHash && channel.AccessHash != accessHash) {
		return mtproto.ErrChannelInvalid
	}
	return nil
}

func miniAppWebViewScopeKey(appKey string, callerID int64, peer *mtproto.InputPeer, startParam *wrapperspb.StringValue, platform string) string {
	start := ""
	if startParam != nil {
		start = startParam.GetValue()
	}
	return fmt.Sprintf("%s:%s:start:%s:platform:%s", appKey, businessControlPeerKey(callerID, peer), start, platform)
}

// miniAppWebViewScopeKeyWithOptions keeps the historical key for the default
// request shape while isolating provider results when a caller changes an
// option that affects the WebView contract. This preserves existing control
// plane records and prevents compact/fullscreen/write access or theme data
// from being reused across otherwise identical app requests.
func miniAppWebViewScopeKeyWithOptions(appKey string, callerID int64, peer *mtproto.InputPeer, startParam *wrapperspb.StringValue, platform string, writeAllowed, compact, fullscreen bool, themeParams *mtproto.DataJSON) string {
	base := miniAppWebViewScopeKey(appKey, callerID, peer, startParam, platform)
	if !writeAllowed && !compact && !fullscreen && (themeParams == nil || strings.TrimSpace(themeParams.GetData()) == "") {
		return base
	}
	options := struct {
		WriteAllowed bool   `json:"write_allowed"`
		Compact      bool   `json:"compact"`
		Fullscreen   bool   `json:"fullscreen"`
		ThemeParams  string `json:"theme_params"`
	}{
		WriteAllowed: writeAllowed,
		Compact:      compact,
		Fullscreen:   fullscreen,
	}
	if themeParams != nil {
		options.ThemeParams = themeParams.GetData()
	}
	return base + ":options" + businessControlHash("", options)
}

func validateWebViewResult(result *mtproto.WebViewResult) error {
	if result == nil || result.GetPredicateName() != mtproto.Predicate_webViewResultUrl || strings.TrimSpace(result.GetUrl()) == "" {
		return mtproto.ErrInternalServerError
	}
	if _, err := miniWebViewURL(result.GetUrl()); err != nil {
		return mtproto.ErrInternalServerError
	}
	if _, _, err := webViewResultQueryID(result); err != nil {
		return err
	}
	return nil
}

// validateMiniAppThemeParams accepts Telegram's DataJSON object shape while
// keeping malformed provider input away from the control plane. Theme values
// are strings in the Bot API; unknown keys remain valid for forward
// compatibility with newer clients.
func validateMiniAppThemeParams(themeParams *mtproto.DataJSON) error {
	if themeParams == nil {
		return nil
	}
	raw := strings.TrimSpace(themeParams.GetData())
	if raw == "" {
		return mtproto.ErrInputRequestInvalid
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || fields == nil {
		return mtproto.ErrInputRequestInvalid
	}
	for key, value := range fields {
		if strings.TrimSpace(key) == "" {
			return mtproto.ErrInputRequestInvalid
		}
		var stringValue string
		if err := json.Unmarshal(value, &stringValue); err != nil {
			return mtproto.ErrInputRequestInvalid
		}
	}
	return nil
}

// signedMiniAppURL adds Telegram's signed init data to a Web App URL. The
// bot token is read only from the internal user service response and is never
// included in the URL or persisted payload.
func (c *ApiFullCore) signedMiniAppURL(uid, botID, queryID int64, rawURL string) (string, error) {
	return c.signedMiniAppURLWithStartParam(uid, botID, queryID, rawURL, "")
}

func (c *ApiFullCore) signedMiniAppURLWithStartParam(uid, botID, queryID int64, rawURL, startParam string) (string, error) {
	validatedURL, err := miniWebViewURL(rawURL)
	if err != nil {
		return "", err
	}
	rawURL = validatedURL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", mtproto.ErrWebdocumentUrlInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return "", mtproto.ErrMethodNotImpl
	}
	botInfo, err := c.svcCtx.Dao.UserGetBotInfoV2(c.requestContext(), &userpb.TLUserGetBotInfoV2{BotId: botID})
	if err != nil {
		return "", err
	}
	if botInfo == nil {
		return "", mtproto.ErrMethodNotImpl
	}
	botToken := strings.TrimSpace(botInfo.GetToken())
	if botToken == "" {
		return "", mtproto.ErrMethodNotImpl
	}
	// Provider URLs are untrusted input at this boundary. Replace any
	// pre-existing init data instead of forwarding it across callers.
	query := parsed.Query()
	query.Del("tgWebAppData")
	parsed.RawQuery = query.Encode()
	rawURL = parsed.String()
	fields := url.Values{
		"auth_date": {strconv.FormatInt(time.Now().Unix(), 10)},
		"user":      {fmt.Sprintf(`{"id":%d}`, uid)},
	}
	if queryID > 0 {
		fields.Set("query_id", strconv.FormatInt(queryID, 10))
	}
	initData, err := botapi.SignWebAppInitData(fields, botToken)
	if err != nil {
		return "", mtproto.ErrInternalServerError
	}
	signedURL, err := botapi.AddWebAppInitData(rawURL, initData)
	if err != nil {
		return "", mtproto.ErrInternalServerError
	}
	return appendMiniAppStartParam(signedURL, startParam)
}

func appendMiniAppStartParam(rawURL, startParam string) (string, error) {
	return botapi.AddWebAppStartParam(rawURL, startParam)
}

func miniStartParam(value *wrapperspb.StringValue) string {
	if value == nil {
		return ""
	}
	return value.GetValue()
}

func (c *ApiFullCore) signWebViewResultURL(uid, botID int64, result *mtproto.WebViewResult, startParam string) error {
	queryID, present, err := webViewResultQueryID(result)
	if err != nil {
		return err
	}
	if !present {
		queryID = 0
	}
	normalizeWebViewResult(result, queryID, present)
	result.Url, err = c.signedMiniAppURLWithStartParam(uid, botID, queryID, result.GetUrl(), startParam)
	return err
}

// normalizeWebViewResult converts provider JSON and legacy constructor data
// into the Layer 229 result shape emitted by current WebView RPCs. The legacy
// query_id_INT64 field is not encoded by the modern constructor.
func normalizeWebViewResult(result *mtproto.WebViewResult, queryID int64, present bool) {
	if result == nil {
		return
	}
	result.Constructor = mtproto.TLConstructor_CRC32_webViewResultUrl_4d22ff98
	result.QueryId_INT64 = 0
	if present {
		result.QueryId_FLAGINT64 = wrapperspb.Int64(queryID)
	} else {
		result.QueryId_FLAGINT64 = nil
	}
}

// signAndPersistAppWebViewResult keeps the durable provider record stable.
// The returned WebView URL contains a fresh auth_date signature, so persisting
// that mutated URL would make a retry of the same provider query look like a
// conflicting request. Persist the verified provider result before signing the
// URL exposed to the client.
func (c *ApiFullCore) signAndPersistAppWebViewResult(uid, botID int64, result *mtproto.WebViewResult, startParam string, request any) error {
	if result == nil {
		return mtproto.ErrInternalServerError
	}
	providerResult := *result
	if err := c.signWebViewResultURL(uid, botID, result, startParam); err != nil {
		return err
	}
	return persistAppWebViewResult(uid, botID, &providerResult, request)
}

// resolveMiniWebViewURL implements Telegram's optional url semantics. A
// request without url is opened from the bot's configured menu web app.
func (c *ApiFullCore) resolveMiniWebViewURL(bot *mtproto.InputUser, requested *wrapperspb.StringValue) (string, error) {
	if requested != nil {
		return requested.GetValue(), nil
	}
	botID, err := c.verifyMiniBot(bot)
	if err != nil {
		return "", err
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return "", mtproto.ErrMethodNotImpl
	}
	botInfo, err := c.svcCtx.Dao.UserGetBotInfoV2(c.requestContext(), &userpb.TLUserGetBotInfoV2{BotId: botID})
	if err != nil {
		return "", err
	}
	if botInfo == nil || botInfo.GetBotInfo() == nil || botInfo.GetBotInfo().GetMenuButton() == nil {
		return "", mtproto.ErrMethodNotImpl
	}
	menu := botInfo.GetBotInfo().GetMenuButton()
	if menu.GetPredicateName() != mtproto.Predicate_botMenuButton || strings.TrimSpace(menu.GetUrl()) == "" {
		return "", mtproto.ErrMethodNotImpl
	}
	return menu.GetUrl(), nil
}

func webViewResultQueryID(result *mtproto.WebViewResult) (int64, bool, error) {
	if result == nil {
		return 0, false, mtproto.ErrInternalServerError
	}
	constructor := result.GetConstructor()
	queryFlag := result.GetQueryId_FLAGINT64()
	queryInt := result.GetQueryId_INT64()
	switch constructor {
	case mtproto.TLConstructor_CRC32_webViewResultUrl_4d22ff98:
		if queryInt != 0 || queryFlag != nil && queryFlag.GetValue() <= 0 {
			return 0, false, mtproto.ErrInternalServerError
		}
		if queryFlag == nil {
			return 0, false, nil
		}
		return queryFlag.GetValue(), true, nil
	case mtproto.TLConstructor_CRC32_webViewResultUrl_c14557c:
		if queryFlag != nil || queryInt <= 0 {
			return 0, false, mtproto.ErrInternalServerError
		}
		return queryInt, true, nil
	case mtproto.TLConstructor_CRC32_UNKNOWN:
		// JSON provider records historically omitted constructor. Infer the
		// only shape represented by the populated fields, but reject mixed
		// fields so a provider cannot silently change Layer 229 semantics.
		if queryFlag != nil {
			if queryFlag.GetValue() <= 0 || queryInt != 0 {
				return 0, false, mtproto.ErrInternalServerError
			}
			return queryFlag.GetValue(), true, nil
		}
		if queryInt < 0 {
			return 0, false, mtproto.ErrInternalServerError
		}
		if queryInt > 0 {
			return queryInt, true, nil
		}
		return 0, false, nil
	default:
		return 0, false, mtproto.ErrInternalServerError
	}
}

// persistAppWebViewResult records the provider's verified URL and query ID in
// the same PostgreSQL table used by requestWebView. This makes prolong and
// restart recovery use the exact owner/bot binding returned by the provider.
func persistAppWebViewResult(uid, botID int64, result *mtproto.WebViewResult, request any) error {
	queryID, present, err := webViewResultQueryID(result)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	payload, err := miniWebViewStablePayload("requestAppWebView", result.GetUrl(), queryID, request)
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	return persist.PutWebViewRequestIfAbsent(persist.WebViewRequest{
		RequestID: fmt.Sprintf("provider:%d:%d", uid, queryID),
		UserID:    uid,
		BotID:     botID,
		Kind:      "requestAppWebView",
		Payload:   payload,
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	})
}

func (c *ApiFullCore) requestMiniWebView(uid int64, bot *mtproto.InputUser, rawURL, startParam, kind, platform string, fullsize, fullscreen bool, request any) (*mtproto.WebViewResult, error) {
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
	rawURL, err = c.signedMiniAppURLWithStartParam(uid, botID, queryID, rawURL, startParam)
	if err != nil {
		return nil, err
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

func (c *ApiFullCore) requestMiniSimpleWebView(uid int64, bot *mtproto.InputUser, rawURL, startParam, kind, platform string, request any) (*mtproto.SimpleWebViewResult, error) {
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
	rawURL, err = c.signedMiniAppURLWithStartParam(uid, botID, queryID, rawURL, startParam)
	if err != nil {
		return nil, err
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
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	if err := c.validateMiniAppPeer(uid, in.GetPeer()); err != nil {
		return nil, err
	}
	rawURL, err := c.resolveMiniWebViewURL(in.GetBot(), in.GetUrl())
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	startParam := ""
	if value := in.GetStartParam(); value != nil {
		startParam = value.GetValue()
	}
	return c.requestMiniWebView(uid, in.GetBot(), rawURL, startParam, "requestWebView", in.GetPlatform(), !in.GetCompact(), in.GetFullscreen(), in)
}

func (c *ApiFullCore) MessagesProlongWebView(in *mtproto.TLMessagesProlongWebView) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetBot() == nil || in.GetQueryId() <= 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	if err := c.validateMiniAppPeer(uid, in.GetPeer()); err != nil {
		return nil, err
	}
	botID, err := c.verifyMiniBot(in.GetBot())
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	prolonged, err := persist.ProlongWebViewQuery(uid, botID, in.GetQueryId(), 5*time.Minute)
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
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	rawURL, err := c.resolveMiniWebViewURL(in.GetBot(), in.GetUrl())
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	startParam := ""
	if value := in.GetStartParam(); value != nil {
		startParam = value.GetValue()
	}
	return c.requestMiniWebView(uid, in.GetBot(), rawURL, startParam, "requestSimpleWebView", in.GetPlatform(), !in.GetCompact(), in.GetFullscreen(), in)
}

func (c *ApiFullCore) MessagesSendWebViewResultMessage(in *mtproto.TLMessagesSendWebViewResultMessage) (*mtproto.WebViewMessageSent, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.GetBotQueryId()) == "" || in.GetResult() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if strings.TrimSpace(in.GetResult().GetId()) == "" || strings.TrimSpace(in.GetResult().GetType()) == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateInlineResultPredicate(in.GetResult().GetPredicateName()); err != nil {
		return nil, err
	}
	// This RPC is a bot-side callback. Authenticate the caller before either
	// reading a control-plane result or contacting the external provider.
	if err := c.requireBotCaller(callerID); err != nil {
		return nil, err
	}
	result := new(mtproto.WebViewMessageSent)
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.sendWebViewResultMessage", "bot-query:"+in.GetBotQueryId(), result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := validateWebViewMessageSent(result); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.sendWebViewResultMessage", map[string]any{
		"scope_key": "bot-query:" + in.GetBotQueryId(), "bot_query_id": in.GetBotQueryId(), "result": in.GetResult(),
	}, result); err != nil {
		return nil, err
	}
	if err := validateWebViewMessageSent(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ApiFullCore) MessagesSendWebViewData(in *mtproto.TLMessagesSendWebViewData) (*mtproto.Updates, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || in.GetRandomId() == 0 || in.GetButtonText() == "" || len(in.GetData()) > 4096 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateProviderBotInputUser(in.GetBot()); err != nil {
		return nil, err
	}
	if _, err := c.verifyMiniBot(in.GetBot()); err != nil {
		return nil, err
	}
	result := new(mtproto.Updates)
	scopeKey := fmt.Sprintf("bot:%d:random:%d", in.GetBot().GetUserId(), in.GetRandomId())
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.sendWebViewData", scopeKey, result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := validateUpdatesResult(result); err != nil {
			return nil, err
		}
		if err := c.pushWebViewDataUpdate(callerID, in.GetBot().GetUserId(), in.GetRandomId(), in.GetButtonText(), in.GetData()); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.sendWebViewData", map[string]any{
		"scope_key": scopeKey, "bot": in.GetBot(), "random_id": in.GetRandomId(), "button_text": in.GetButtonText(), "data": in.GetData(),
	}, result); err != nil {
		return nil, err
	}
	if err := validateUpdatesResult(result); err != nil {
		return nil, err
	}
	if err := c.pushWebViewDataUpdate(callerID, in.GetBot().GetUserId(), in.GetRandomId(), in.GetButtonText(), in.GetData()); err != nil {
		return nil, err
	}
	return result, nil
}

// pushWebViewDataUpdate delivers the Bot API-shaped service message to the
// bot's durable update queue after the external provider accepted the data.
func (c *ApiFullCore) pushWebViewDataUpdate(callerID, botID, randomID int64, buttonText, data string) error {
	d := c.apifullDao()
	if d == nil || d.SyncClient == nil {
		// Provider contract tests intentionally omit the sync client. Production
		// APIFull always wires it through the service constructor.
		return nil
	}
	if d.UserClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	profile, err := d.UserGetImmutableUser(c.requestContext(), &userpb.TLUserGetImmutableUser{Id: callerID})
	if err != nil {
		return err
	}
	if profile == nil || profile.GetUser() == nil || profile.Deleted() {
		return mtproto.ErrUserIdInvalid
	}
	messageID, err := newInlineQueryID()
	if err != nil {
		return mtproto.ErrInternalServerError
	}
	messageID = messageID & 0x7fffffff
	if messageID == 0 {
		messageID = 1
	}
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Id:     int32(messageID),
		PeerId: mtproto.MakePeerUser(callerID),
		FromId: mtproto.MakePeerUser(callerID),
		Date:   int32(time.Now().Unix()),
		Action: mtproto.MakeMessageActionWebViewDataSentMe(buttonText, data),
		// SavedPeerId is not part of Bot API Message conversion. Carry the
		// client random_id through the sync hop so the durable update store can
		// enforce Telegram's random-id idempotency after provider retries.
		SavedPeerId: mtproto.MakePeerUser(randomID),
	}).To_Message()
	update := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: message}).To_Update()
	_, err = d.SyncClient.SyncPushBotUpdates(c.requestContext(), &syncpb.TLSyncPushBotUpdates{
		UserId:  botID,
		Updates: mtproto.MakeUpdatesByUpdatesUsers([]*mtproto.User{profile.ToUser(botID)}, update),
	})
	return err
}

func (c *ApiFullCore) MessagesGetBotApp(in *mtproto.TLMessagesGetBotApp) (*mtproto.Messages_BotApp, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetApp() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateInputBotApp(in.GetApp()); err != nil {
		return nil, err
	}
	result := new(mtproto.Messages_BotApp)
	app := in.GetApp()
	if _, err := c.verifyMiniBot(app.GetBotId()); err != nil {
		return nil, err
	}
	appKey := fmt.Sprintf("bot:%d:id:%d", app.GetBotId().GetUserId(), app.GetId())
	if app.GetPredicateName() == mtproto.Predicate_inputBotAppShortName {
		appKey = fmt.Sprintf("bot:%d:short:%s", app.GetBotId().GetUserId(), app.GetShortName())
	}
	if local, loadErr := c.loadLocalBusinessResult("messages.getBotApp", appKey, result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if result.GetPredicateName() != mtproto.Predicate_messages_botApp || result.GetApp() == nil || result.GetApp().GetId() <= 0 || result.GetApp().GetAccessHash() == 0 {
			return nil, mtproto.ErrInternalServerError
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.getBotApp", map[string]any{
		"scope_key": appKey, "app": in.GetApp(), "hash": in.GetHash(), "caller_user_id": callerID,
	}, result); err != nil {
		return nil, err
	}
	if result.GetPredicateName() != mtproto.Predicate_messages_botApp || result.GetApp() == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) MessagesRequestAppWebView53618BCE(in *mtproto.TLMessagesRequestAppWebView53618BCE) (*mtproto.WebViewResult, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetApp() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	if err := c.validateMiniAppPeer(callerID, in.GetPeer()); err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if err := validateInputBotApp(in.GetApp()); err != nil {
		return nil, err
	}
	result := new(mtproto.WebViewResult)
	app := in.GetApp()
	botID := app.GetBotId().GetUserId()
	if verifiedBotID, err := c.verifyMiniBot(app.GetBotId()); err != nil || verifiedBotID != botID {
		if err != nil {
			return nil, err
		}
		return nil, mtproto.ErrUserIdInvalid
	}
	appKey := fmt.Sprintf("bot:%d:id:%d", app.GetBotId().GetUserId(), app.GetId())
	if app.GetPredicateName() == mtproto.Predicate_inputBotAppShortName {
		appKey = fmt.Sprintf("bot:%d:short:%s", app.GetBotId().GetUserId(), app.GetShortName())
	}
	scopeKey := miniAppWebViewScopeKeyWithOptions(appKey, callerID, in.GetPeer(), in.GetStartParam(), in.GetPlatform(), in.GetWriteAllowed(), in.GetCompact(), in.GetFullscreen(), in.GetThemeParams())
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.requestAppWebView", scopeKey, result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := validateWebViewResult(result); err != nil {
			return nil, err
		}
		if err := c.signAndPersistAppWebViewResult(callerID, botID, result, miniStartParam(in.GetStartParam()), in); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := c.callBusinessProvider("messages.requestAppWebView", map[string]any{
		"scope_key": scopeKey, "write_allowed": in.GetWriteAllowed(), "compact": in.GetCompact(), "fullscreen": in.GetFullscreen(),
		"peer": in.GetPeer(), "app": in.GetApp(), "start_param": in.GetStartParam(), "theme_params": in.GetThemeParams(), "platform": in.GetPlatform(),
	}, result); err != nil {
		return nil, err
	}
	if err := validateWebViewResult(result); err != nil {
		return nil, err
	}
	if err := c.signAndPersistAppWebViewResult(callerID, botID, result, miniStartParam(in.GetStartParam()), in); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ApiFullCore) MessagesRequestChatJoinWebView(in *mtproto.TLMessagesRequestChatJoinWebView) (*mtproto.WebViewResult, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() <= 0 {
		return nil, mtproto.ErrQueryIdEmpty
	}
	if err := validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	result := new(mtproto.WebViewResult)
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.requestChatJoinWebView", businessControlQueryKey(in.GetQueryId()), result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err := validateWebViewResult(result); err != nil {
			return nil, err
		}
		normalizeWebViewResult(result, func() int64 {
			queryID, _, _ := webViewResultQueryID(result)
			return queryID
		}(), func() bool {
			_, present, _ := webViewResultQueryID(result)
			return present
		}())
		return result, nil
	}
	if err := c.callBusinessProvider("messages.requestChatJoinWebView", map[string]any{
		"scope_key": businessControlQueryKey(in.GetQueryId()), "query_id": in.GetQueryId(), "theme_params": in.GetThemeParams(), "platform": in.GetPlatform(),
	}, result); err != nil {
		return nil, err
	}
	if err := validateWebViewResult(result); err != nil {
		return nil, err
	}
	queryID, present, err := webViewResultQueryID(result)
	if err != nil {
		return nil, err
	}
	normalizeWebViewResult(result, queryID, present)
	return result, nil
}

func (c *ApiFullCore) BotsCanSendMessage(in *mtproto.TLBotsCanSendMessage) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
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
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
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
	if in == nil || in.GetBot() == nil || strings.TrimSpace(in.GetCustomMethod()) == "" || !validBotDataJSON(in.GetParams()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if _, err := c.verifyMiniBot(in.GetBot()); err != nil {
		return nil, err
	}
	result := new(mtproto.DataJSON)
	if local, loadErr := c.loadLocalBusinessResultForCaller("bots.invokeWebViewCustomMethod", businessControlHash("bot:"+strconv.FormatInt(in.GetBot().GetUserId(), 10)+":method:"+in.GetCustomMethod(), in.GetParams()), result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if !validBotDataJSON(result) {
			return nil, mtproto.ErrInternalServerError
		}
		return result, nil
	}
	if err := c.callBusinessProvider("bots.invokeWebViewCustomMethod", map[string]any{
		"scope_key": businessControlHash("bot:"+strconv.FormatInt(in.GetBot().GetUserId(), 10)+":method:"+in.GetCustomMethod(), in.GetParams()), "bot": in.GetBot(), "custom_method": in.GetCustomMethod(), "params": in.GetParams(),
	}, result); err != nil {
		return nil, err
	}
	if !validBotDataJSON(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) BotsCheckDownloadFileParams(in *mtproto.TLBotsCheckDownloadFileParams) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetBot() == nil || strings.TrimSpace(in.GetFileName()) == "" || strings.TrimSpace(in.GetUrl()) == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if _, err := c.verifyMiniBot(in.GetBot()); err != nil {
		return nil, err
	}
	var result struct {
		Value *bool `json:"value"`
	}
	if local, loadErr := c.loadLocalBusinessResultForCaller("bots.checkDownloadFileParams", businessControlHash("bot:"+strconv.FormatInt(in.GetBot().GetUserId(), 10)+":file:"+in.GetFileName(), in.GetUrl()), &result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if result.Value == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if *result.Value {
			return mtproto.BoolTrue, nil
		}
		return mtproto.BoolFalse, nil
	}
	if err := c.callBusinessProvider("bots.checkDownloadFileParams", map[string]any{
		"bot": in.GetBot(), "file_name": in.GetFileName(), "url": in.GetUrl(),
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

func (c *ApiFullCore) BotsRequestWebViewButton(in *mtproto.TLBotsRequestWebViewButton) (*mtproto.Bots_RequestedButton, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	userID, err := c.verifyMiniUser(uid, in.GetUserId())
	if err != nil {
		return nil, err
	}
	button := in.GetButton()
	if button == nil {
		return nil, mtproto.ErrButtonDataInvalid
	}
	switch button.GetPredicateName() {
	case mtproto.Predicate_keyboardButtonWebView, mtproto.Predicate_keyboardButtonSimpleWebView:
		if _, err = miniWebViewURL(button.GetUrl()); err != nil {
			return nil, err
		}
	case mtproto.Predicate_inputKeyboardButtonRequestPeer:
		if err = validateRequestPeerKeyboardButton(button); err != nil {
			return nil, err
		}
	default:
		return nil, mtproto.ErrButtonDataInvalid
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
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
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
	if button.GetPredicateName() == mtproto.Predicate_inputKeyboardButtonRequestPeer {
		return mtproto.MakeTLKeyboardButtonRequestPeer(&button).To_KeyboardButton(), nil
	}
	return &button, nil
}

func validateRequestPeerKeyboardButton(button *mtproto.KeyboardButton) error {
	if button == nil || button.GetPeerType() == nil {
		return mtproto.ErrButtonDataInvalid
	}
	switch button.GetPeerType().GetPredicateName() {
	case mtproto.Predicate_requestPeerTypeUser:
		if button.GetMaxQuantity() < 1 || button.GetMaxQuantity() > 10 {
			return mtproto.ErrButtonDataInvalid
		}
	case mtproto.Predicate_requestPeerTypeChat, mtproto.Predicate_requestPeerTypeBroadcast:
		if button.GetMaxQuantity() != 1 {
			return mtproto.ErrButtonDataInvalid
		}
	case mtproto.Predicate_requestPeerTypeCreateBot:
		if button.GetMaxQuantity() != 1 || !button.GetPeerType().GetBotManaged() {
			return mtproto.ErrButtonDataInvalid
		}
	default:
		return mtproto.ErrButtonDataInvalid
	}
	return nil
}

func (c *ApiFullCore) MessagesRequestSimpleWebView1A46500A(in *mtproto.TLMessagesRequestSimpleWebView1A46500A) (*mtproto.SimpleWebViewResult, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	rawURL, err := c.resolveMiniWebViewURL(in.GetBot(), in.GetUrl())
	if err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	startParam := ""
	if value := in.GetStartParam(); value != nil {
		startParam = value.GetValue()
	}
	return c.requestMiniSimpleWebView(uid, in.GetBot(), rawURL, startParam, "requestSimpleWebView", in.GetPlatform(), in)
}

func (c *ApiFullCore) MessagesRequestAppWebView8C5A3B3C(in *mtproto.TLMessagesRequestAppWebView8C5A3B3C) (*mtproto.AppWebViewResult, error) {
	callerID, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetApp() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err = validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	if err = c.validateMiniAppPeer(callerID, in.GetPeer()); err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if err = validateInputBotApp(in.GetApp()); err != nil {
		return nil, err
	}
	app := in.GetApp()
	botID := app.GetBotId().GetUserId()
	if verifiedBotID, verifyErr := c.verifyMiniBot(app.GetBotId()); verifyErr != nil || verifiedBotID != botID {
		if verifyErr != nil {
			return nil, verifyErr
		}
		return nil, mtproto.ErrUserIdInvalid
	}
	appKey := fmt.Sprintf("bot:%d:id:%d", botID, app.GetId())
	if app.GetPredicateName() == mtproto.Predicate_inputBotAppShortName {
		appKey = fmt.Sprintf("bot:%d:short:%s", botID, app.GetShortName())
	}
	scopeKey := miniAppWebViewScopeKeyWithOptions(appKey, callerID, in.GetPeer(), in.GetStartParam(), in.GetPlatform(), in.GetWriteAllowed(), false, false, in.GetThemeParams())
	result := new(mtproto.WebViewResult)
	if local, loadErr := c.loadLocalBusinessResultForCaller("messages.requestAppWebView", scopeKey, result); loadErr != nil {
		return nil, loadErr
	} else if local {
		if err = validateWebViewResult(result); err != nil {
			return nil, err
		}
		if err = c.signAndPersistAppWebViewResult(callerID, botID, result, miniStartParam(in.GetStartParam()), in); err != nil {
			return nil, err
		}
		return mtproto.MakeTLAppWebViewResultUrl(&mtproto.AppWebViewResult{Url: result.GetUrl()}).To_AppWebViewResult(), nil
	}
	if err = c.callBusinessProvider("messages.requestAppWebView", map[string]any{
		"scope_key": scopeKey, "write_allowed": in.GetWriteAllowed(), "peer": in.GetPeer(), "app": in.GetApp(),
		"start_param": in.GetStartParam(), "theme_params": in.GetThemeParams(), "platform": in.GetPlatform(),
	}, result); err != nil {
		return nil, err
	}
	if err = validateWebViewResult(result); err != nil {
		return nil, err
	}
	if err = c.signAndPersistAppWebViewResult(callerID, botID, result, miniStartParam(in.GetStartParam()), in); err != nil {
		return nil, err
	}
	return mtproto.MakeTLAppWebViewResultUrl(&mtproto.AppWebViewResult{Url: result.GetUrl()}).To_AppWebViewResult(), nil
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
	if err := validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	rawURL := in.GetUrl()
	if strings.TrimSpace(rawURL) == "" {
		rawURL, err = c.resolveMiniWebViewURL(in.GetBot(), nil)
		if err != nil {
			return nil, err
		}
	}
	return c.requestMiniSimpleWebView(uid, in.GetBot(), rawURL, "", "requestSimpleWebView", in.GetPlatform(), in)
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
	if err := validateMiniAppThemeParams(in.GetThemeParams()); err != nil {
		return nil, err
	}
	rawURL := in.GetUrl()
	if strings.TrimSpace(rawURL) == "" {
		rawURL, err = c.resolveMiniWebViewURL(in.GetBot(), nil)
		if err != nil {
			return nil, err
		}
	}
	return c.requestMiniSimpleWebView(uid, in.GetBot(), rawURL, "", "requestSimpleWebView", "", in)
}
