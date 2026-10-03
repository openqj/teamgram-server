package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type deleteByPhonesUserClientStub struct {
	userclient.UserClient
	contacts             *userpb.Vector_ContactData
	importers            *userpb.Vector_InputContact
	deleteResponse       *mtproto.Bool
	deleteImportResponse *mtproto.Bool
	contactListCalls     int
	importerRequests     []*userpb.TLUserGetImportersByPhone
	deleteRequests       []*userpb.TLUserDeleteContact
	deleteImportRequests []*userpb.TLUserDeleteImportersByPhone
}

func (s *deleteByPhonesUserClientStub) UserGetContactList(context.Context, *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	s.contactListCalls++
	return s.contacts, nil
}

func (s *deleteByPhonesUserClientStub) UserGetImportersByPhone(_ context.Context, in *userpb.TLUserGetImportersByPhone) (*userpb.Vector_InputContact, error) {
	s.importerRequests = append(s.importerRequests, in)
	return s.importers, nil
}

func (s *deleteByPhonesUserClientStub) UserDeleteContact(_ context.Context, in *userpb.TLUserDeleteContact) (*mtproto.Bool, error) {
	s.deleteRequests = append(s.deleteRequests, in)
	return s.deleteResponse, nil
}

func (s *deleteByPhonesUserClientStub) UserDeleteImportersByPhone(_ context.Context, in *userpb.TLUserDeleteImportersByPhone) (*mtproto.Bool, error) {
	s.deleteImportRequests = append(s.deleteImportRequests, in)
	return s.deleteImportResponse, nil
}

func TestContactsDeleteByPhonesNormalizesForBackendAndSavedRows(t *testing.T) {
	const userID = int64(82030)
	store := &importContactsStore{data: map[string]string{
		savedContactsKey(userID): `[{"phone":"+12025550102","first_name":"Delete"},{"phone":"+12025550103","first_name":"Keep"}]`,
	}}
	withImportContactsStore(t, store)
	client := &deleteByPhonesUserClientStub{
		contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{
			ContactUserId: 44,
			Phone:         wrapperspb.String("12025550102"),
		}}},
		importers:            &userpb.Vector_InputContact{Datas: []*mtproto.InputContact{importPhoneContact(9, "12025550102", "Other")}},
		deleteResponse:       mtproto.BoolTrue,
		deleteImportResponse: mtproto.BoolTrue,
	}
	core := newImportContactsTestCore(userID, client)

	got, err := core.ContactsDeleteByPhones(&mtproto.TLContactsDeleteByPhones{Phones: []string{"+1 202 555 0102"}})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("ContactsDeleteByPhones() = (%v, %v), want BoolTrue", got, err)
	}
	if len(client.importerRequests) != 0 {
		t.Fatalf("deleteByPhones performed global importer lookups: %+v", client.importerRequests)
	}
	if len(client.deleteImportRequests) != 1 || client.deleteImportRequests[0].GetPhone() != "12025550102" {
		t.Fatalf("unregistered import delete requests = %+v, want canonical phone 12025550102", client.deleteImportRequests)
	}
	if len(client.deleteRequests) != 1 || client.deleteRequests[0].GetId() != 44 {
		t.Fatalf("delete requests = %+v, want contact id 44", client.deleteRequests)
	}
	if got := store.data[savedContactsKey(userID)]; got != `[{"phone":"12025550103","first_name":"Keep","last_name":"","date":0}]` {
		t.Fatalf("saved contacts = %q, want only canonicalized remaining row", got)
	}
}

func TestContactsDeleteByPhonesRemovesCallerUnregisteredImport(t *testing.T) {
	const userID = int64(82032)
	store := &importContactsStore{data: map[string]string{
		savedContactsKey(userID): `[{"phone":"+12025550104","first_name":"Delete"}]`,
	}}
	withImportContactsStore(t, store)
	client := &deleteByPhonesUserClientStub{
		contacts:             &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{}},
		deleteImportResponse: mtproto.BoolTrue,
	}
	got, err := newImportContactsTestCore(userID, client).ContactsDeleteByPhones(&mtproto.TLContactsDeleteByPhones{Phones: []string{"+1 202 555 0104"}})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("ContactsDeleteByPhones(unregistered) = (%v, %v), want BoolTrue", got, err)
	}
	if len(client.importerRequests) != 0 {
		t.Fatalf("deleteByPhones performed global importer lookups: %+v", client.importerRequests)
	}
	if len(client.deleteImportRequests) != 1 || client.deleteImportRequests[0].GetPhone() != "12025550104" {
		t.Fatalf("unregistered import delete requests = %+v, want canonical phone 12025550104", client.deleteImportRequests)
	}
	if got := store.data[savedContactsKey(userID)]; got != "[]" {
		t.Fatalf("saved contacts = %q, want empty list", got)
	}
}

func TestContactsDeleteByPhonesRejectsInvalidPhoneBeforeBackend(t *testing.T) {
	const userID = int64(82031)
	store := &importContactsStore{data: make(map[string]string)}
	withImportContactsStore(t, store)
	client := &deleteByPhonesUserClientStub{}
	got, err := newImportContactsTestCore(userID, client).ContactsDeleteByPhones(&mtproto.TLContactsDeleteByPhones{Phones: []string{"+999"}})
	if got != nil || !errors.Is(err, mtproto.ErrPhoneNumberInvalid) {
		t.Fatalf("ContactsDeleteByPhones(invalid) = (%v, %v), want PHONE_NUMBER_INVALID", got, err)
	}
	if client.contactListCalls != 0 || len(client.importerRequests) != 0 || len(client.deleteRequests) != 0 || len(client.deleteImportRequests) != 0 {
		t.Fatalf("invalid phone caused backend side effects: contacts=%d importer lookups=%d deletes=%d unregistered deletes=%d", client.contactListCalls, len(client.importerRequests), len(client.deleteRequests), len(client.deleteImportRequests))
	}
}
