// Copyright 2026 Teamgram Authors
// All rights reserved.

// Package dccontext carries the trusted DC identity selected by the MTProto
// gateway across internal gRPC calls. It intentionally uses a private context
// value plus a namespaced gRPC metadata key; no generated protobuf field is
// needed for this internal routing detail.
package dccontext

import (
	"context"
	"strconv"

	grpcmetadata "google.golang.org/grpc/metadata"
)

const metadataKey = "x-teamgram-source-dc-id"

type contextKey struct{}

// WithDCID records the trusted DC identity in a context. Values must come
// from the gateway transport decoder, never from client RPC arguments.
func WithDCID(ctx context.Context, dcID int32) context.Context {
	if ctx == nil || dcID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, dcID)
}

// DCID returns the DC identity carried in a context or its incoming gRPC
// metadata. The metadata fallback is needed at the receiving service boundary.
func DCID(ctx context.Context) (int32, bool) {
	if ctx == nil {
		return 0, false
	}
	if dcID, ok := ctx.Value(contextKey{}).(int32); ok && dcID > 0 {
		return dcID, true
	}
	md, ok := grpcmetadata.FromIncomingContext(ctx)
	if !ok {
		return 0, false
	}
	return parseValues(md.Get(metadataKey))
}

// WithOutgoingDCID appends the trusted DC identity to outgoing gRPC metadata
// while retaining it as a local context value for downstream calls.
func WithOutgoingDCID(ctx context.Context, dcID int32) context.Context {
	if ctx == nil || dcID <= 0 {
		return ctx
	}
	ctx = WithDCID(ctx, dcID)
	return grpcmetadata.AppendToOutgoingContext(ctx, metadataKey, strconv.FormatInt(int64(dcID), 10))
}

func parseValues(values []string) (int32, bool) {
	if len(values) != 1 {
		return 0, false
	}
	dcID, err := strconv.ParseInt(values[0], 10, 32)
	if err != nil || dcID <= 0 {
		return 0, false
	}
	return int32(dcID), true
}
