package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestHistoryImportUsesLocalPostgresBackedStateWithoutProvider(t *testing.T) {
	previous := persist.Default
	persist.Use(&localHistoryImportStore{values: map[string]string{}})
	t.Cleanup(func() { persist.Use(previous) })

	const uid int64 = 88201
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	parsed, err := core.MessagesCheckHistoryImport(&mtproto.TLMessagesCheckHistoryImport{
		ImportHead: `{"type":"private","title":"exported chat"}`,
	})
	if err != nil || parsed == nil || !parsed.GetPm() || parsed.GetGroup() || parsed.GetTitle() == nil || parsed.GetTitle().GetValue() != "exported chat" {
		t.Fatalf("local check = (%v, %v)", parsed, err)
	}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 99201}).To_InputPeer()
	started, err := core.MessagesInitHistoryImport(&mtproto.TLMessagesInitHistoryImport{
		Peer: peer, File: &mtproto.InputFile{Id_INT64: 7, Parts: 2, Name: "export.json"}, MediaCount: 3,
	})
	if err != nil || started == nil || started.GetId() <= 0 {
		t.Fatalf("local init = (%v, %v)", started, err)
	}
	confirm, err := core.MessagesCheckHistoryImportPeer(&mtproto.TLMessagesCheckHistoryImportPeer{Peer: peer})
	if err != nil || confirm == nil || confirm.GetConfirmText() == "" {
		t.Fatalf("local peer check = (%v, %v)", confirm, err)
	}
	if got, err := core.MessagesStartHistoryImport(&mtproto.TLMessagesStartHistoryImport{Peer: peer, ImportId: started.GetId()}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("local start = (%v, %v)", got, err)
	}
	if got, err := core.MessagesStartHistoryImport(&mtproto.TLMessagesStartHistoryImport{Peer: peer, ImportId: started.GetId()}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("local idempotent start = (%v, %v)", got, err)
	}
}

func TestHistoryImportLocalCheckRejectsAmbiguousHead(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 88202}}
	if got, err := core.MessagesCheckHistoryImport(&mtproto.TLMessagesCheckHistoryImport{ImportHead: "export.json"}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("ambiguous local check = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}

type localHistoryImportStore struct{ values map[string]string }

func (s *localHistoryImportStore) Get(key string) (string, error) { return s.values[key], nil }
func (s *localHistoryImportStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}
func (s *localHistoryImportStore) Update(key string, fn func(string) (string, error)) error {
	next, err := fn(s.values[key])
	if err != nil {
		return err
	}
	s.values[key] = next
	return nil
}
