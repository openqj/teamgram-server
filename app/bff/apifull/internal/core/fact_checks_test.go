package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

type factMessageReaderStub struct {
	box *mtproto.MessageBox
	err error
}

func (s *factMessageReaderStub) MessageGetUserMessage(context.Context, *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.box, nil
}

func factTestCore(userID int64, reader dao.PollMessageReader) *ApiFullCore {
	return &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{PollMessageReader: reader}},
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func factTestBox(msgID int32, peerType int32, peerID int64) *mtproto.MessageBox {
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		MessageId: msgID,
		PeerType:  peerType,
		PeerId:    peerID,
	}).To_MessageBox()
}

func TestFactCheckRoundtrip(t *testing.T) {
	peer := &mtproto.InputPeer{UserId: 7}
	key := factStoreKey(1, peer, 42)
	if err := persist.Default.Set(key, ""); err != nil {
		t.Fatal(err)
	}

	anon := &ApiFullCore{}
	if _, err := anon.MessagesEditFactCheck(&mtproto.TLMessagesEditFactCheck{
		Peer: peer, MsgId: 42, Text: &mtproto.TextWithEntities{Text: "nope"},
	}); err == nil {
		t.Fatal("edit without auth")
	}
	if _, err := anon.MessagesDeleteFactCheck(&mtproto.TLMessagesDeleteFactCheck{
		Peer: peer, MsgId: 42,
	}); err == nil {
		t.Fatal("delete without auth")
	}
	if _, err := anon.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{
		Peer: peer, MsgId: []int32{42},
	}); err == nil {
		t.Fatal("get without auth")
	}

	c := factTestCore(1, &factMessageReaderStub{box: factTestBox(42, mtproto.PEER_USER, 7)})
	text := &mtproto.TextWithEntities{Text: "checked"}
	if _, err := c.MessagesEditFactCheck(&mtproto.TLMessagesEditFactCheck{
		Peer: peer, MsgId: 42, Text: text,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{
		Peer: peer, MsgId: []int32{42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Datas) != 1 || got.Datas[0].GetText().GetText() != "checked" || got.Datas[0].GetNeedCheck() {
		t.Fatalf("get after edit: %#v", got)
	}

	if _, err = c.MessagesDeleteFactCheck(&mtproto.TLMessagesDeleteFactCheck{
		Peer: peer, MsgId: 42,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = c.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{
		Peer: peer, MsgId: []int32{42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Datas) != 1 || !got.Datas[0].GetNeedCheck() || got.Datas[0].GetText() != nil {
		t.Fatalf("get after delete: %#v", got)
	}
}

func TestFactCheckRejectsUnknownOrMismatchedMessage(t *testing.T) {
	peer := &mtproto.InputPeer{UserId: 7}
	unknown := factTestCore(1, &factMessageReaderStub{err: mtproto.ErrMessageIdInvalid})
	if _, err := unknown.MessagesEditFactCheck(&mtproto.TLMessagesEditFactCheck{Peer: peer, MsgId: 42}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("unknown message error = %v", err)
	}

	mismatch := factTestCore(1, &factMessageReaderStub{box: factTestBox(42, mtproto.PEER_USER, 8)})
	if _, err := mismatch.MessagesDeleteFactCheck(&mtproto.TLMessagesDeleteFactCheck{Peer: peer, MsgId: 42}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("mismatched peer error = %v", err)
	}
}

func TestFactCheckRejectsMalformedRequestsAndMissingReader(t *testing.T) {
	noReader := factTestCore(1, nil)
	if _, err := noReader.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{Peer: &mtproto.InputPeer{UserId: 7}, MsgId: []int32{42}}); !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing reader error = %v", err)
	}

	reader := &factMessageReaderStub{box: factTestBox(42, mtproto.PEER_USER, 7)}
	c := factTestCore(1, reader)
	if _, err := c.MessagesEditFactCheck(nil); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("nil edit error = %v", err)
	}
	if _, err := c.MessagesDeleteFactCheck(&mtproto.TLMessagesDeleteFactCheck{Peer: &mtproto.InputPeer{UserId: 7}}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("zero delete id error = %v", err)
	}
	if _, err := c.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{MsgId: []int32{42}}); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("missing get peer error = %v", err)
	}
	if _, err := c.MessagesGetFactCheck(&mtproto.TLMessagesGetFactCheck{Peer: &mtproto.InputPeer{UserId: 7}}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("empty get ids error = %v", err)
	}
}
