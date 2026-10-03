package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func newSendMultiMediaValidationCore() *MessagesCore {
	return &MessagesCore{
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func sendMultiMediaPeer() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
}

func sendMultiMediaEmptyItem() *mtproto.InputSingleMedia {
	return mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{
		Media: mtproto.MakeTLInputMediaEmpty(nil).To_InputMedia(),
	}).To_InputSingleMedia()
}

func TestMessagesSendMultiMediaRejectsInvalidInput(t *testing.T) {
	valid := func() *mtproto.TLMessagesSendMultiMedia {
		return &mtproto.TLMessagesSendMultiMedia{
			Peer:       sendMultiMediaPeer(),
			MultiMedia: []*mtproto.InputSingleMedia{sendMultiMediaEmptyItem()},
		}
	}

	tests := []struct {
		name string
		core *MessagesCore
		in   *mtproto.TLMessagesSendMultiMedia
		want error
	}{
		{name: "nil core", want: mtproto.ErrAuthKeyUnregistered},
		{name: "missing metadata", core: &MessagesCore{}, in: valid(), want: mtproto.ErrAuthKeyUnregistered},
		{name: "nil request", core: newSendMultiMediaValidationCore(), want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", core: newSendMultiMediaValidationCore(), in: &mtproto.TLMessagesSendMultiMedia{MultiMedia: valid().MultiMedia}, want: mtproto.ErrPeerIdInvalid},
		{name: "empty media vector", core: newSendMultiMediaValidationCore(), in: &mtproto.TLMessagesSendMultiMedia{Peer: sendMultiMediaPeer()}, want: mtproto.ErrInputRequestInvalid},
		{name: "empty peer", core: newSendMultiMediaValidationCore(), in: func() *mtproto.TLMessagesSendMultiMedia {
			in := valid()
			in.Peer = mtproto.MakeTLInputPeerEmpty(nil).To_InputPeer()
			return in
		}(), want: mtproto.ErrPeerIdInvalid},
		{name: "zero user peer", core: newSendMultiMediaValidationCore(), in: func() *mtproto.TLMessagesSendMultiMedia {
			in := valid()
			in.Peer = mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 0, AccessHash: 1}).To_InputPeer()
			return in
		}(), want: mtproto.ErrPeerIdInvalid},
		{name: "nil media item", core: newSendMultiMediaValidationCore(), in: func() *mtproto.TLMessagesSendMultiMedia {
			in := valid()
			in.MultiMedia = []*mtproto.InputSingleMedia{nil}
			return in
		}(), want: mtproto.ErrInputRequestInvalid},
		{name: "caption too long", core: newSendMultiMediaValidationCore(), in: func() *mtproto.TLMessagesSendMultiMedia {
			in := valid()
			in.MultiMedia[0].Message = strings.Repeat("x", 4001)
			return in
		}(), want: mtproto.ErrMediaCaptionTooLong},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.core.MessagesSendMultiMedia(test.in)
			if got != nil || !errors.Is(err, test.want) {
				t.Fatalf("MessagesSendMultiMedia() = (%v, %v), want (nil, %v)", got, err, test.want)
			}
		})
	}
}

func TestMessagesSendMultiMediaRejectsMissingMediaBeforeProvider(t *testing.T) {
	core := newSendMultiMediaValidationCore()
	in := &mtproto.TLMessagesSendMultiMedia{
		Peer:       sendMultiMediaPeer(),
		MultiMedia: []*mtproto.InputSingleMedia{mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{}).To_InputSingleMedia()},
	}

	got, err := core.MessagesSendMultiMedia(in)
	if got != nil || !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("MessagesSendMultiMedia() = (%v, %v), want (nil, MEDIA_INVALID)", got, err)
	}
}

func TestMessagesSendMultiMediaFailsClosedWithoutProvider(t *testing.T) {
	core := newSendMultiMediaValidationCore()
	in := &mtproto.TLMessagesSendMultiMedia{
		Peer:       sendMultiMediaPeer(),
		MultiMedia: []*mtproto.InputSingleMedia{sendMultiMediaEmptyItem()},
	}

	got, err := core.MessagesSendMultiMedia(in)
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesSendMultiMedia() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

func TestMessagesSendMultiMediaFailsClosedWithoutIDGenerator(t *testing.T) {
	core := newSendMultiMediaValidationCore()
	core.svcCtx = &svc.ServiceContext{Dao: &dao.Dao{
		MsgClient: &forwardMsgClientStub{response: mtproto.MakeEmptyUpdates()},
	}}
	in := &mtproto.TLMessagesSendMultiMedia{
		Peer:       sendMultiMediaPeer(),
		MultiMedia: []*mtproto.InputSingleMedia{sendMultiMediaEmptyItem()},
	}

	got, err := core.MessagesSendMultiMedia(in)
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesSendMultiMedia(missing id generator) = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

func TestMessagesSendMultiMediaRejectsStoryReply(t *testing.T) {
	core := newSendMultiMediaValidationCore()
	in := &mtproto.TLMessagesSendMultiMedia{
		Peer:       sendMultiMediaPeer(),
		MultiMedia: []*mtproto.InputSingleMedia{sendMultiMediaEmptyItem()},
		ReplyTo: mtproto.MakeTLInputReplyToStory(&mtproto.InputReplyTo{
			StoryId: 1,
		}).To_InputReplyTo(),
	}

	got, err := core.MessagesSendMultiMedia(in)
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesSendMultiMedia() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}
