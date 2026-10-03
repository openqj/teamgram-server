package dccontext

import (
	"context"
	"testing"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestWithOutgoingDCIDRoundTripsThroughIncomingMetadata(t *testing.T) {
	outgoing := WithOutgoingDCID(context.Background(), 2)
	md, ok := grpcmetadata.FromOutgoingContext(outgoing)
	if !ok {
		t.Fatal("missing outgoing metadata")
	}
	incoming := grpcmetadata.NewIncomingContext(context.Background(), md)
	got, ok := DCID(incoming)
	if !ok || got != 2 {
		t.Fatalf("DCID() = (%d, %v), want (2, true)", got, ok)
	}
}

func TestDCIDRejectsAmbiguousOrInvalidMetadata(t *testing.T) {
	tests := []struct {
		name string
		md   grpcmetadata.MD
	}{
		{name: "missing", md: grpcmetadata.MD{}},
		{name: "duplicate", md: grpcmetadata.Pairs(metadataKey, "1", metadataKey, "2")},
		{name: "zero", md: grpcmetadata.Pairs(metadataKey, "0")},
		{name: "invalid", md: grpcmetadata.Pairs(metadataKey, "dc2")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DCID(grpcmetadata.NewIncomingContext(context.Background(), tt.md))
			if ok || got != 0 {
				t.Fatalf("DCID() = (%d, %v), want (0, false)", got, ok)
			}
		})
	}
}
