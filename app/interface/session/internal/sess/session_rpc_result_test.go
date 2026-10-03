package sess

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

type encodeFailResult struct{ err error }

func (r encodeFailResult) Encode(x *mtproto.EncodeBuf, _ int32) error {
	x.UInt(0xdeadbeef)
	return r.err
}

func (r encodeFailResult) Decode(*mtproto.DecodeBuf) error { return r.err }
func (encodeFailResult) String() string                    { return "encode failure" }

func TestEncodeRpcResultBodyReturnsEncodeError(t *testing.T) {
	want := errors.New("encode failed")
	if _, err := encodeRpcResultBody(1, encodeFailResult{err: want}, 229); !errors.Is(err, want) {
		t.Fatalf("encodeRpcResultBody() error = %v, want %v", err, want)
	}
}
