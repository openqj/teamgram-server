package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAicomposeToneInputValidationAndAccessHash(t *testing.T) {
	uid := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	created, err := c.AicomposeCreateTone(&mtproto.TLAicomposeCreateTone{Title: "warm"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.AicomposeGetTone(&mtproto.TLAicomposeGetTone{Tone: &mtproto.InputAiComposeTone{
		Id: created.GetId(), AccessHash: created.GetAccessHash() + 1,
	}})
	if err != nil || got == nil || len(got.GetTones()) != 0 {
		t.Fatalf("wrong hash lookup = (%+v, %v), want empty result", got, err)
	}
	if _, err = c.AicomposeGetTone(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil get tone error = %v", err)
	}
	if _, err = c.AicomposeDeleteTone(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil delete tone error = %v", err)
	}
}
