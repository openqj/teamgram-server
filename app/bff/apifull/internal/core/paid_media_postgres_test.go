package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

type extendedMediaMessageReader struct {
	boxes []*mtproto.MessageBox
	last  *messagepb.TLMessageGetUserMessageList
}

func (r *extendedMediaMessageReader) MessageGetUserMessageList(_ context.Context, in *messagepb.TLMessageGetUserMessageList) (*messagepb.Vector_MessageBox, error) {
	r.last = in
	return &messagepb.Vector_MessageBox{Datas: r.boxes}, nil
}

func extendedMediaBox(userID int64, id int32, paid bool) *mtproto.MessageBox {
	message := &mtproto.Message{Id: id, PeerId: mtproto.MakePeerUser(userID)}
	if paid {
		media := mtproto.MakeTLMessageMediaPaidMedia(&mtproto.MessageMedia{
			ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: []*mtproto.MessageExtendedMedia{{
				PredicateName: mtproto.Predicate_messageExtendedMediaPreview,
			}},
		}).To_MessageMedia()
		message.Media = media
	}
	return &mtproto.MessageBox{
		MessageId: id,
		PeerType:  mtproto.PEER_USER,
		PeerId:    userID,
		Message:   mtproto.MakeTLMessage(message).To_Message(),
	}
}

func TestMessagesGetExtendedMediaUsesAuthoritativeMessageProvider(t *testing.T) {
	const userID int64 = 981701
	reader := &extendedMediaMessageReader{boxes: []*mtproto.MessageBox{
		extendedMediaBox(userID, 7, true),
		extendedMediaBox(userID, 8, false),
		extendedMediaBox(userID+1, 9, true),
	}}
	core := &ApiFullCore{
		MD:     &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageListReader: reader}},
	}
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	updates, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Peer: peer, Id: []int32{7, 8, 9}})
	if err != nil || updates == nil {
		t.Fatalf("get extended media: updates=%+v err=%v", updates, err)
	}
	if reader.last == nil || reader.last.GetUserId() != userID || len(reader.last.GetIdList()) != 3 {
		t.Fatalf("provider request=%+v", reader.last)
	}
	if len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateMessageExtendedMedia {
		t.Fatalf("updates=%+v, want one updateMessageExtendedMedia", updates.GetUpdates())
	}
	if len(updates.GetUsers()) != 0 || len(updates.GetChats()) != 0 {
		t.Fatalf("unexpected entities: users=%d chats=%d", len(updates.GetUsers()), len(updates.GetChats()))
	}
	if err := updates.Encode(mtproto.NewEncodeBuf(256), 229); err != nil {
		t.Fatalf("encode updates: %v", err)
	}
}

func TestMessagesGetExtendedMediaValidatesPeerAndProvider(t *testing.T) {
	const userID int64 = 981702
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	if got, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Peer: mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(), Id: []int32{1}}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing provider: updates=%+v err=%v", got, err)
	}
	channel := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 7, AccessHash: 8}).To_InputPeer()
	if got, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Peer: channel, Id: []int32{1}}); got != nil || !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("channel peer: updates=%+v err=%v", got, err)
	}
	userWithoutHash := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID + 1}).To_InputPeer()
	if got, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Peer: userWithoutHash, Id: []int32{1}}); got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("user peer without access hash: updates=%+v err=%v", got, err)
	}
	if got, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Peer: mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()}); err != nil || got == nil || len(got.GetUpdates()) != 0 {
		t.Fatalf("empty id vector: updates=%+v err=%v", got, err)
	}
	if got, err := core.MessagesGetExtendedMedia(&mtproto.TLMessagesGetExtendedMedia{Peer: mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer(), Id: []int32{0}}); got != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("invalid message id: updates=%+v err=%v", got, err)
	}
}
