package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestContactsDeleteByPhonesRejectsNilDeleteProviderResults(t *testing.T) {
	tests := []struct {
		name   string
		client *deleteByPhonesUserClientStub
	}{
		{
			name: "existing contact",
			client: &deleteByPhonesUserClientStub{
				contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{
					ContactUserId: 44,
					Phone:         wrapperspb.String("12025550102"),
				}}},
				deleteImportResponse: mtproto.BoolTrue,
			},
		},
		{
			name: "unregistered import",
			client: &deleteByPhonesUserClientStub{
				contacts:       &userpb.Vector_ContactData{},
				deleteResponse: mtproto.BoolTrue,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const userID = int64(82033)
			store := &importContactsStore{data: map[string]string{}}
			withImportContactsStore(t, store)

			result, err := newImportContactsTestCore(userID, tt.client).ContactsDeleteByPhones(&mtproto.TLContactsDeleteByPhones{
				Phones: []string{"+1 202 555 0102"},
			})
			if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("nil delete provider result = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
			}
		})
	}
}
