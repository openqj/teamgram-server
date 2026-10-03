package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsPhoneNumberUnoccupiedError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "documented mtproto error", err: mtproto.ErrPhoneNumberUnoccupied, want: true},
		{name: "grpc not found", err: status.Error(codes.NotFound, "missing"), want: true},
		{name: "backend unavailable", err: status.Error(codes.Unavailable, "user service down")},
		{name: "backend internal", err: status.Error(codes.Internal, "database failed")},
		{name: "non status error", err: errors.New("lookup failed")},
		{name: "wrong message for mtproto code", err: status.Error(status.Code(mtproto.ErrPhoneNumberUnoccupied), "different")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPhoneNumberUnoccupiedError(tc.err); got != tc.want {
				t.Fatalf("isPhoneNumberUnoccupiedError(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}
