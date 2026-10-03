package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type importContactsStore struct {
	data   map[string]string
	setErr error
}

func (s *importContactsStore) Get(key string) (string, error) { return s.data[key], nil }

func (s *importContactsStore) Set(key, value string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.data[key] = value
	return nil
}

type importContactsUserClientStub struct {
	userclient.UserClient
	response *userpb.UserImportedContacts
	err      error
	request  *userpb.TLUserImportContacts
	calls    int
}

func (s *importContactsUserClientStub) UserImportContacts(_ context.Context, in *userpb.TLUserImportContacts) (*userpb.UserImportedContacts, error) {
	s.calls++
	s.request = in
	return s.response, s.err
}

func newImportContactsTestCore(userID int64, client userclient.UserClient) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func importPhoneContact(clientID int64, phone, first string) *mtproto.InputContact {
	return mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
		ClientId:  clientID,
		Phone:     phone,
		FirstName: first,
	}).To_InputContact()
}

func withImportContactsStore(t *testing.T, store *importContactsStore) {
	t.Helper()
	old := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = old })
}

func TestContactsImportContactsPreservesUserResponseOrderAndSavesPhones(t *testing.T) {
	const userID = int64(82001)
	withImportContactsStore(t, &importContactsStore{data: make(map[string]string)})
	client := &importContactsUserClientStub{response: &userpb.UserImportedContacts{
		Imported: []*mtproto.ImportedContact{
			{UserId: 20, ClientId: 2},
			{UserId: 10, ClientId: 1},
		},
		PopularInvites: []*mtproto.PopularContact{
			{ClientId: 1, Importers: 4},
			{ClientId: 2, Importers: 3},
		},
		RetryContacts: []int64{1, 2},
		Users: []*mtproto.User{
			{Id: 10, FirstName: mtproto.MakeFlagsString("Ten")},
			{Id: 20, FirstName: mtproto.MakeFlagsString("Twenty")},
		},
	}}
	core := newImportContactsTestCore(userID, client)
	contacts := []*mtproto.InputContact{
		importPhoneContact(2, "+12025550102", "Twenty"),
		importPhoneContact(1, "+12025550103", "Ten"),
	}

	got, err := core.ContactsImportContacts(&mtproto.TLContactsImportContacts{Contacts: contacts})
	if err != nil || got == nil {
		t.Fatalf("ContactsImportContacts() = (%+v, %v), want imported contacts", got, err)
	}
	if client.calls != 1 || client.request == nil || client.request.GetUserId() != userID || len(client.request.GetContacts()) != 2 {
		t.Fatalf("user import request = (%d, %+v), want one request for user %d", client.calls, client.request, userID)
	}
	if client.request.GetContacts()[0].GetPhone() != "12025550102" || client.request.GetContacts()[1].GetPhone() != "12025550103" {
		t.Fatalf("user import phones = [%s %s], want normalized digit strings", client.request.GetContacts()[0].GetPhone(), client.request.GetContacts()[1].GetPhone())
	}
	if got.GetImported()[0].GetClientId() != 2 || got.GetImported()[1].GetClientId() != 1 {
		t.Fatalf("imported order = [%d %d], want request order [2 1]", got.GetImported()[0].GetClientId(), got.GetImported()[1].GetClientId())
	}
	if got.GetPopularInvites()[0].GetClientId() != 2 || got.GetPopularInvites()[1].GetClientId() != 1 {
		t.Fatalf("popular invite order = [%d %d], want request order [2 1]", got.GetPopularInvites()[0].GetClientId(), got.GetPopularInvites()[1].GetClientId())
	}
	if len(got.GetRetryContacts()) != 2 || got.GetRetryContacts()[0] != 2 || got.GetRetryContacts()[1] != 1 {
		t.Fatalf("retry contacts = %v, want request order [2 1]", got.GetRetryContacts())
	}
	if got.GetUsers()[0].GetId() != 20 || got.GetUsers()[1].GetId() != 10 {
		t.Fatalf("user order = [%d %d], want request order [20 10]", got.GetUsers()[0].GetId(), got.GetUsers()[1].GetId())
	}
	saved, err := core.ContactsGetSaved(&mtproto.TLContactsGetSaved{})
	if err != nil || len(saved.GetDatas()) != 2 || saved.GetDatas()[0].GetPhone() != "12025550102" || saved.GetDatas()[1].GetPhone() != "12025550103" {
		t.Fatalf("saved contacts = (%+v, %v), want canonical phones in request order", saved, err)
	}
}

func TestContactsImportContactsRejectsCanonicalDuplicateAndInvalidPhone(t *testing.T) {
	tests := []struct {
		name    string
		phones  []string
		wantErr error
	}{
		{name: "same phone in different formats", phones: []string{"+12025550102", "+1 202 555 0102"}, wantErr: mtproto.ErrInputRequestInvalid},
		{name: "invalid phone", phones: []string{"+999"}, wantErr: mtproto.ErrPhoneNumberInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contacts := make([]*mtproto.InputContact, 0, len(tt.phones))
			for i, phone := range tt.phones {
				contacts = append(contacts, importPhoneContact(int64(i+1), phone, "Contact"))
			}
			client := &importContactsUserClientStub{}
			got, err := newImportContactsTestCore(82020, client).ContactsImportContacts(&mtproto.TLContactsImportContacts{Contacts: contacts})
			if got != nil || !errors.Is(err, tt.wantErr) {
				t.Fatalf("ContactsImportContacts() = (%+v, %v), want error %v", got, err, tt.wantErr)
			}
			if client.calls != 0 {
				t.Fatalf("invalid input caused %d backend calls, want none", client.calls)
			}
		})
	}
}

func TestNormalizeSavedContactsCanonicalizesLegacyAndSpecialPhones(t *testing.T) {
	saved := normalizeSavedContacts([]savedPhoneContact{
		{Phone: "+12025550102", FirstName: "Old"},
		{Phone: "12025550102", FirstName: "Updated"},
		{Phone: "+42400", FirstName: "Short code"},
		{Phone: "+888 0888 0080", FirstName: "Fragment"},
	})
	if len(saved) != 3 {
		t.Fatalf("normalized saved contacts count = %d, want 3: %+v", len(saved), saved)
	}
	if saved[0].Phone != "12025550102" || saved[0].FirstName != "Updated" {
		t.Fatalf("normalized first contact = %+v, want canonical phone and latest name", saved[0])
	}
	if saved[1].Phone != "42400" || saved[2].Phone != "88808880080" {
		t.Fatalf("normalized special phones = [%s %s], want [42400 88808880080]", saved[1].Phone, saved[2].Phone)
	}
}

func TestContactsImportContactsEmptyVectorIsNoOp(t *testing.T) {
	const userID = int64(82002)
	store := &importContactsStore{data: make(map[string]string)}
	withImportContactsStore(t, store)
	client := &importContactsUserClientStub{}
	got, err := newImportContactsTestCore(userID, client).ContactsImportContacts(&mtproto.TLContactsImportContacts{})
	if err != nil || got == nil || len(got.GetImported()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("empty import = (%+v, %v), want empty success", got, err)
	}
	if client.calls != 0 || store.data[savedContactsKey(userID)] != "" {
		t.Fatalf("empty import side effects: calls=%d saved=%q, want none", client.calls, store.data[savedContactsKey(userID)])
	}
}

func TestContactsImportContactsRejectsInvalidInputBeforeBackend(t *testing.T) {
	validA := importPhoneContact(1, "+12025550101", "A")
	tests := []struct {
		name    string
		request *mtproto.TLContactsImportContacts
		wantErr error
	}{
		{name: "nil request", wantErr: mtproto.ErrInputRequestInvalid},
		{name: "nil contact", request: &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{nil}}, wantErr: mtproto.ErrInputRequestInvalid},
		{name: "blank phone", request: &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{importPhoneContact(1, "  ", "A")}}, wantErr: mtproto.ErrInputRequestInvalid},
		{name: "duplicate phone", request: &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{validA, importPhoneContact(2, "+12025550101", "B")}}, wantErr: mtproto.ErrInputRequestInvalid},
		{name: "duplicate client id", request: &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{validA, importPhoneContact(1, "+12025550102", "B")}}, wantErr: mtproto.ErrInputRequestInvalid},
		{name: "wrong constructor", request: &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{{ClientId: 1, Phone: "+12025550101"}}}, wantErr: mtproto.ErrInputRequestInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &importContactsUserClientStub{}
			got, err := newImportContactsTestCore(82003, client).ContactsImportContacts(tt.request)
			if got != nil || !errors.Is(err, tt.wantErr) {
				t.Fatalf("ContactsImportContacts() = (%+v, %v), want error %v", got, err, tt.wantErr)
			}
			if client.calls != 0 {
				t.Fatalf("invalid input caused %d backend calls, want none", client.calls)
			}
		})
	}
}

func TestContactsImportContactsPropagatesBackendAndPersistenceErrors(t *testing.T) {
	const userID = int64(82004)
	contact := importPhoneContact(1, "+12025550104", "A")
	request := &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{contact}}

	backendErr := errors.New("user service unavailable")
	withImportContactsStore(t, &importContactsStore{data: make(map[string]string)})
	client := &importContactsUserClientStub{err: backendErr}
	if got, err := newImportContactsTestCore(userID, client).ContactsImportContacts(request); got != nil || !errors.Is(err, backendErr) {
		t.Fatalf("backend error result = (%+v, %v), want %v", got, err, backendErr)
	}

	storeErr := errors.New("saved contact store unavailable")
	store := &importContactsStore{data: make(map[string]string), setErr: storeErr}
	withImportContactsStore(t, store)
	client = &importContactsUserClientStub{response: &userpb.UserImportedContacts{Imported: []*mtproto.ImportedContact{}, Users: []*mtproto.User{}}}
	got, err := newImportContactsTestCore(userID, client).ContactsImportContacts(request)
	if got != nil || !errors.Is(err, storeErr) {
		t.Fatalf("persistence failure result = (%+v, %v), want %v", got, err, storeErr)
	}
	if client.calls != 1 || store.data[savedContactsKey(userID)] != "" {
		t.Fatalf("persistence failure state: calls=%d saved=%q, want backend called and no saved value", client.calls, store.data[savedContactsKey(userID)])
	}
}

func TestContactsImportContactsRejectsIncompleteBackendResponse(t *testing.T) {
	const userID = int64(82005)
	store := &importContactsStore{data: make(map[string]string)}
	withImportContactsStore(t, store)
	client := &importContactsUserClientStub{response: &userpb.UserImportedContacts{
		Imported: []*mtproto.ImportedContact{{UserId: 10, ClientId: 1}},
		Users:    []*mtproto.User{},
	}}
	request := &mtproto.TLContactsImportContacts{Contacts: []*mtproto.InputContact{importPhoneContact(1, "+12025550105", "A")}}
	if got, err := newImportContactsTestCore(userID, client).ContactsImportContacts(request); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("incomplete backend result = (%+v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
	if store.data[savedContactsKey(userID)] != "" {
		t.Fatalf("incomplete backend response wrote saved contacts %q", store.data[savedContactsKey(userID)])
	}
}
