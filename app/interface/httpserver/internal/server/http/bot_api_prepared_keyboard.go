// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botAPIPreparedKeyboardButton struct {
	Text              string                                 `json:"text"`
	IconCustomEmojiID string                                 `json:"icon_custom_emoji_id,omitempty"`
	Style             string                                 `json:"style,omitempty"`
	RequestUsers      *botAPIKeyboardButtonRequestUsers      `json:"request_users,omitempty"`
	RequestChat       *botAPIKeyboardButtonRequestChat       `json:"request_chat,omitempty"`
	RequestManagedBot *botAPIKeyboardButtonRequestManagedBot `json:"request_managed_bot,omitempty"`
}

type botAPIKeyboardButtonRequestUsers struct {
	RequestID       *int64 `json:"request_id"`
	UserIsBot       *bool  `json:"user_is_bot,omitempty"`
	UserIsPremium   *bool  `json:"user_is_premium,omitempty"`
	MaxQuantity     *int64 `json:"max_quantity,omitempty"`
	RequestName     bool   `json:"request_name,omitempty"`
	RequestUsername bool   `json:"request_username,omitempty"`
	RequestPhoto    bool   `json:"request_photo,omitempty"`
}

type botAPIKeyboardButtonRequestChat struct {
	RequestID               *int64                          `json:"request_id"`
	ChatIsChannel           *bool                           `json:"chat_is_channel"`
	ChatIsForum             *bool                           `json:"chat_is_forum,omitempty"`
	ChatHasUsername         *bool                           `json:"chat_has_username,omitempty"`
	ChatIsCreated           *bool                           `json:"chat_is_created,omitempty"`
	UserAdministratorRights *botapi.ChatAdministratorRights `json:"user_administrator_rights,omitempty"`
	BotAdministratorRights  *botapi.ChatAdministratorRights `json:"bot_administrator_rights,omitempty"`
	BotIsMember             *bool                           `json:"bot_is_member,omitempty"`
	RequestTitle            bool                            `json:"request_title,omitempty"`
	RequestUsername         bool                            `json:"request_username,omitempty"`
	RequestPhoto            bool                            `json:"request_photo,omitempty"`
}

type botAPIKeyboardButtonRequestManagedBot struct {
	RequestID         *int64  `json:"request_id"`
	SuggestedName     *string `json:"suggested_name,omitempty"`
	SuggestedUsername *string `json:"suggested_username,omitempty"`
}

func savePreparedKeyboardButton(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.MiniBotAppsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: mini app service is not configured"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	button, err := botAPIInputPreparedKeyboardButton(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: button is invalid"
	}
	saved, err := ctx.MiniBotAppsClient.BotsRequestWebViewButton(botRPCContext(r.Context(), botID), &mtproto.TLBotsRequestWebViewButton{
		UserId: inputUser,
		Button: button,
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if saved == nil || strings.TrimSpace(saved.GetWebappReqId()) == "" {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: prepared keyboard button was not returned"
	}
	return &botapi.PreparedKeyboardButton{ID: saved.GetWebappReqId()}, http.StatusOK, 0, ""
}

func botAPIInputPreparedKeyboardButton(request *botAPIRequest) (*mtproto.KeyboardButton, error) {
	if request == nil || len(request.raw("button")) == 0 || string(request.raw("button")) == "null" {
		return nil, errors.New("button is required")
	}
	var input botAPIPreparedKeyboardButton
	if err := request.json("button", &input); err != nil {
		return nil, err
	}
	if !utf8.ValidString(input.Text) || utf8.RuneCountInString(input.Text) == 0 || utf8.RuneCountInString(input.Text) > 64 {
		return nil, errors.New("text is invalid")
	}
	style, err := botAPIKeyboardButtonStyle(input.Style, input.IconCustomEmojiID)
	if err != nil {
		return nil, err
	}
	kinds := 0
	if input.RequestUsers != nil {
		kinds++
	}
	if input.RequestChat != nil {
		kinds++
	}
	if input.RequestManagedBot != nil {
		kinds++
	}
	if kinds != 1 {
		return nil, errors.New("exactly one request button type is required")
	}
	if input.RequestUsers != nil {
		return botAPIInputPreparedUsersButton(input.Text, style, input.RequestUsers)
	}
	if input.RequestChat != nil {
		return botAPIInputPreparedChatButton(input.Text, style, input.RequestChat)
	}
	return botAPIInputPreparedManagedBotButton(input.Text, style, input.RequestManagedBot)
}

func botAPIKeyboardButtonStyle(styleName, iconID string) (*mtproto.KeyboardButtonStyle, error) {
	styleName = strings.ToLower(strings.TrimSpace(styleName))
	if styleName != "" && styleName != "primary" && styleName != "danger" && styleName != "success" {
		return nil, errors.New("style is invalid")
	}
	style := &mtproto.KeyboardButtonStyle{}
	switch styleName {
	case "primary":
		style.BgPrimary = true
	case "danger":
		style.BgDanger = true
	case "success":
		style.BgSuccess = true
	}
	if strings.TrimSpace(iconID) != "" {
		id, err := strconv.ParseInt(strings.TrimSpace(iconID), 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("icon_custom_emoji_id is invalid")
		}
		style.Icon = wrapperspb.Int64(id)
	}
	if styleName == "" && style.Icon == nil {
		return nil, nil
	}
	return mtproto.MakeTLKeyboardButtonStyle(style).To_KeyboardButtonStyle(), nil
}

func botAPIRequestID(value *int64) (int32, error) {
	if value == nil || *value < -1<<31 || *value > 1<<31-1 {
		return 0, errors.New("request_id is invalid")
	}
	return int32(*value), nil
}

func botAPIPreparedButtonBase(text string, style *mtproto.KeyboardButtonStyle, requestID int32, peerType *mtproto.RequestPeerType, maxQuantity int32) *mtproto.KeyboardButton {
	return &mtproto.KeyboardButton{
		Style:       style,
		Text:        text,
		ButtonId:    requestID,
		PeerType:    peerType,
		MaxQuantity: maxQuantity,
	}
}

func botAPIInputPreparedUsersButton(text string, style *mtproto.KeyboardButtonStyle, input *botAPIKeyboardButtonRequestUsers) (*mtproto.KeyboardButton, error) {
	requestID, err := botAPIRequestID(input.RequestID)
	if err != nil {
		return nil, err
	}
	maxQuantity := int64(1)
	if input.MaxQuantity != nil {
		maxQuantity = *input.MaxQuantity
	}
	if maxQuantity < 1 || maxQuantity > 10 {
		return nil, errors.New("max_quantity is invalid")
	}
	peerType := &mtproto.RequestPeerType{}
	if input.UserIsBot != nil {
		peerType.Bot = mtproto.ToBool(*input.UserIsBot)
	}
	if input.UserIsPremium != nil {
		peerType.Premium = mtproto.ToBool(*input.UserIsPremium)
	}
	peer := mtproto.MakeTLRequestPeerTypeUser(peerType).To_RequestPeerType()
	button := botAPIPreparedButtonBase(text, style, requestID, peer, int32(maxQuantity))
	button.NameRequested = input.RequestName
	button.UsernameRequested = input.RequestUsername
	button.PhotoRequested = input.RequestPhoto
	return mtproto.MakeTLInputKeyboardButtonRequestPeer(button).To_KeyboardButton(), nil
}

func botAPIInputPreparedChatButton(text string, style *mtproto.KeyboardButtonStyle, input *botAPIKeyboardButtonRequestChat) (*mtproto.KeyboardButton, error) {
	requestID, err := botAPIRequestID(input.RequestID)
	if err != nil {
		return nil, err
	}
	if input.ChatIsChannel == nil {
		return nil, errors.New("chat_is_channel is required")
	}
	userRights := botAPIPreparedChatAdminRights(input.UserAdministratorRights)
	botRights := botAPIPreparedChatAdminRights(input.BotAdministratorRights)
	peer := &mtproto.RequestPeerType{}
	if input.ChatHasUsername != nil {
		peer.HasUsername = mtproto.ToBool(*input.ChatHasUsername)
	}
	if input.ChatIsForum != nil {
		if *input.ChatIsChannel {
			return nil, errors.New("chat_is_forum is invalid for channels")
		}
		peer.Forum = mtproto.ToBool(*input.ChatIsForum)
	}
	if input.ChatIsCreated != nil && *input.ChatIsCreated {
		peer.Creator = true
	}
	if input.BotIsMember != nil && *input.BotIsMember {
		peer.BotParticipant = true
	}
	peer.UserAdminRights = userRights
	peer.BotAdminRights = botRights
	if *input.ChatIsChannel {
		peer = mtproto.MakeTLRequestPeerTypeBroadcast(peer).To_RequestPeerType()
	} else {
		peer = mtproto.MakeTLRequestPeerTypeChat(peer).To_RequestPeerType()
	}
	button := botAPIPreparedButtonBase(text, style, requestID, peer, 1)
	button.NameRequested = input.RequestTitle
	button.UsernameRequested = input.RequestUsername
	button.PhotoRequested = input.RequestPhoto
	return mtproto.MakeTLInputKeyboardButtonRequestPeer(button).To_KeyboardButton(), nil
}

func botAPIInputPreparedManagedBotButton(text string, style *mtproto.KeyboardButtonStyle, input *botAPIKeyboardButtonRequestManagedBot) (*mtproto.KeyboardButton, error) {
	requestID, err := botAPIRequestID(input.RequestID)
	if err != nil {
		return nil, err
	}
	peer := &mtproto.RequestPeerType{BotManaged: true}
	if input.SuggestedName != nil {
		if !utf8.ValidString(*input.SuggestedName) {
			return nil, errors.New("suggested_name is invalid")
		}
		peer.SuggestedName = wrapperspb.String(*input.SuggestedName)
	}
	if input.SuggestedUsername != nil {
		if !utf8.ValidString(*input.SuggestedUsername) {
			return nil, errors.New("suggested_username is invalid")
		}
		peer.SuggestedUsername = wrapperspb.String(*input.SuggestedUsername)
	}
	peer = mtproto.MakeTLRequestPeerTypeCreateBot(peer).To_RequestPeerType()
	return mtproto.MakeTLInputKeyboardButtonRequestPeer(botAPIPreparedButtonBase(text, style, requestID, peer, 1)).To_KeyboardButton(), nil
}

func botAPIPreparedChatAdminRights(input *botapi.ChatAdministratorRights) *mtproto.ChatAdminRights {
	if input == nil {
		return nil
	}
	return mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo:           input.CanChangeInfo,
		PostMessages:         input.CanPostMessages,
		EditMessages:         input.CanEditMessages,
		DeleteMessages:       input.CanDeleteMessages,
		BanUsers:             input.CanRestrictMembers,
		InviteUsers:          input.CanInviteUsers,
		PinMessages:          input.CanPinMessages,
		AddAdmins:            input.CanPromoteMembers,
		Anonymous:            input.IsAnonymous,
		ManageCall:           input.CanManageVideoChats,
		Other:                input.CanManageChat,
		ManageTopics:         input.CanManageTopics,
		PostStories:          input.CanPostStories,
		EditStories:          input.CanEditStories,
		DeleteStories:        input.CanDeleteStories,
		ManageDirectMessages: input.CanManageDirectMessages,
	}).To_ChatAdminRights()
}
