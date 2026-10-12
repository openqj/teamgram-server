// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func botAPIInvitePeer(r *http.Request, ctx *svc.ServiceContext, botID int64, request *botAPIRequest) (*mtproto.InputPeer, int32, int64, error) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, 0, 0, errors.New("invalid chat id")
	}
	return botAPIInputPeer(botID, peerType, peerID), peerType, peerID, nil
}

func botAPIOptionalInt32(request *botAPIRequest, name string, min, max int64) (*wrapperspb.Int32Value, error) {
	if request == nil || len(request.raw(name)) == 0 || string(request.raw(name)) == "null" || request.string(name) == "" {
		return nil, nil
	}
	value, err := request.int64(name)
	if err != nil || value < min || value > max {
		return nil, errors.New("invalid " + name)
	}
	return wrapperspb.Int32(int32(value)), nil
}

func botAPIInviteLinkFromMTProto(invite *mtproto.ExportedChatInvite, creator *botapi.User) *botapi.ChatInviteLink {
	if invite == nil {
		return nil
	}
	name := ""
	if invite.GetTitle() != nil {
		name = invite.GetTitle().GetValue()
	}
	expireDate := int64(0)
	if invite.GetExpireDate() != nil {
		expireDate = int64(invite.GetExpireDate().GetValue())
	}
	memberLimit := 0
	if invite.GetUsageLimit() != nil {
		memberLimit = int(invite.GetUsageLimit().GetValue())
	}
	pending := 0
	if invite.GetRequested() != nil {
		pending = int(invite.GetRequested().GetValue())
	}
	link := &botapi.ChatInviteLink{
		InviteLink:              invite.GetLink(),
		Creator:                 creator,
		CreatesJoinRequest:      invite.GetRequestNeeded(),
		IsPrimary:               invite.GetPermanent(),
		IsRevoked:               invite.GetRevoked(),
		Name:                    name,
		ExpireDate:              expireDate,
		MemberLimit:             memberLimit,
		PendingJoinRequestCount: pending,
	}
	return link
}

func botAPIExportedChatInvite(result *mtproto.Messages_ExportedChatInvite) *mtproto.ExportedChatInvite {
	if result == nil {
		return nil
	}
	if result.GetInvite() != nil {
		return result.GetInvite()
	}
	return result.GetNewInvite()
}

func botAPIInviteCreator(ctx *svc.ServiceContext, requestCtx context.Context, botID, adminID int64) *botapi.User {
	creatorID := adminID
	if creatorID <= 0 {
		creatorID = botID
	}
	if ctx != nil && ctx.UserClient != nil {
		// The bot itself is the creator for Bot API calls. For an invite created
		// by another administrator, use the authoritative user record when it is
		// available and keep a minimal shape when it is not.
		if user, err := ctx.UserClient.UserGetImmutableUser(botRPCContext(requestCtx, creatorID), &userpb.TLUserGetImmutableUser{Id: creatorID}); err == nil && user != nil {
			if converted := botapi.UserFromImmutableUser(user); converted != nil {
				return converted
			}
		}
	}
	return &botapi.User{ID: creatorID, IsBot: creatorID == botID}
}

func exportChatInviteLink(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, _, _, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if ctx == nil || ctx.ChatInvitesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service is not configured"
	}
	invite, err := ctx.ChatInvitesClient.MessagesExportChatInvite(botRPCContext(r.Context(), botID), &mtproto.TLMessagesExportChatInvite{
		LegacyRevokePermanent: true,
		Peer:                  peer,
	})
	if err != nil || invite == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return invite.GetLink(), http.StatusOK, 0, ""
}

func createChatInviteLink(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, _, _, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	if ctx == nil || ctx.ChatInvitesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service is not configured"
	}
	expireDate, err := botAPIOptionalInt32(request, "expire_date", 1, 2147483647)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: expire_date is invalid"
	}
	memberLimit, err := botAPIOptionalInt32(request, "member_limit", 1, 99999)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: member_limit is invalid"
	}
	name := request.string("name")
	if utf8.RuneCountInString(name) > 32 {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is too long"
	}
	requestNeeded, err := request.bool("creates_join_request")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: creates_join_request is invalid"
	}
	invite, err := ctx.ChatInvitesClient.MessagesExportChatInvite(botRPCContext(r.Context(), botID), &mtproto.TLMessagesExportChatInvite{
		Peer:          peer,
		RequestNeeded: requestNeeded,
		ExpireDate:    expireDate,
		UsageLimit:    memberLimit,
		Title:         optionalString(name),
	})
	if err != nil || invite == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	creator := botAPIInviteCreator(ctx, r.Context(), botID, invite.GetAdminId())
	return botAPIInviteLinkFromMTProto(invite, creator), http.StatusOK, 0, ""
}

func editChatInviteLink(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editChatInviteLinkWithRevoked(ctx, r, botID, request, false)
}

func revokeChatInviteLink(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editChatInviteLinkWithRevoked(ctx, r, botID, request, true)
}

func editChatInviteLinkWithRevoked(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, revoked bool) (any, int, int, string) {
	peer, _, _, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	link := request.string("invite_link")
	if link == "" {
		return nil, http.StatusBadRequest, 400, "Bad Request: invite_link is required"
	}
	if ctx == nil || ctx.ChatInvitesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service is not configured"
	}
	expireDate, err := botAPIOptionalInt32(request, "expire_date", 1, 2147483647)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: expire_date is invalid"
	}
	memberLimit, err := botAPIOptionalInt32(request, "member_limit", 1, 99999)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: member_limit is invalid"
	}
	name := request.string("name")
	if utf8.RuneCountInString(name) > 32 {
		return nil, http.StatusBadRequest, 400, "Bad Request: name is too long"
	}
	requestNeeded, err := request.bool("creates_join_request")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: creates_join_request is invalid"
	}
	edit := &mtproto.TLMessagesEditExportedChatInvite{
		Revoked:    revoked,
		Peer:       peer,
		Link:       link,
		ExpireDate: expireDate,
		UsageLimit: memberLimit,
		Title:      optionalString(name),
	}
	if request.raw("creates_join_request") != nil {
		edit.RequestNeeded = mtproto.ToBool(requestNeeded)
	}
	result, err := ctx.ChatInvitesClient.MessagesEditExportedChatInvite(botRPCContext(r.Context(), botID), edit)
	invite := botAPIExportedChatInvite(result)
	if err != nil || invite == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	creator := botAPIInviteCreator(ctx, r.Context(), botID, invite.GetAdminId())
	return botAPIInviteLinkFromMTProto(invite, creator), http.StatusOK, 0, ""
}

func handleChatJoinRequest(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, approved bool) (any, int, int, string) {
	peer, _, _, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if ctx == nil || ctx.ChatInvitesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: chat invite service is not configured"
	}
	_, err = ctx.ChatInvitesClient.MessagesHideChatJoinRequest(botRPCContext(r.Context(), botID), &mtproto.TLMessagesHideChatJoinRequest{
		Approved: approved,
		Peer:     peer,
		UserId:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: userID}).To_InputUser(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIForumPeer(r *http.Request, ctx *svc.ServiceContext, botID int64, request *botAPIRequest) (*mtproto.InputPeer, int32, int64, error) {
	peer, peerType, peerID, err := botAPIInvitePeer(r, ctx, botID, request)
	if err != nil {
		return nil, 0, 0, err
	}
	return peer, peerType, peerID, nil
}

func botAPIForumChannel(peerID int64) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel()
}

func botAPIForumTopicFromMTProto(topic *mtproto.ForumTopic) *botapi.ForumTopic {
	if topic == nil {
		return nil
	}
	result := &botapi.ForumTopic{
		MessageThreadID: int(topic.GetId()),
		Name:            topic.GetTitle(),
		IconColor:       int(topic.GetIconColor()),
	}
	if topic.GetIconEmojiId() != nil && topic.GetIconEmojiId().GetValue() != 0 {
		result.IconCustomEmojiID = strconv.FormatInt(topic.GetIconEmojiId().GetValue(), 10)
	}
	return result
}

func botAPIForumTopics(ctx *svc.ServiceContext, requestCtx context.Context, botID int64, peerType int32, peerID int64, limit int32) (*mtproto.Messages_ForumTopics, error) {
	if ctx == nil || ctx.ForumsClient == nil {
		return nil, errors.New("forum service is not configured")
	}
	callCtx := botRPCContext(requestCtx, botID)
	if peerType == mtproto.PEER_CHANNEL {
		result, err := ctx.ForumsClient.ChannelsGetForumTopics(callCtx, &mtproto.TLChannelsGetForumTopics{Channel: botAPIForumChannel(peerID), Limit: limit})
		return result, err
	}
	result, err := ctx.ForumsClient.MessagesGetForumTopics(callCtx, &mtproto.TLMessagesGetForumTopics{
		Peer: botAPIInputPeer(botID, peerType, peerID), Limit: limit,
	})
	return result, err
}

func parseForumTopicOptions(request *botAPIRequest) (string, *wrapperspb.Int32Value, *wrapperspb.Int64Value, error) {
	title := request.string("name")
	if title == "" || utf8.RuneCountInString(title) > 128 {
		return "", nil, nil, errors.New("invalid topic name")
	}
	iconColor, err := botAPIOptionalInt32(request, "icon_color", 0, 0xFFFFFF)
	if err != nil {
		return "", nil, nil, err
	}
	var iconEmoji *wrapperspb.Int64Value
	if raw := request.raw("icon_custom_emoji_id"); len(raw) > 0 && string(raw) != "null" && request.string("icon_custom_emoji_id") != "" {
		id, parseErr := request.int64("icon_custom_emoji_id")
		if parseErr != nil || id <= 0 {
			return "", nil, nil, errors.New("invalid icon_custom_emoji_id")
		}
		iconEmoji = wrapperspb.Int64(id)
	}
	return title, iconColor, iconEmoji, nil
}

func createForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, peerType, peerID, err := botAPIForumPeer(r, ctx, botID, request)
	if err != nil || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	title, iconColor, iconEmoji, err := parseForumTopicOptions(request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	if ctx == nil || ctx.ForumsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: forum service is not configured"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		_, err = ctx.ForumsClient.ChannelsCreateForumTopic(callCtx, &mtproto.TLChannelsCreateForumTopic{
			Channel: botAPIForumChannel(peerID), Title: title, IconColor: iconColor, IconEmojiId: iconEmoji, RandomId: randomMessageID(),
		})
	} else {
		_, err = ctx.ForumsClient.MessagesCreateForumTopic(callCtx, &mtproto.TLMessagesCreateForumTopic{
			Peer: peer, Title: title, IconColor: iconColor, IconEmojiId: iconEmoji, RandomId: randomMessageID(),
		})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	// The Layer 229 create method returns Updates. The APIFull forum adapter
	// persists the topic transactionally, so read it back to return the actual
	// topic id rather than inventing a Bot API thread id.
	topics, err := botAPIForumTopics(ctx, r.Context(), botID, peerType, peerID, 100)
	if err != nil || topics == nil {
		return nil, http.StatusInternalServerError, 500, rpcDescription(err)
	}
	var selected *mtproto.ForumTopic
	for _, topic := range topics.GetTopics() {
		if topic != nil && topic.GetTitle() == title && (selected == nil || topic.GetId() > selected.GetId()) {
			selected = topic
		}
	}
	if selected == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: created forum topic was not readable"
	}
	return botAPIForumTopicFromMTProto(selected), http.StatusOK, 0, ""
}

func editForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, closedValue, setClosed, hiddenValue, setHidden bool) (any, int, int, string) {
	peer, peerType, peerID, err := botAPIForumPeer(r, ctx, botID, request)
	if err != nil || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	topicID, err := request.int64("message_thread_id")
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	if topicID <= 0 && setHidden {
		// Telegram's general topic has a stable thread id of 1.
		topicID = 1
	}
	if topicID <= 0 || topicID > 2147483647 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	if ctx == nil || ctx.ForumsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: forum service is not configured"
	}
	var title *wrapperspb.StringValue
	if raw := request.raw("name"); len(raw) > 0 && request.string("name") != "" {
		name := request.string("name")
		if utf8.RuneCountInString(name) > 128 {
			return nil, http.StatusBadRequest, 400, "Bad Request: name is too long"
		}
		title = wrapperspb.String(name)
	}
	var iconEmoji *wrapperspb.Int64Value
	if raw := request.raw("icon_custom_emoji_id"); len(raw) > 0 && string(raw) != "null" {
		value := request.string("icon_custom_emoji_id")
		if value != "" {
			id, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil || id <= 0 {
				return nil, http.StatusBadRequest, 400, "Bad Request: icon_custom_emoji_id is invalid"
			}
			iconEmoji = wrapperspb.Int64(id)
		} else {
			iconEmoji = wrapperspb.Int64(0)
		}
	}
	var closed, hidden *mtproto.Bool
	if setClosed {
		closed = mtproto.ToBool(closedValue)
	}
	if setHidden {
		hidden = mtproto.ToBool(hiddenValue)
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		_, err = ctx.ForumsClient.ChannelsEditForumTopic(callCtx, &mtproto.TLChannelsEditForumTopic{
			Channel: botAPIForumChannel(peerID), TopicId: int32(topicID), Title: title, IconEmojiId: iconEmoji, Closed: closed, Hidden: hidden,
		})
	} else {
		_, err = ctx.ForumsClient.MessagesEditForumTopic(callCtx, &mtproto.TLMessagesEditForumTopic{
			Peer: peer, TopicId: int32(topicID), Title: title, IconEmojiId: iconEmoji, Closed: closed, Hidden: hidden,
		})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func closeForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editForumTopic(ctx, r, botID, request, true, true, false, false)
}

func reopenForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editForumTopic(ctx, r, botID, request, false, true, false, false)
}

func hideGeneralForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editForumTopic(ctx, r, botID, request, false, false, true, true)
}

func unhideGeneralForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editForumTopic(ctx, r, botID, request, false, false, false, true)
}

func deleteForumTopic(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, peerType, peerID, err := botAPIForumPeer(r, ctx, botID, request)
	if err != nil || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	topicID, err := request.int64("message_thread_id")
	if err != nil || topicID <= 0 || topicID > 2147483647 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	if ctx == nil || ctx.ForumsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: forum service is not configured"
	}
	callCtx := botRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		_, err = ctx.ForumsClient.ChannelsDeleteTopicHistory(callCtx, &mtproto.TLChannelsDeleteTopicHistory{Channel: botAPIForumChannel(peerID), TopMsgId: int32(topicID)})
	} else {
		_, err = ctx.ForumsClient.MessagesDeleteTopicHistory(callCtx, &mtproto.TLMessagesDeleteTopicHistory{Peer: peer, TopMsgId: int32(topicID)})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func unpinAllForumTopicMessages(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	peer, peerType, _, err := botAPIForumPeer(r, ctx, botID, request)
	if err != nil || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	topicID, err := request.int64("message_thread_id")
	if err != nil || topicID <= 0 || topicID > 2147483647 {
		return nil, http.StatusBadRequest, 400, "Bad Request: message_thread_id is invalid"
	}
	if ctx == nil || ctx.MessagesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: message service is not configured"
	}
	_, err = ctx.MessagesClient.MessagesUnpinAllMessages(botRPCContext(r.Context(), botID), &mtproto.TLMessagesUnpinAllMessages{
		Peer: peer, TopMsgId: wrapperspb.Int32(int32(topicID)),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}
