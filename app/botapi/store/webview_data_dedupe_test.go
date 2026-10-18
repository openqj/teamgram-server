package store

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi"
)

func webViewDataUpdateForDedupe(callerID, randomID int64) *mtproto.Update {
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Id:          17,
		PeerId:      mtproto.MakePeerUser(callerID),
		FromId:      mtproto.MakePeerUser(callerID),
		Action:      mtproto.MakeMessageActionWebViewDataSentMe("Open", "payload"),
		SavedPeerId: mtproto.MakePeerUser(randomID),
	}).To_Message()
	return mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: message}).To_Update()
}

func TestBotUpdateDedupeKeyWebViewDataUsesCallerAndRandomID(t *testing.T) {
	update := webViewDataUpdateForDedupe(42, 1001)
	first := botUpdateDedupeKey(update)
	if first != "web_app_data:42:1001" {
		t.Fatalf("web app data dedupe key = %q", first)
	}
	converted, ok := botapi.UpdateFromMTProtoForBot(update, nil, nil, 900000000001)
	if !ok || converted == nil || converted.Message == nil || converted.Message.WebAppData == nil {
		t.Fatalf("web app data update conversion = (%#v, %v)", converted, ok)
	}
	if converted.Message.ViaBot != nil || converted.Message.SenderBusinessBot != nil {
		t.Fatalf("internal random_id marker leaked into Bot API message: %#v", converted.Message)
	}
	if got := botUpdateDedupeKey(webViewDataUpdateForDedupe(42, 1002)); got == first {
		t.Fatalf("different random_id reused dedupe key %q", got)
	}
	if got := botUpdateDedupeKey(webViewDataUpdateForDedupe(43, 1001)); got == first {
		t.Fatalf("different caller reused dedupe key %q", got)
	}
}

func TestBotUpdateDedupeKeyIgnoresUnmarkedWebViewData(t *testing.T) {
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Id:     17,
		PeerId: mtproto.MakePeerUser(42),
		FromId: mtproto.MakePeerUser(42),
		Action: mtproto.MakeMessageActionWebViewDataSentMe("Open", "payload"),
	}).To_Message()
	update := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: message}).To_Update()
	if got := botUpdateDedupeKey(update); got != "" {
		t.Fatalf("unmarked web app data dedupe key = %q, want empty", got)
	}
}
