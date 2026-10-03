package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	updatesdao "github.com/teamgram/teamgram-server/app/bff/updates/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/updates/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	updatesclient "github.com/teamgram/teamgram-server/app/service/biz/updates/client"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type secretUpdatesReaderStub struct {
	current int32
	diff    updatesdao.SecretDifference
	err     error
	qts     int32
	limit   int32
}

func (s *secretUpdatesReaderStub) CurrentQTS(context.Context, int64) (int32, error) {
	return s.current, s.err
}

func (s *secretUpdatesReaderStub) GetDifference(_ context.Context, _ int64, qts, limit int32) (updatesdao.SecretDifference, error) {
	s.qts = qts
	s.limit = limit
	return s.diff, s.err
}

type updatesClientStub struct {
	updatesclient.UpdatesClient
	state *mtproto.Updates_State
	diff  *updates.Difference
}

func (s *updatesClientStub) UpdatesGetStateV2(context.Context, *updates.TLUpdatesGetStateV2) (*mtproto.Updates_State, error) {
	return s.state, nil
}

func (s *updatesClientStub) UpdatesGetDifferenceV2(context.Context, *updates.TLUpdatesGetDifferenceV2) (*updates.Difference, error) {
	return s.diff, nil
}

type authsessionClientStub struct {
	authsessionclient.AuthsessionClient
	calls int
}

func (s *authsessionClientStub) AuthsessionGetPermAuthKeyId(context.Context, *authsession.TLAuthsessionGetPermAuthKeyId) (*mtproto.Int64, error) {
	s.calls++
	return mtproto.MakeTLInt64(&mtproto.Int64{V: 11}).To_Int64(), nil
}

type userClientStub struct {
	userclient.UserClient
}

func (s *userClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
}

func secretUpdatesCore(reader updatesdao.SecretUpdatesReader, backend updatesclient.UpdatesClient, auth authsessionclient.AuthsessionClient) *UpdatesCore {
	core := New(context.Background(), &svc.ServiceContext{Dao: &updatesdao.Dao{
		SecretUpdates:     reader,
		UpdatesClient:     backend,
		AuthsessionClient: auth,
		UserClient:        &userClientStub{},
	}})
	core.MD = &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 9}
	return core
}

func TestUpdatesGetStateUsesAuthoritativeSecretQTS(t *testing.T) {
	state := mtproto.MakeTLUpdatesState(&mtproto.Updates_State{Pts: 3, Qts: 99, Date: 4, Seq: 5}).To_Updates_State()
	core := secretUpdatesCore(
		&secretUpdatesReaderStub{current: 8},
		&updatesClientStub{state: state},
		nil,
	)

	got, err := core.UpdatesGetState(&mtproto.TLUpdatesGetState{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPts() != 3 || got.GetQts() != 8 || got.GetSeq() != 5 {
		t.Fatalf("state = %+v", got)
	}
}

func TestUpdatesGetDifferenceReturnsOrderedSecretSlice(t *testing.T) {
	reader := &secretUpdatesReaderStub{diff: updatesdao.SecretDifference{
		CurrentQTS: 5,
		HasMore:    true,
		Messages: []updatesdao.SecretMessage{
			{ChatID: 20, RandomID: 101, QTS: 3, Date: 30, Data: []byte("first")},
			{ChatID: 20, RandomID: 102, QTS: 4, Date: 31, Data: []byte("service"), Service: true},
		},
	}}
	backend := &updatesClientStub{diff: updates.MakeTLDifferenceEmpty(&updates.Difference{
		State: mtproto.MakeTLUpdatesState(&mtproto.Updates_State{Pts: 12, Date: 40, Seq: 2}).To_Updates_State(),
	}).To_Difference()}
	core := secretUpdatesCore(reader, backend, &authsessionClientStub{})

	got, err := core.UpdatesGetDifference(&mtproto.TLUpdatesGetDifference{
		Pts: 12, Date: 40, Qts: 2, QtsLimit: wrapperspb.Int32(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reader.qts != 2 || reader.limit != 2 {
		t.Fatalf("reader arguments = qts %d limit %d", reader.qts, reader.limit)
	}
	if got.GetPredicateName() != mtproto.Predicate_updates_differenceSlice || got.GetIntermediateState().GetQts() != 4 {
		t.Fatalf("difference = %+v", got)
	}
	messages := got.GetNewEncryptedMessages()
	if len(messages) != 2 || messages[0].GetRandomId() != 101 || messages[1].GetRandomId() != 102 {
		t.Fatalf("encrypted messages = %+v", messages)
	}
	if messages[0].GetPredicateName() != mtproto.Predicate_encryptedMessage || messages[0].GetFile().GetPredicateName() != mtproto.Predicate_encryptedFileEmpty {
		t.Fatalf("ordinary encrypted message = %+v", messages[0])
	}
	if messages[1].GetPredicateName() != mtproto.Predicate_encryptedMessageService {
		t.Fatalf("service encrypted message = %+v", messages[1])
	}
	if err = got.Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("encode Layer 229 difference slice: %v", err)
	}
}

func TestUpdatesGetDifferenceReturnsCompleteSecretTail(t *testing.T) {
	reader := &secretUpdatesReaderStub{diff: updatesdao.SecretDifference{
		CurrentQTS: 3,
		Messages: []updatesdao.SecretMessage{{
			ChatID: 20, RandomID: 103, QTS: 3, Date: 32, Data: []byte("file"),
			File: &updatesdao.SecretFile{ID: 50, AccessHash: 51, Size: 52, DCID: 4, KeyFingerprint: 53},
		}},
	}}
	backend := &updatesClientStub{diff: updates.MakeTLDifferenceEmpty(&updates.Difference{
		State: mtproto.MakeTLUpdatesState(&mtproto.Updates_State{Pts: 12, Date: 40, Seq: 2}).To_Updates_State(),
	}).To_Difference()}
	core := secretUpdatesCore(reader, backend, &authsessionClientStub{})

	got, err := core.UpdatesGetDifference(&mtproto.TLUpdatesGetDifference{Pts: 12, Date: 40, Qts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_updates_difference || got.GetState().GetQts() != 3 {
		t.Fatalf("difference = %+v", got)
	}
	file := got.GetNewEncryptedMessages()[0].GetFile()
	if file.GetPredicateName() != mtproto.Predicate_encryptedFile || file.GetId() != 50 || file.GetSize2_INT64() != 52 {
		t.Fatalf("encrypted file = %+v", file)
	}
	if err = got.Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("encode Layer 229 difference: %v", err)
	}
}

func TestUpdatesGetDifferenceRejectsFutureQTSBeforeBackendCalls(t *testing.T) {
	reader := &secretUpdatesReaderStub{err: updatesdao.ErrMaxQTSInvalid}
	auth := &authsessionClientStub{}
	core := secretUpdatesCore(reader, &updatesClientStub{}, auth)

	got, err := core.UpdatesGetDifference(&mtproto.TLUpdatesGetDifference{Qts: 6})
	if got != nil || !errors.Is(err, mtproto.ErrMaxQtsInvalid) {
		t.Fatalf("result = (%+v, %v)", got, err)
	}
	if auth.calls != 0 {
		t.Fatalf("auth backend called %d times", auth.calls)
	}
}
