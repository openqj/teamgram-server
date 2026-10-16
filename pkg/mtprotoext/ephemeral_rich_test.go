package mtprotoext

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

func TestEphemeralRichMessageRoundTrip(t *testing.T) {
	message := &mtproto.EphemeralMessage{Id: 7}
	rich := mtproto.MakeTLRichMessage(&mtproto.RichMessage{Rtl: true}).To_RichMessage()
	if err := SetEphemeralRichMessage(message, rich); err != nil {
		t.Fatal(err)
	}
	got, err := EphemeralRichMessage(message)
	if err != nil || got == nil || !got.GetRtl() {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	wire, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(mtproto.EphemeralMessage)
	if err := proto.Unmarshal(wire, decoded); err != nil {
		t.Fatal(err)
	}
	got, err = EphemeralRichMessage(decoded)
	if err != nil || got == nil || !got.GetRtl() {
		t.Fatalf("protobuf round trip = %+v, %v", got, err)
	}

	if err := SetEphemeralRichMessage(message, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := EphemeralRichMessage(message); err != nil || got != nil {
		t.Fatalf("cleared sidecar = %+v, %v", got, err)
	}
}

func TestEphemeralRichMessagePreservesOtherUnknownFields(t *testing.T) {
	message := &mtproto.EphemeralMessage{}
	message.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	if err := SetEphemeralRichMessage(message, mtproto.MakeTLRichMessage(&mtproto.RichMessage{}).To_RichMessage()); err != nil {
		t.Fatal(err)
	}
	if got := message.ProtoReflect().GetUnknown(); len(got) == 0 || got[0] != 0x98 {
		t.Fatalf("unknown field was not preserved: %x", got)
	}
}

func TestEphemeralRichMessageDoesNotChangeTLWire(t *testing.T) {
	base := &mtproto.EphemeralMessage{
		PredicateName: mtproto.Predicate_ephemeralMessage,
		Constructor:   mtproto.TLConstructor_CRC32_ephemeralMessage,
		Id:            7,
		FromId:        mtproto.MakePeerUser(100),
		PeerId:        mtproto.MakePeerUser(200),
		Message:       "draft",
	}
	encode := func(message *mtproto.EphemeralMessage) []byte {
		buf := mtproto.NewEncodeBuf(128)
		if err := message.To_EphemeralMessage().Encode(buf, 229); err != nil {
			t.Fatal(err)
		}
		return append([]byte(nil), buf.GetBuf()...)
	}
	want := encode(base)
	if err := SetEphemeralRichMessage(base, mtproto.MakeTLRichMessage(&mtproto.RichMessage{Rtl: true}).To_RichMessage()); err != nil {
		t.Fatal(err)
	}
	got := encode(base)
	if string(got) != string(want) {
		t.Fatalf("TL wire changed: before=%x after=%x", want, got)
	}
}
