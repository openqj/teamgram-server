package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesEditChatPhotoRejectsInvalidRequests(t *testing.T) {
	core := &ChatsCore{ctx: context.Background(), Logger: logx.WithContext(context.Background()), MD: &metadata.RpcMetadata{UserId: 42}}
	for name, in := range map[string]*mtproto.TLMessagesEditChatPhoto{
		"nil request": nil,
		"zero chat":   {Photo: mtproto.MakeTLInputChatPhotoEmpty(nil).To_InputChatPhoto()},
		"nil photo":   {ChatId: 7},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := core.MessagesEditChatPhoto(in)
			if got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("result = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
			}
		})
	}
}

func TestMessagesEditChatPhotoRejectsUnauthenticated(t *testing.T) {
	core := &ChatsCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
	got, err := core.MessagesEditChatPhoto(nil)
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("result = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
