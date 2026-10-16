// Copyright 2026 Teamgram Authors
// All rights reserved.

// Package businessctx carries the trusted business-connection scope across
// internal RPC boundaries. The public MTProto request keeps the standard
// invokeWithBusinessConnection wrapper; this package only preserves the
// wrapper's authorization context while the request crosses BFF services.
package businessctx

import (
	"context"
	"strconv"

	grpcmetadata "google.golang.org/grpc/metadata"
)

const (
	connectionMetadataKey    = "x-teamgram-business-connection-id"
	ownerMetadataKey         = "x-teamgram-business-owner-id"
	replyMetadataKey         = "x-teamgram-business-can-reply"
	readMetadataKey          = "x-teamgram-business-can-read"
	deleteSentMetadataKey    = "x-teamgram-business-can-delete-sent"
	deleteAllMetadataKey     = "x-teamgram-business-can-delete-all"
	editNameMetadataKey      = "x-teamgram-business-can-edit-name"
	editBioMetadataKey       = "x-teamgram-business-can-edit-bio"
	editPhotoMetadataKey     = "x-teamgram-business-can-edit-profile-photo"
	editUsernameMetadataKey  = "x-teamgram-business-can-edit-username"
	viewGiftsMetadataKey     = "x-teamgram-business-can-view-gifts"
	sellGiftsMetadataKey     = "x-teamgram-business-can-sell-gifts"
	changeGiftsMetadataKey   = "x-teamgram-business-can-change-gift-settings"
	transferGiftsMetadataKey = "x-teamgram-business-can-transfer-and-upgrade-gifts"
	transferStarsMetadataKey = "x-teamgram-business-can-transfer-stars"
)

type contextKey struct{}

// Scope is the authoritative connection and rights selected by the business
// connection registry. It is intentionally small and only includes rights
// needed by message operations.
type Scope struct {
	ConnectionID     string
	OwnerID          int64
	CanReply         bool
	CanReadMessages  bool
	CanDeleteSent    bool
	CanDeleteAll     bool
	CanEditName      bool
	CanEditBio       bool
	CanEditPhoto     bool
	CanEditUsername  bool
	CanViewGifts     bool
	CanSellGifts     bool
	CanChangeGifts   bool
	CanTransferGifts bool
	CanTransferStars bool
}

func Valid(scope Scope) bool {
	return scope.ConnectionID != "" && scope.OwnerID > 0
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	if ctx == nil || !Valid(scope) {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, scope)
}

// ScopeFromContext reads the local value first and then the incoming gRPC
// metadata used at a service boundary.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	if ctx == nil {
		return Scope{}, false
	}
	if scope, ok := ctx.Value(contextKey{}).(Scope); ok && Valid(scope) {
		return scope, true
	}
	md, ok := grpcmetadata.FromIncomingContext(ctx)
	if !ok {
		return Scope{}, false
	}
	scope := Scope{}
	scope.ConnectionID = single(md.Get(connectionMetadataKey))
	scope.OwnerID = parseInt64(single(md.Get(ownerMetadataKey)))
	scope.CanReply = parseBool(single(md.Get(replyMetadataKey)))
	scope.CanReadMessages = parseBool(single(md.Get(readMetadataKey)))
	scope.CanDeleteSent = parseBool(single(md.Get(deleteSentMetadataKey)))
	scope.CanDeleteAll = parseBool(single(md.Get(deleteAllMetadataKey)))
	scope.CanEditName = parseBool(single(md.Get(editNameMetadataKey)))
	scope.CanEditBio = parseBool(single(md.Get(editBioMetadataKey)))
	scope.CanEditPhoto = parseBool(single(md.Get(editPhotoMetadataKey)))
	scope.CanEditUsername = parseBool(single(md.Get(editUsernameMetadataKey)))
	scope.CanViewGifts = parseBool(single(md.Get(viewGiftsMetadataKey)))
	scope.CanSellGifts = parseBool(single(md.Get(sellGiftsMetadataKey)))
	scope.CanChangeGifts = parseBool(single(md.Get(changeGiftsMetadataKey)))
	scope.CanTransferGifts = parseBool(single(md.Get(transferGiftsMetadataKey)))
	scope.CanTransferStars = parseBool(single(md.Get(transferStarsMetadataKey)))
	return scope, Valid(scope)
}

// WithOutgoingScope preserves a scope when a service invokes the next gRPC
// hop. The local value is retained as well so chained calls can reuse it.
func WithOutgoingScope(ctx context.Context, scope Scope) context.Context {
	if ctx == nil || !Valid(scope) {
		return ctx
	}
	ctx = WithScope(ctx, scope)
	return grpcmetadata.AppendToOutgoingContext(ctx,
		connectionMetadataKey, scope.ConnectionID,
		ownerMetadataKey, strconv.FormatInt(scope.OwnerID, 10),
		replyMetadataKey, strconv.FormatBool(scope.CanReply),
		readMetadataKey, strconv.FormatBool(scope.CanReadMessages),
		deleteSentMetadataKey, strconv.FormatBool(scope.CanDeleteSent),
		deleteAllMetadataKey, strconv.FormatBool(scope.CanDeleteAll),
		editNameMetadataKey, strconv.FormatBool(scope.CanEditName),
		editBioMetadataKey, strconv.FormatBool(scope.CanEditBio),
		editPhotoMetadataKey, strconv.FormatBool(scope.CanEditPhoto),
		editUsernameMetadataKey, strconv.FormatBool(scope.CanEditUsername),
		viewGiftsMetadataKey, strconv.FormatBool(scope.CanViewGifts),
		sellGiftsMetadataKey, strconv.FormatBool(scope.CanSellGifts),
		changeGiftsMetadataKey, strconv.FormatBool(scope.CanChangeGifts),
		transferGiftsMetadataKey, strconv.FormatBool(scope.CanTransferGifts),
		transferStarsMetadataKey, strconv.FormatBool(scope.CanTransferStars),
	)
}

func single(values []string) string {
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
