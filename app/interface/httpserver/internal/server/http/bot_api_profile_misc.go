package http

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botAPIInputProfilePhoto struct {
	Type               string  `json:"type"`
	Photo              string  `json:"photo"`
	Animation          string  `json:"animation"`
	MainFrameTimestamp float64 `json:"main_frame_timestamp"`
}

func setMyProfilePhoto(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile photo service is not configured"
	}
	if request == nil || len(request.raw("photo")) == 0 || string(request.raw("photo")) == "null" {
		return nil, http.StatusBadRequest, 400, "Bad Request: photo is required"
	}
	var profile botAPIInputProfilePhoto
	if err := request.json("photo", &profile); err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: photo is invalid"
	}
	profile.Type = strings.ToLower(strings.TrimSpace(profile.Type))
	if profile.Type != "static" && profile.Type != "animated" {
		return nil, http.StatusBadRequest, 400, "Bad Request: photo type is invalid"
	}
	reference := profile.Photo
	if profile.Type == "animated" {
		reference = profile.Animation
	}
	if !strings.HasPrefix(reference, "attach://") {
		return nil, http.StatusBadRequest, 400, "Bad Request: profile photo must be a new multipart upload"
	}
	file, err := uploadBotAPIProfilePhotoFile(r, ctx, botID, reference, profile.Type)
	if err != nil {
		return nil, http.StatusBadRequest, 400, "Bad Request: " + err.Error()
	}
	upload := &mtproto.TLPhotosUploadProfilePhoto{}
	if profile.Type == "static" {
		upload.File = file
	} else {
		if math.IsNaN(profile.MainFrameTimestamp) || math.IsInf(profile.MainFrameTimestamp, 0) || profile.MainFrameTimestamp < 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: main_frame_timestamp is invalid"
		}
		upload.Video = file
		if profile.MainFrameTimestamp > 0 {
			upload.VideoStartTs = wrapperspb.Double(profile.MainFrameTimestamp)
		}
	}
	result, err := ctx.UserChannelProfilesClient.PhotosUploadProfilePhoto(botMediaRPCContext(r.Context(), botID), upload)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: profile photo was not returned"
	}
	return true, http.StatusOK, 0, ""
}

func removeMyProfilePhoto(ctx *svc.ServiceContext, r *http.Request, bot *mtproto.ImmutableUser) (any, int, int, string) {
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile photo service is not configured"
	}
	if bot == nil || bot.GetUser() == nil {
		return nil, http.StatusUnauthorized, 401, "Unauthorized"
	}
	photo := bot.GetUser().GetProfilePhoto()
	if photo == nil || photo.GetId() <= 0 {
		return true, http.StatusOK, 0, ""
	}
	_, err := ctx.UserChannelProfilesClient.PhotosDeletePhotos(botRPCContext(r.Context(), bot.GetUser().GetId()), &mtproto.TLPhotosDeletePhotos{
		Id: []*mtproto.InputPhoto{mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
			Id:            photo.GetId(),
			AccessHash:    photo.GetAccessHash(),
			FileReference: photo.GetFileReference(),
		}).To_InputPhoto()},
	})
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func uploadBotAPIProfilePhotoFile(r *http.Request, ctx *svc.ServiceContext, botID int64, reference, fallbackName string) (*mtproto.InputFile, error) {
	return uploadBotAPIProfilePhotoFileWithContext(r, ctx, botMediaRPCContext(requestContext(r), botID), botID, reference, fallbackName)
}

func uploadBotAPIProfilePhotoFileWithContext(r *http.Request, ctx *svc.ServiceContext, callCtx context.Context, creatorID int64, reference, fallbackName string) (*mtproto.InputFile, error) {
	if r == nil {
		return nil, errors.New("multipart upload is required")
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, errors.New("invalid multipart form")
	}
	field := strings.TrimPrefix(reference, "attach://")
	if strings.TrimSpace(field) == "" {
		return nil, errors.New("profile photo attachment is empty")
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, errors.New("profile photo attachment is missing")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64<<20+1))
	if err != nil {
		return nil, errors.New("failed to read profile photo")
	}
	if len(data) == 0 || len(data) > 64<<20 {
		return nil, errors.New("profile photo is too large or empty")
	}
	name := filepath.Base(header.Filename)
	if name == "." || name == "" {
		name = fallbackName
	}
	return uploadBotAPIFileWithContext(callCtx, ctx, creatorID, name, data)
}

func getUserProfileAudios(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	offset, err := request.int("offset")
	if err != nil || offset < 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: offset is invalid"
	}
	limit := 100
	if request.has("limit") {
		limit, err = request.int("limit")
		if err != nil || limit < 1 || limit > 100 {
			return nil, http.StatusBadRequest, 400, "Bad Request: limit is invalid"
		}
	}
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile audio service is not configured"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, r.Context(), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	audios, err := ctx.UserChannelProfilesClient.UsersGetSavedMusic(botRPCContext(r.Context(), botID), &mtproto.TLUsersGetSavedMusic{
		Id: inputUser, Offset: int32(offset), Limit: int32(limit),
	})
	if err != nil || audios == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botapi.UserProfileAudiosFromMTProto(audios), http.StatusOK, 0, ""
}

func getCustomEmojiStickers(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	var rawIDs []string
	if request == nil || request.json("custom_emoji_ids", &rawIDs) != nil || len(rawIDs) == 0 || len(rawIDs) > 200 {
		return nil, http.StatusBadRequest, 400, "Bad Request: custom_emoji_ids must contain between 1 and 200 identifiers"
	}
	ids := make([]int64, len(rawIDs))
	for i, rawID := range rawIDs {
		id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
		if err != nil || id <= 0 {
			return nil, http.StatusBadRequest, 400, "Bad Request: custom_emoji_ids contains an invalid identifier"
		}
		ids[i] = id
	}
	if ctx == nil || ctx.CustomEmojisClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: custom emoji service is not configured"
	}
	documents, err := ctx.CustomEmojisClient.MessagesGetCustomEmojiDocuments(botRPCContext(r.Context(), botID), &mtproto.TLMessagesGetCustomEmojiDocuments{DocumentId: ids})
	if err != nil || documents == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botapi.CustomEmojiStickersFromMTProto(documents.GetDatas()), http.StatusOK, 0, ""
}

// getForumTopicIconStickers returns the custom emoji catalog Telegram exposes
// for forum topic icons. The Layer 229 surface names this catalog
// account.getDefaultGroupPhotoEmojis; its document vector is the same Sticker
// representation used by getCustomEmojiStickers.
func getForumTopicIconStickers(ctx *svc.ServiceContext, r *http.Request, botID int64) (any, int, int, string) {
	if ctx == nil || ctx.CustomEmojisClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: custom emoji service is not configured"
	}
	callCtx := botRPCContext(requestContext(r), botID)
	emojis, err := ctx.CustomEmojisClient.AccountGetDefaultGroupPhotoEmojis(callCtx, &mtproto.TLAccountGetDefaultGroupPhotoEmojis{})
	if err != nil || emojis == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	ids := emojis.GetDocumentId()
	if len(ids) == 0 {
		return []*botapi.Sticker{}, http.StatusOK, 0, ""
	}
	documents, err := ctx.CustomEmojisClient.MessagesGetCustomEmojiDocuments(callCtx, &mtproto.TLMessagesGetCustomEmojiDocuments{DocumentId: ids})
	if err != nil || documents == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return botapi.CustomEmojiStickersFromMTProto(documents.GetDatas()), http.StatusOK, 0, ""
}

func getUserPersonalChatMessages(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	userID, err := request.int64("user_id")
	if err != nil || userID <= 0 {
		return nil, http.StatusBadRequest, 400, "Bad Request: user_id is invalid"
	}
	if request == nil || !request.has("limit") {
		return nil, http.StatusBadRequest, 400, "Bad Request: limit is required"
	}
	limit, err := request.int("limit")
	if err != nil || limit < 1 || limit > 20 {
		return nil, http.StatusBadRequest, 400, "Bad Request: limit is invalid"
	}
	if ctx == nil || ctx.ChannelsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: channel service is not configured"
	}
	inputUser, err := botAPIProfilePhotoInputUser(ctx, requestContext(r), botID, userID)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	result, err := ctx.ChannelsClient.MessagesGetPersonalChannelHistory(botRPCContext(requestContext(r), botID), &mtproto.TLMessagesGetPersonalChannelHistory{
		UserId: inputUser,
		Limit:  int32(limit),
	})
	if err != nil || result == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	messages := make([]*botapi.Message, 0, len(result.GetMessages()))
	for _, message := range result.GetMessages() {
		if converted := botapi.MessageFromMTProto(message, result.GetUsers(), result.GetChats()); converted != nil {
			messages = append(messages, converted)
		}
	}
	return messages, http.StatusOK, 0, ""
}

func requestContext(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}
