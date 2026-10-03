package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMessagesSummarizeTextDoesNotReturnOriginalAsSummary(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	request := &mtproto.TLMessagesSummarizeText{
		Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 100}).To_InputPeer(),
		Id:   7,
	}

	got, err := core.MessagesSummarizeText(request)
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesSummarizeText() = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestMessagesTranslateRichMessageDoesNotReturnOriginalAsTranslation(t *testing.T) {
	core := &MessagesCore{}
	text := mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{Html: "source text"}).To_InputRichMessage()
	request := &mtproto.TLMessagesTranslateRichMessage{
		Text:   []*mtproto.InputRichMessage{text},
		ToLang: "fr",
	}

	got, err := core.MessagesTranslateRichMessage(request)
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesTranslateRichMessage() = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestMessagesComposeRichMessageWithAIDoesNotReturnOriginalAsResult(t *testing.T) {
	core := &MessagesCore{}
	text := mtproto.MakeTLInputRichMessage(&mtproto.InputRichMessage{Html: "source text"}).To_InputRichMessage()
	request := &mtproto.TLMessagesComposeRichMessageWithAI{Text: text, Proofread: true}

	got, err := core.MessagesComposeRichMessageWithAI(request)
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("MessagesComposeRichMessageWithAI() = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}
