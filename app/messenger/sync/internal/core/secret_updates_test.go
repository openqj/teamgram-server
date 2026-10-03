package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestProcessUpdatesPushesNewEncryptedMessage(t *testing.T) {
	update := mtproto.MakeTLUpdateNewEncryptedMessage(&mtproto.Update{
		Message_ENCRYPTEDMESSAGE: mtproto.MakeTLEncryptedMessageService(&mtproto.EncryptedMessage{
			RandomId: 1,
			ChatId:   2,
			Date:     3,
			Bytes:    []byte{4},
		}).To_EncryptedMessage(),
		Qts: 5,
	}).To_Update()

	needPush, err := (&SyncCore{}).processUpdates(syncTypeUser, 7, false, mtproto.MakeUpdatesByUpdates(update))
	if err != nil || !needPush {
		t.Fatalf("needPush=%v err=%v", needPush, err)
	}
}
