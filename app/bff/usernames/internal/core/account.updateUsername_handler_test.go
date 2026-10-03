package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type updateUsernameUserClient struct {
	user_client.UserClient
	getCalls    int
	updateCalls int
	updateBool  *mtproto.Bool
	updateErr   error
	getErr      error
	emptyReply  bool
}

func (f *updateUsernameUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.emptyReply {
		return nil, nil
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: &mtproto.UserData{Id: 42, FirstName: "Audit", LastName: "User", Username: "oldname"},
	}).To_ImmutableUser(), nil
}

func (f *updateUsernameUserClient) UserUpdateUsername(context.Context, *userpb.TLUserUpdateUsername) (*mtproto.Bool, error) {
	f.updateCalls++
	return f.updateBool, f.updateErr
}

type updateUsernameSyncClient struct {
	sync_client.SyncClient
	calls int
	err   error
	reply *mtproto.Void
}

func (f *updateUsernameSyncClient) SyncUpdatesNotMe(context.Context, *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	f.calls++
	return f.reply, f.err
}

func TestAccountUpdateUsernameRejectsInvalidFormatBeforeRPC(t *testing.T) {
	client := &updateUsernameUserClient{}
	core := newUpdateUsernameTestCore(client)

	for _, username := range []string{"abc", "1invalid", "has-hyphen"} {
		if _, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: username}); err != mtproto.ErrUsernameInvalid {
			t.Errorf("username %q: got error %v, want %v", username, err, mtproto.ErrUsernameInvalid)
		}
	}
	if client.getCalls != 0 || client.updateCalls != 0 {
		t.Fatalf("invalid usernames reached user service: get=%d update=%d", client.getCalls, client.updateCalls)
	}
}

func TestAccountUpdateUsernamePropagatesWriteFailure(t *testing.T) {
	writeErr := errors.New("injected username write failure")
	client := &updateUsernameUserClient{updateErr: writeErr}
	core := newUpdateUsernameTestCore(client)

	_, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: "newname"})
	if err != writeErr {
		t.Fatalf("got error %v, want propagated error %v", err, writeErr)
	}
	if client.getCalls != 1 || client.updateCalls != 1 {
		t.Fatalf("unexpected RPC calls: get=%d update=%d", client.getCalls, client.updateCalls)
	}
}

func TestAccountUpdateUsernameRejectsFalseBackendReply(t *testing.T) {
	client := &updateUsernameUserClient{updateBool: mtproto.BoolFalse}
	core := newUpdateUsernameTestCore(client)

	if _, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: "newname"}); err != mtproto.ErrInternalServerError {
		t.Fatalf("got error %v, want %v", err, mtproto.ErrInternalServerError)
	}
}

func TestAccountUpdateUsernameRequiresAuthenticationAndRequest(t *testing.T) {
	client := &updateUsernameUserClient{}
	core := newUpdateUsernameTestCore(client)
	core.MD = &metadata.RpcMetadata{}
	if _, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: "newname"}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated update error = %v, want %v", err, mtproto.ErrAuthKeyUnregistered)
	}
	core.MD.UserId = 42
	if _, err := core.AccountUpdateUsername(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil request error = %v, want %v", err, mtproto.ErrInputRequestInvalid)
	}
	if client.getCalls != 0 || client.updateCalls != 0 {
		t.Fatalf("rejected requests reached user service: get=%d update=%d", client.getCalls, client.updateCalls)
	}
}

func TestAccountUpdateUsernameRejectsEmptyUserResponse(t *testing.T) {
	client := &updateUsernameUserClient{emptyReply: true}
	core := newUpdateUsernameTestCore(client)

	if _, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: "newname"}); err != mtproto.ErrInternalServerError {
		t.Fatalf("empty user response error = %v, want %v", err, mtproto.ErrInternalServerError)
	}
	if client.updateCalls != 0 {
		t.Fatalf("empty user response still reached update RPC: %d", client.updateCalls)
	}
}

func TestAccountUpdateUsernamePropagatesSyncFailure(t *testing.T) {
	syncErr := errors.New("injected sync failure")
	client := &updateUsernameUserClient{updateBool: mtproto.BoolTrue}
	syncClient := &updateUsernameSyncClient{reply: &mtproto.Void{}, err: syncErr}
	core := newUpdateUsernameTestCore(client)
	core.svcCtx.Dao.SyncClient = syncClient

	if _, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: "newname"}); !errors.Is(err, syncErr) {
		t.Fatalf("sync failure = %v, want wrapped %v", err, syncErr)
	}
	if syncClient.calls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncClient.calls)
	}
}

func TestAccountUpdateUsernameRejectsEmptySyncResponse(t *testing.T) {
	client := &updateUsernameUserClient{updateBool: mtproto.BoolTrue}
	syncClient := &updateUsernameSyncClient{}
	core := newUpdateUsernameTestCore(client)
	core.svcCtx.Dao.SyncClient = syncClient

	if _, err := core.AccountUpdateUsername(&mtproto.TLAccountUpdateUsername{Username: "newname"}); err == nil {
		t.Fatal("empty sync response was reported as success")
	}
}

func newUpdateUsernameTestCore(client *updateUsernameUserClient) *UsernamesCore {
	core := New(context.Background(), &svc.ServiceContext{
		Dao: &dao.Dao{
			UserClient: client,
			SyncClient: &updateUsernameSyncClient{reply: &mtproto.Void{}},
		},
	})
	core.MD = &metadata.RpcMetadata{UserId: 42}
	return core
}
