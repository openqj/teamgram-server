package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type getStatusesUserClientStub struct {
	userclient.UserClient
	contactList      *userpb.Vector_ContactData
	contactErr       error
	lastSeenList     *userpb.Vector_LastSeenData
	lastSeenErr      error
	contactListCalls int
	lastSeenCalls    int
	lastSeenRequest  *userpb.TLUserGetLastSeens
}

func (s *getStatusesUserClientStub) UserGetContactList(context.Context, *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	s.contactListCalls++
	return s.contactList, s.contactErr
}

func (s *getStatusesUserClientStub) UserGetLastSeens(_ context.Context, in *userpb.TLUserGetLastSeens) (*userpb.Vector_LastSeenData, error) {
	s.lastSeenCalls++
	s.lastSeenRequest = in
	return s.lastSeenList, s.lastSeenErr
}

func newGetStatusesTestCore(client *getStatusesUserClientStub) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func TestContactsGetStatusesPropagatesDependencyErrors(t *testing.T) {
	contactErr := errors.New("contact list unavailable")
	client := &getStatusesUserClientStub{contactErr: contactErr}
	got, err := newGetStatusesTestCore(client).ContactsGetStatuses(&mtproto.TLContactsGetStatuses{})
	if got != nil || err != contactErr {
		t.Fatalf("ContactsGetStatuses() = (%+v, %v), want (nil, contact error)", got, err)
	}
	if client.lastSeenCalls != 0 {
		t.Fatalf("UserGetLastSeens called %d times after contact list failure, want 0", client.lastSeenCalls)
	}

	lastSeenErr := errors.New("last seen unavailable")
	client = &getStatusesUserClientStub{
		contactList: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 84}}},
		lastSeenErr: lastSeenErr,
	}
	got, err = newGetStatusesTestCore(client).ContactsGetStatuses(&mtproto.TLContactsGetStatuses{})
	if got != nil || err != lastSeenErr {
		t.Fatalf("ContactsGetStatuses() = (%+v, %v), want (nil, last seen error)", got, err)
	}
}

func TestContactsGetStatusesHandlesEmptyAndNilResponses(t *testing.T) {
	tests := []struct {
		name              string
		contactList       *userpb.Vector_ContactData
		lastSeen          *userpb.Vector_LastSeenData
		wantLastSeenCalls int
	}{
		{name: "nil contact response"},
		{
			name: "nil last seen response",
			contactList: &userpb.Vector_ContactData{
				Datas: []*mtproto.ContactData{{ContactUserId: 84}},
			},
			wantLastSeenCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &getStatusesUserClientStub{
				contactList:  tt.contactList,
				lastSeenList: tt.lastSeen,
			}
			got, err := newGetStatusesTestCore(client).ContactsGetStatuses(&mtproto.TLContactsGetStatuses{})
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("ContactsGetStatuses() = (%+v, %v), want internal server error", got, err)
			}
			if client.lastSeenCalls != tt.wantLastSeenCalls {
				t.Fatalf("UserGetLastSeens called %d times, want %d", client.lastSeenCalls, tt.wantLastSeenCalls)
			}
		})
	}

	client := &getStatusesUserClientStub{contactList: &userpb.Vector_ContactData{}}
	got, err := newGetStatusesTestCore(client).ContactsGetStatuses(&mtproto.TLContactsGetStatuses{})
	if err != nil || got == nil || len(got.Datas) != 0 {
		t.Fatalf("ContactsGetStatuses() for empty contacts = (%+v, %v), want empty vector", got, err)
	}
	if client.lastSeenCalls != 0 {
		t.Fatalf("UserGetLastSeens called %d times for empty contacts, want 0", client.lastSeenCalls)
	}
}

func TestContactsGetStatusesIncludesContactsWithoutLastSeen(t *testing.T) {
	client := &getStatusesUserClientStub{
		contactList: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{
			{ContactUserId: 84},
			{ContactUserId: 85},
		}},
		lastSeenList: &userpb.Vector_LastSeenData{Datas: []*userpb.LastSeenData{
			{UserId: 84, LastSeenAt: 1},
		}},
	}
	got, err := newGetStatusesTestCore(client).ContactsGetStatuses(&mtproto.TLContactsGetStatuses{})
	if err != nil || got == nil || len(got.Datas) != 2 {
		t.Fatalf("ContactsGetStatuses() = (%+v, %v), want one status per contact", got, err)
	}
	if got.Datas[0].GetUserId() != 84 || got.Datas[0].GetStatus() == nil {
		t.Fatalf("status = %+v, want user 84 with a status", got.Datas[0])
	}
	if got.Datas[1].GetUserId() != 85 || got.Datas[1].GetStatus().GetPredicateName() != "userStatusEmpty" {
		t.Fatalf("status = %+v, want user 85 with userStatusEmpty", got.Datas[1])
	}
	if client.lastSeenRequest == nil || !reflect.DeepEqual(client.lastSeenRequest.Id, []int64{84, 85}) {
		t.Fatalf("user.getLastSeens request IDs = %v, want [84 85]", client.lastSeenRequest.GetId())
	}
}

func TestContactsGetStatusesRejectsNilEntries(t *testing.T) {
	tests := []struct {
		name              string
		contactList       *userpb.Vector_ContactData
		lastSeen          *userpb.Vector_LastSeenData
		wantLastSeenCalls int
	}{
		{
			name:        "nil contact",
			contactList: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{nil}},
		},
		{
			name: "nil last seen",
			contactList: &userpb.Vector_ContactData{
				Datas: []*mtproto.ContactData{{ContactUserId: 84}},
			},
			lastSeen:          &userpb.Vector_LastSeenData{Datas: []*userpb.LastSeenData{nil}},
			wantLastSeenCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &getStatusesUserClientStub{contactList: tt.contactList, lastSeenList: tt.lastSeen}
			got, err := newGetStatusesTestCore(client).ContactsGetStatuses(&mtproto.TLContactsGetStatuses{})
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("ContactsGetStatuses() = (%+v, %v), want internal server error", got, err)
			}
			if client.lastSeenCalls != tt.wantLastSeenCalls {
				t.Fatalf("UserGetLastSeens called %d times, want %d", client.lastSeenCalls, tt.wantLastSeenCalls)
			}
		})
	}
}
