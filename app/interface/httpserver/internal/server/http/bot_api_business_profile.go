// Copyright 2026 Teamgram Authors
// All rights reserved.

package http

import (
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/businessctx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func botAPIBusinessProfileScope(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest, allowed func(businessctx.Scope) bool) (businessctx.Scope, int, int, string) {
	connectionID := strings.TrimSpace(request.string("business_connection_id"))
	if connectionID == "" || len(connectionID) > 128 {
		return businessctx.Scope{}, http.StatusBadRequest, 400, "Bad Request: business_connection_id is invalid"
	}
	scope, err := resolveBotAPIBusinessScope(ctx, requestContext(r), botID, connectionID)
	if err != nil {
		return businessctx.Scope{}, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if allowed != nil && !allowed(scope) {
		return businessctx.Scope{}, http.StatusBadRequest, 400, "Bad Request: business connection does not grant this right"
	}
	return scope, 0, 0, ""
}

func setBusinessAccountName(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanEditName })
	if code != 0 {
		return nil, status, code, description
	}
	firstName := strings.TrimSpace(request.string("first_name"))
	if firstName == "" || len([]rune(firstName)) > 64 {
		return nil, http.StatusBadRequest, 400, "Bad Request: first_name is invalid"
	}
	update := &mtproto.TLAccountUpdateProfile{FirstName: wrapperspb.String(firstName)}
	if request.has("last_name") {
		lastName := strings.TrimSpace(request.string("last_name"))
		if len([]rune(lastName)) > 64 {
			return nil, http.StatusBadRequest, 400, "Bad Request: last_name is invalid"
		}
		update.LastName = wrapperspb.String(lastName)
	}
	if _, err := ctx.UserChannelProfilesClient.AccountUpdateProfile(businessBotCallContext(requestContext(r), botID, scope), update); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setBusinessAccountUsername(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UsernamesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: username service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanEditUsername })
	if code != 0 {
		return nil, status, code, description
	}
	if !request.has("username") {
		return nil, http.StatusBadRequest, 400, "Bad Request: username is required"
	}
	username := strings.TrimSpace(request.string("username"))
	if len([]rune(username)) > 32 {
		return nil, http.StatusBadRequest, 400, "Bad Request: username is invalid"
	}
	if _, err := ctx.UsernamesClient.AccountUpdateUsername(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLAccountUpdateUsername{Username: username}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setBusinessAccountBio(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanEditBio })
	if code != 0 {
		return nil, status, code, description
	}
	if !request.has("bio") {
		return nil, http.StatusBadRequest, 400, "Bad Request: bio is required"
	}
	bio := request.text("bio")
	if len([]rune(bio)) > 140 {
		return nil, http.StatusBadRequest, 400, "Bad Request: bio is invalid"
	}
	if _, err := ctx.UserChannelProfilesClient.AccountUpdateProfile(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLAccountUpdateProfile{About: wrapperspb.String(bio)}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func setBusinessAccountProfilePhoto(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UserChannelProfilesClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile photo service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanEditPhoto })
	if code != 0 {
		return nil, status, code, description
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
	callCtx := businessBotMediaCallContext(requestContext(r), botID, scope)
	// The temporary DFS upload is authenticated as the bot. The profile RPC
	// uses the business scope to apply the resulting photo to the owner, but
	// media.uploadProfilePhotoFile reads the staged file by PermAuthKeyId.
	file, err := uploadBotAPIProfilePhotoFileWithContext(r, ctx, callCtx, botID, reference, profile.Type)
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
	result, err := ctx.UserChannelProfilesClient.PhotosUploadProfilePhoto(callCtx, upload)
	if err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: profile photo was not returned"
	}
	return true, http.StatusOK, 0, ""
}

func removeBusinessAccountProfilePhoto(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.UserChannelProfilesClient == nil || ctx.UserClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: profile photo service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanEditPhoto })
	if code != 0 {
		return nil, status, code, description
	}
	owner, err := ctx.UserClient.UserGetImmutableUser(botRPCContext(requestContext(r), botID), &userpb.TLUserGetImmutableUser{Id: scope.OwnerID})
	if err != nil || owner == nil || owner.GetUser() == nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	photo := owner.GetUser().GetProfilePhoto()
	if photo == nil || photo.GetId() <= 0 {
		return true, http.StatusOK, 0, ""
	}
	if _, err = ctx.UserChannelProfilesClient.PhotosDeletePhotos(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLPhotosDeletePhotos{Id: []*mtproto.InputPhoto{mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
		Id: photo.GetId(), AccessHash: photo.GetAccessHash(), FileReference: photo.GetFileReference(),
	}).To_InputPhoto()}}); err != nil {
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	return true, http.StatusOK, 0, ""
}

func getBusinessAccountStarBalance(ctx *svc.ServiceContext, r *http.Request, botID int64, request *botAPIRequest) (any, int, int, string) {
	if ctx == nil || ctx.StarsClient == nil {
		return nil, http.StatusNotImplemented, 501, "Not Implemented: payment service is not configured"
	}
	scope, status, code, description := botAPIBusinessProfileScope(ctx, r, botID, request, func(scope businessctx.Scope) bool { return scope.CanTransferStars })
	if code != 0 {
		return nil, status, code, description
	}
	result, err := ctx.StarsClient.PaymentsGetStarsStatus(businessBotCallContext(requestContext(r), botID, scope), &mtproto.TLPaymentsGetStarsStatus{Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()})
	if err != nil {
		if errors.Is(err, mtproto.ErrMethodNotImpl) {
			return botapi.StarAmount{}, http.StatusOK, 0, ""
		}
		return nil, http.StatusBadRequest, 400, rpcDescription(err)
	}
	if result == nil {
		return nil, http.StatusInternalServerError, 500, "Internal Server Error: Stars balance was not returned"
	}
	return botapi.StarAmount{Amount: result.GetBalance_INT64(), NanostarAmount: starsNanostarAmount(result.GetBalance_STARSAMOUNT())}, http.StatusOK, 0, ""
}
