package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestEphemeralSendAndDeleteUnavailable(t *testing.T) {
	const uid int64 = 81014001
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	keys := []string{
		"misc:81014001:EphemeralSendMessage",
		"misc:81014001:EphemeralDeleteMessage",
		"b14:81014001:",
	}
	for _, key := range keys {
		if err := persist.Default.Set(key, ""); err != nil {
			t.Fatal(err)
		}
	}

	if reply, err := c.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{Message: "unsupported"}); reply != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("send: reply=%+v err=%v, want nil reply and METHOD_NOT_IMPL", reply, err)
	}
	if reply, err := c.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{Id: 1}); reply != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("delete: reply=%+v err=%v, want nil reply and METHOD_NOT_IMPL", reply, err)
	}
	for _, key := range keys {
		if got, err := persist.Default.Get(key); err != nil || got != "" {
			t.Fatalf("key %q was written: %q, %v", key, got, err)
		}
	}

	unauthorized := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	if reply, err := unauthorized.EphemeralSendMessage(&mtproto.TLEphemeralSendMessage{}); reply != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthorized send: reply=%+v err=%v, want AUTH_KEY_UNREGISTERED", reply, err)
	}
	if reply, err := unauthorized.EphemeralDeleteMessage(&mtproto.TLEphemeralDeleteMessage{}); reply != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthorized delete: reply=%+v err=%v, want AUTH_KEY_UNREGISTERED", reply, err)
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

func TestEphemeralReportAndCallbackUnavailable(t *testing.T) {
	const uid int64 = 81014002
	const noteKey = "b14:81014002:"
	store := &ephemeralStoreProbe{values: map[string]string{noteKey: "existing note"}}
	oldStore := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(oldStore) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if reply, err := c.EphemeralReportMessage(&mtproto.TLEphemeralReportMessage{
		Id: 42, Option: []byte("report option"), Message: "report text",
	}); reply != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("report: reply=%+v err=%v, want nil reply and METHOD_NOT_IMPL", reply, err)
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
