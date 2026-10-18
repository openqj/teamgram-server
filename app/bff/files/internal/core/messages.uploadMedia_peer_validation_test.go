package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesUploadMediaRejectsMalformedPeers(t *testing.T) {
	core := &FilesCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 42, UserId: 42},
		Logger: logx.WithContext(context.Background()),
	}
	media := &mtproto.InputMedia{PredicateName: mtproto.Predicate_inputMediaEmpty}
	tests := []struct {
		name string
		peer *mtproto.InputPeer
	}{
		{name: "missing", peer: nil},
		{name: "empty", peer: mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer()},
		{name: "user missing access hash", peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84}).To_InputPeer()},
		{name: "channel missing access hash", peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 84}).To_InputPeer()},
		{name: "chat zero id", peer: mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{}).To_InputPeer()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.MessagesUploadMedia(&mtproto.TLMessagesUploadMedia{Peer: tt.peer, Media: media})
			if got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("MessagesUploadMedia() = (%v, %v), want nil and PEER_ID_INVALID", got, err)
			}
		})
	}
}

func TestMessagesUploadMediaRejectsNilReceiver(t *testing.T) {
	var core *FilesCore
	got, err := core.MessagesUploadMedia(nil)
	if got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MessagesUploadMedia(nil receiver) = (%v, %v), want nil and INPUT_REQUEST_INVALID", got, err)
	}
}

func TestMessagesUploadMediaRejectsMissingAuthMetadata(t *testing.T) {
	core := &FilesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
	got, err := core.MessagesUploadMedia(&mtproto.TLMessagesUploadMedia{
		Peer:  mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media: &mtproto.InputMedia{PredicateName: mtproto.Predicate_inputMediaEmpty},
	})
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("MessagesUploadMedia(missing auth) = (%v, %v), want nil and AUTH_KEY_UNREGISTERED", got, err)
	}
}

func TestMessagesUploadMediaAcceptsSelfPeerForLocalMedia(t *testing.T) {
	core := &FilesCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 42, UserId: 42},
		Logger: logx.WithContext(context.Background()),
	}
	got, err := core.MessagesUploadMedia(&mtproto.TLMessagesUploadMedia{
		Peer:  mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media: &mtproto.InputMedia{PredicateName: mtproto.Predicate_inputMediaEmpty},
	})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
		t.Fatalf("MessagesUploadMedia(self) = (%v, %v), want messageMediaEmpty", got, err)
	}
}
