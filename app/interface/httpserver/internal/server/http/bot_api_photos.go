// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func getUserProfilePhotos(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	offset, err := request.int("offset")
	if err != nil || offset < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: offset is invalid"
	}
	limit := 100
	if len(request.raw("limit")) > 0 {
		limit, err = request.int("limit")
		if err != nil || limit < 1 || limit > 100 {
			return nil, http.StatusBadRequest, 400, "Bad Request: limit is invalid"
		}
	}
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile photo service is not configured"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	photos, err := ctx.UserChannelProfilesClient.PhotosGetUserPhotos(botRPCContext(r.Context(), botID), &mtproto.TLPhotosGetUserPhotos{
		UserId: inputUser,
		Offset: int32(offset),
		Limit:  int32(limit),
	})
	if err != nil || photos == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botapi.UserProfilePhotosFromMTProto(photos), http.StatusOK, 0, ""
}

func botAPIProfilePhotoInputUser(ctx *svc.ServiceContext, requestCtx context.Context, botID, userID int64) (*mtproto.InputUser, error) {
	if userID == botID {
		return mtproto.MakeTLInputUserSelf(nil).To_InputUser(), nil
	}
	if ctx == nil || ctx.UserClient == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	user, err := ctx.UserClient.UserGetImmutableUser(botRPCContext(requestCtx, botID), &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return nil, err
	}
	if user == nil || user.GetUser() == nil || user.Deleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	accessHash := user.GetUser().GetAccessHash()
	if accessHash == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: userID, AccessHash: accessHash}).To_InputUser(), nil
}

func setChatPhoto(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	photo, err := botAPIInputChatPhoto(r, ctx, botID, request)
	if err != nil {
		return nil, http.StatusBadRequest, 400, err.Error()
	}
	callCtx := botMediaRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		_, err = ctx.ChannelsClient.ChannelsEditPhoto(callCtx, &mtproto.TLChannelsEditPhoto{
			Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
			Photo:   photo,
		})
	} else {
		if ctx == nil || ctx.ChatsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
		}
		_, err = ctx.ChatsClient.MessagesEditChatPhoto(callCtx, &mtproto.TLMessagesEditChatPhoto{ChatId: peerID, Photo: photo})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func deleteChatPhoto(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	_, peerType, peerID, err := resolveBotAPIChatID(r.Context(), ctx, botID, request.string("chat_id"))
	if err != nil || peerID <= 0 || peerType == mtproto.PEER_USER {
		return nil, http.StatusBadRequest, 400, "Bad Request: chat_id is invalid"
	}
	photo := mtproto.MakeTLInputChatPhotoEmpty(nil).To_InputChatPhoto()
	callCtx := botMediaRPCContext(r.Context(), botID)
	if peerType == mtproto.PEER_CHANNEL {
		if ctx == nil || ctx.ChannelsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
		}
		_, err = ctx.ChannelsClient.ChannelsEditPhoto(callCtx, &mtproto.TLChannelsEditPhoto{
			Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: peerID, AccessHash: peerID}).To_InputChannel(),
			Photo:   photo,
		})
	} else {
		if ctx == nil || ctx.ChatsClient == nil {
			return nil, http.StatusNotImplemented, 501, "Not Implemented: chat service is not configured"
		}
		_, err = ctx.ChatsClient.MessagesEditChatPhoto(callCtx, &mtproto.TLMessagesEditChatPhoto{ChatId: peerID, Photo: photo})
	}
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func botAPIInputChatPhoto(r *http.Request, ctx *svc.ServiceContext, botID int64, request *botAPIRequest) (*mtproto.InputChatPhoto, error) {
	if r != nil && strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			return nil, errors.New("Bad Request: invalid multipart form")
		}
	}
	if file, header, err := r.FormFile("photo"); err == nil {
		defer file.Close()
		name := filepath.Base(header.Filename)
		if name == "." || name == "" {
			name = "photo"
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 64<<20+1))
		if readErr != nil {
			return nil, errors.New("Bad Request: failed to read uploaded photo")
		}
		if len(data) == 0 || len(data) > 64<<20 {
			return nil, errors.New("Bad Request: photo is too large or empty")
		}
		inputFile, uploadErr := uploadBotAPIFile(r.Context(), ctx, botID, name, data)
		if uploadErr != nil {
			return nil, errors.New(rpcDescription(uploadErr))
		}
		return mtproto.MakeTLInputChatUploadedPhoto(&mtproto.InputChatPhoto{File: inputFile}).To_InputChatPhoto(), nil
	}
	fileID := request.string("photo")
	ref, err := parseBotAPIFileID(fileID)
	if err != nil || ref.kind != "photo" {
		return nil, errors.New("Bad Request: photo is required and must be a photo file_id")
	}
	return mtproto.MakeTLInputChatPhoto(&mtproto.InputChatPhoto{
		Id: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: ref.id, AccessHash: ref.accessHash}).To_InputPhoto(),
	}).To_InputChatPhoto(), nil
}
