package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

type ringtoneErrorStore struct {
	err error
}

func (s *ringtoneErrorStore) Get(string) (string, error) {
	return "", s.err
}

func (s *ringtoneErrorStore) Set(string, string) error {
	return s.err
}

func TestRingtoneSaveGetRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	doc := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 42, AccessHash: 7}).To_InputDocument()
	if _, err := c.AccountSaveRingtone(&mtproto.TLAccountSaveRingtone{Id: doc, Unsave: mtproto.BoolFalse}); err != nil {
		t.Fatal(err)
	}
	got, err := c.AccountGetSavedRingtones(&mtproto.TLAccountGetSavedRingtones{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.GetRingtones()) == 0 || got.GetRingtones()[0].GetId() != 42 || got.GetRingtones()[0].GetAccessHash() != 7 {
		t.Fatalf("ringtone get: %+v", got)
	}
}

func TestRingtoneSaveUnsaveAndHash(t *testing.T) {
	const userID = int64(81019031)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	doc := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 43, AccessHash: 8, FileReference: []byte("ref")}).To_InputDocument()
	if _, err := c.AccountSaveRingtone(&mtproto.TLAccountSaveRingtone{Id: doc}); err != nil {
		t.Fatal(err)
	}
	first, err := c.AccountGetSavedRingtones(&mtproto.TLAccountGetSavedRingtones{})
	if err != nil || first == nil || len(first.GetRingtones()) != 1 {
		t.Fatalf("first read = (%v, %v), want one ringtone", first, err)
	}
	unchanged, err := c.AccountGetSavedRingtones(&mtproto.TLAccountGetSavedRingtones{Hash: first.GetHash()})
	if err != nil || unchanged == nil || unchanged.GetPredicateName() != mtproto.Predicate_account_savedRingtonesNotModified {
		t.Fatalf("not modified read = (%v, %v), want account.savedRingtonesNotModified", unchanged, err)
	}
	if _, err = c.AccountSaveRingtone(&mtproto.TLAccountSaveRingtone{Id: doc, Unsave: mtproto.BoolTrue}); err != nil {
		t.Fatal(err)
	}
	last, err := c.AccountGetSavedRingtones(&mtproto.TLAccountGetSavedRingtones{})
	if err != nil || last == nil || len(last.GetRingtones()) != 0 {
		t.Fatalf("after unsave = (%v, %v), want empty list", last, err)
	}
}

func TestRingtoneStorageErrorsPropagate(t *testing.T) {
	wantErr := errors.New("ringtone store unavailable")
	oldStore := persist.Default
	persist.Default = &ringtoneErrorStore{err: wantErr}
	t.Cleanup(func() { persist.Default = oldStore })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81019032}}
	doc := mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: 44, AccessHash: 9}).To_InputDocument()
	if result, err := c.AccountSaveRingtone(&mtproto.TLAccountSaveRingtone{Id: doc}); result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("save error = (%v, %v), want nil and %v", result, err, wantErr)
	}
	if result, err := c.AccountGetSavedRingtones(&mtproto.TLAccountGetSavedRingtones{}); result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("get error = (%v, %v), want nil and %v", result, err, wantErr)
	}
}
