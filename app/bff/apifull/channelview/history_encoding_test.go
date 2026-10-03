package channelview

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelMessageEncodingIncludesSharedViewsFields(t *testing.T) {
	row := domain.ChannelMessage{ChannelID: 123, Sender: 456, MessageID: 7, Pts: 9, Date: 1700000000, Text: "encoding"}
	msg := messageOf(row, 0)
	if msg.GetViews() == nil || msg.GetForwards() == nil {
		t.Fatalf("channel message must encode both shared views/forwards fields: views=%v forwards=%v", msg.GetViews(), msg.GetForwards())
	}

	buf := mtproto.NewEncodeBuf(1024)
	if err := msg.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	decode := mtproto.NewDecodeBuf(buf.GetBuf())
	decoded := decode.Object()
	if err := decode.GetError(); err != nil {
		t.Fatal(err)
	}
	if decode.GetOffset() != decode.GetSize() {
		t.Fatalf("message left %d bytes", decode.GetSize()-decode.GetOffset())
	}
	message, ok := decoded.(*mtproto.TLMessage)
	if !ok || message.GetViews() == nil || message.GetForwards() == nil {
		t.Fatalf("decoded channel message lost shared views/forwards fields: %#v", decoded)
	}
}
