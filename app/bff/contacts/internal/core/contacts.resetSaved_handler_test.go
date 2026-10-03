package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type resetSavedUserClientStub struct {
	userclient.UserClient
	request  *userpb.TLUserDeleteContact
	response *mtproto.Bool
	err      error
}

func (s *resetSavedUserClientStub) UserDeleteContact(_ context.Context, in *userpb.TLUserDeleteContact) (*mtproto.Bool, error) {
	s.request = in
	return s.response, s.err
}

func TestContactsResetSavedClearsBackendAndSavedPhoneList(t *testing.T) {
	const userID = int64(82011)
	store := &importContactsStore{data: map[string]string{
		savedContactsKey(userID): `[{"phone":"+15550101","first_name":"Ada"}]`,
	}}
	withImportContactsStore(t, store)
	client := &resetSavedUserClientStub{response: mtproto.BoolTrue}
	core := newImportContactsTestCore(userID, client)

	got, err := core.ContactsResetSaved(&mtproto.TLContactsResetSaved{})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("ContactsResetSaved() = (%v, %v), want BoolTrue", got, err)
	}
	if client.request == nil || client.request.GetUserId() != userID || client.request.GetId() != 0 {
		t.Fatalf("backend reset request = %+v, want user=%d and reset id=0", client.request, userID)
	}
	if store.data[savedContactsKey(userID)] != "[]" {
		t.Fatalf("saved phone contacts = %q, want empty list", store.data[savedContactsKey(userID)])
	}
}

func TestContactsResetSavedKeepsLocalListWhenBackendFails(t *testing.T) {
	const userID = int64(82012)
	const saved = `[{"phone":"+15550102","first_name":"Grace"}]`
	store := &importContactsStore{data: map[string]string{savedContactsKey(userID): saved}}
	withImportContactsStore(t, store)
	wantErr := errors.New("user backend unavailable")
	client := &resetSavedUserClientStub{err: wantErr}
	core := newImportContactsTestCore(userID, client)

	got, err := core.ContactsResetSaved(&mtproto.TLContactsResetSaved{})
	if !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("ContactsResetSaved() = (%v, %v), want backend error", got, err)
	}
	if client.request == nil || client.request.GetUserId() != userID || client.request.GetId() != 0 {
		t.Fatalf("backend reset request = %+v, want user=%d and reset id=0", client.request, userID)
	}
	if store.data[savedContactsKey(userID)] != saved {
		t.Fatalf("saved phone contacts changed on backend failure: %q", store.data[savedContactsKey(userID)])
	}
}

func TestContactsResetSavedKeepsLocalListWhenBackendRejects(t *testing.T) {
	const userID = int64(82013)
	const saved = `[{"phone":"+15550103","first_name":"Lin"}]`
	store := &importContactsStore{data: map[string]string{savedContactsKey(userID): saved}}
	withImportContactsStore(t, store)
	client := &resetSavedUserClientStub{response: mtproto.BoolFalse}
	core := newImportContactsTestCore(userID, client)

	got, err := core.ContactsResetSaved(&mtproto.TLContactsResetSaved{})
	if err != nil || got != mtproto.BoolFalse {
		t.Fatalf("ContactsResetSaved() = (%v, %v), want BoolFalse", got, err)
	}
	if store.data[savedContactsKey(userID)] != saved {
		t.Fatalf("saved phone contacts changed after backend rejection: %q", store.data[savedContactsKey(userID)])
	}
}

func TestContactsResetSavedRejectsMissingRequestBeforeBackend(t *testing.T) {
	client := &resetSavedUserClientStub{response: mtproto.BoolTrue}
	core := newImportContactsTestCore(82014, client)

	got, err := core.ContactsResetSaved(nil)
	if !errors.Is(err, mtproto.ErrInputRequestInvalid) || got != nil {
		t.Fatalf("ContactsResetSaved(nil) = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
	if client.request != nil {
		t.Fatalf("invalid request reached backend: %+v", client.request)
	}
}
