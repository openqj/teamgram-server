// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"net/http"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
)

// banChatSenderChat blocks a channel from sending messages in a supergroup.
// Telegram represents the sender chat as an InputPeerChannel participant;
// channels.editBanned is the Layer 229 operation already used for member bans.
func banChatSenderChat(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editChatSenderBan(ctx, r, botID, request, true)
}

func unbanChatSenderChat(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	return editChatSenderBan(ctx, r, botID, request, false)
}

func editChatSenderBan(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, banned bool) (any, int, int, string) {
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	_, chatType, chatID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || chatID <= 0 || chatType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id must be a supergroup"
	}
	_, senderType, senderID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("sender_chat_id"))
	if err != nil || senderID <= 0 || senderType != mtproto.PEER_CHANNEL {
		return nil, http.StatusBadRequest, 400, "Bad Request: sender_chat_id must be a channel"
	}

	rights := &mtproto.ChatBannedRights{}
	if banned {
		rights = &mtproto.ChatBannedRights{
			ViewMessages:    true,
			SendMessages:    true,
			SendMedia:       true,
			SendStickers:    true,
			SendGifs:        true,
			SendGames:       true,
			SendInline:      true,
			EmbedLinks:      true,
			SendPolls:       true,
			ChangeInfo:      true,
			InviteUsers:     true,
			PinMessages:     true,
			ManageTopics:    true,
			SendPhotos:      true,
			SendVideos:      true,
			SendRoundvideos: true,
			SendAudios:      true,
			SendVoices:      true,
			SendDocs:        true,
			SendPlain:       true,
			SendReactions:   true,
		}
	}
	_, err = ctx.ChannelsClient.ChannelsEditBanned(botRPCContext(r.Context(), botID), &mtproto.TLChannelsEditBanned{
		Channel:      mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: chatID, AccessHash: chatID}).To_InputChannel(),
		Participant:  botAPIInputPeer(botID, senderType, senderID),
		BannedRights: mtproto.MakeTLChatBannedRights(rights).To_ChatBannedRights(),
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}
