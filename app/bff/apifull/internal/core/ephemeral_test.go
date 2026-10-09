package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestEphemeralSendAndDeleteRoundTrip(t *testing.T) {
	const uid int64 = 81014001
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	if _, key, err := ephemeralStoreKey(uid, peer, 0); err != nil {
		t.Fatal(err)
	} else if err = persist.Default.Set(key, ""); err != nil {
		t.Fatal(err)
	}

	first, err := c.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{
		Peer: peer, Message: "hello", RandomId: 81014001,
	})
	if err != nil || first == nil || len(first.GetUpdates()) != 1 || first.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetMessage() != "hello" {
		t.Fatalf("send: reply=%+v err=%v", first, err)
	}
	second, err := c.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{
		Peer: peer, Message: "different", RandomId: 81014001,
	})
	if err != nil || second == nil || second.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetId() != first.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetId() {
		t.Fatalf("idempotent send: reply=%+v err=%v", second, err)
	}

	messageID := first.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetId()
	if reply, err := c.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{Peer: peer, Id: messageID}); err != nil || reply != mtproto.BoolTrue {
		t.Fatalf("delete: reply=%+v err=%v", reply, err)
	}
	if reply, err := c.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{Peer: peer, Id: messageID}); reply != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("repeat delete: reply=%+v err=%v, want MESSAGE_ID_INVALID", reply, err)
	}
	if _, key, err := ephemeralStoreKey(uid, peer, 0); err != nil {
		t.Fatal(err)
	} else if got, err := persist.Default.Get(key); err != nil || got != "[]" {
		t.Fatalf("stored state after delete = %q, %v", got, err)
	}

	if reply, err := c.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{Peer: peer}); reply != nil || !errors.Is(err, mtproto.ErrMessageEmpty) {
		t.Fatalf("empty send: reply=%+v err=%v, want MESSAGE_EMPTY", reply, err)
	}
	if reply, err := c.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{Id: 1}); reply != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("missing peer delete: reply=%+v err=%v, want PEER_ID_INVALID", reply, err)
	}

	// Both sides of a user conversation derive the same canonical PostgreSQL
	// key even when the receiver names the sender as peer and itself as
	// receiver_id.
	const receiverID int64 = 81014003
	target := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: receiverID}).To_InputPeer()
	shared, err := c.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{
		Peer: target, ReceiverId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: receiverID}).To_InputUser(), Message: "shared",
	})
	if err != nil || shared == nil {
		t.Fatalf("shared send: reply=%+v err=%v", shared, err)
	}
	receiver := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: receiverID}}
	fromSender := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid}).To_InputPeer()
	if reply, err := receiver.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{
		Peer: fromSender, ReceiverId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: receiverID}).To_InputUser(), Id: shared.GetUpdates()[0].GetMessage_EPHEMERALMESSAGE().GetId(),
	}); err != nil || reply != mtproto.BoolTrue {
		t.Fatalf("shared receiver delete: reply=%+v err=%v", reply, err)
	}

	unauthorized := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	if reply, err := unauthorized.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{}); reply != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthorized send: reply=%+v err=%v, want AUTH_KEY_UNREGISTERED", reply, err)
	}
	if reply, err := unauthorized.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{}); reply != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthorized delete: reply=%+v err=%v, want AUTH_KEY_UNREGISTERED", reply, err)
	}
	if _, key, err := ephemeralStoreKey(uid, peer, 0); err != nil {
		t.Fatal(err)
	} else if err = persist.Default.Set(key, ""); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"misc:81014001:EphemeralSendMessage", "misc:81014001:EphemeralDeleteMessage", "b14:81014001:"} {
		if got, err := persist.Default.Get(key); err != nil || got != "" {
			t.Fatalf("legacy key %q was written: %q, %v", key, got, err)
		}
	}
}

type ephemeralStoreProbe struct {
	values map[string]string
	reads  int
	writes int
}

func (s *ephemeralStoreProbe) Get(key string) (string, error) {
	s.reads++
	return s.values[key], nil
}

func (s *ephemeralStoreProbe) Set(key, value string) error {
	s.writes++
	s.values[key] = value
	return nil
}

func TestEphemeralReportAndCallback(t *testing.T) {
	const uid int64 = 81014002
	const noteKey = "b14:81014002:"
	store := &ephemeralStoreProbe{values: map[string]string{noteKey: "existing note"}}
	oldStore := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(oldStore) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if reply, err := c.EphemeralReportMessage(&mtproto.TLEphemeralReportMessage{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Id: 42, Option: []byte("report option"), Message: "report text",
	}); err != nil || reply == nil || reply.GetPredicateName() != mtproto.Predicate_reportResultReported {
		t.Fatalf("report: reply=%+v err=%v", reply, err)
	}
	if reply, err := c.EphemeralGetCallbackAnswer(&mtproto.TLEphemeralGetCallbackAnswer{
		Id: 42, Data: []byte("callback data"),
	}); reply != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("callback: reply=%+v err=%v, want nil reply and METHOD_NOT_IMPL", reply, err)
	}
	if store.reads != 0 || store.writes != 0 || store.values[noteKey] != "existing note" {
		t.Fatalf("store accessed: reads=%d writes=%d note=%q", store.reads, store.writes, store.values[noteKey])
	}

	unauthorized := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	if reply, err := unauthorized.EphemeralReportMessage(&mtproto.TLEphemeralReportMessage{}); reply != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthorized report: reply=%+v err=%v, want AUTH_KEY_UNREGISTERED", reply, err)
	}
	if reply, err := unauthorized.EphemeralGetCallbackAnswer(&mtproto.TLEphemeralGetCallbackAnswer{}); reply != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthorized callback: reply=%+v err=%v, want AUTH_KEY_UNREGISTERED", reply, err)
	}
	if store.reads != 0 || store.writes != 0 {
		t.Fatalf("unauthorized calls accessed store: reads=%d writes=%d", store.reads, store.writes)
	}
}
