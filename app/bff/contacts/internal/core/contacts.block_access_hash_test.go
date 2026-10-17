package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestContactsBlockRejectsMissingOrStaleUserAccessHash(t *testing.T) {
	for _, tt := range []struct {
		name string
		hash int64
	}{
		{name: "missing", hash: 0},
		{name: "stale", hash: 999},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &blockUserClientStub{
				users:      blockTestUsers(42, 84),
				blockReply: mtproto.BoolTrue,
			}
			core := newBlockTestCore(42, client, &blockSyncClientStub{})
			got, err := core.ContactsBlock(&mtproto.TLContactsBlock{
				Id: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: tt.hash}).To_InputPeer(),
			})
			if got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("ContactsBlock() = (%v, %v), want nil and PEER_ID_INVALID", got, err)
			}
			if client.blockCalls != 0 {
				t.Fatalf("UserBlockPeer calls = %d, want 0", client.blockCalls)
			}
		})
	}
}

func TestContactsUnblockRejectsMissingOrStaleUserAccessHash(t *testing.T) {
	for _, tt := range []struct {
		name string
		hash int64
	}{
		{name: "missing", hash: 0},
		{name: "stale", hash: 999},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &unblockUserClientStub{
				users:  unblockTestUsers(42, 84),
				result: mtproto.BoolTrue,
			}
			core := newUnblockTestCore(42, client, &unblockSyncClientStub{})
			got, err := core.ContactsUnblock(&mtproto.TLContactsUnblock{
				Id: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 84, AccessHash: tt.hash}).To_InputPeer(),
			})
			if got != nil || !errors.Is(err, mtproto.ErrPeerIdInvalid) {
				t.Fatalf("ContactsUnblock() = (%v, %v), want nil and PEER_ID_INVALID", got, err)
			}
			if client.unblockCalls != 0 {
				t.Fatalf("UserUnBlockPeer calls = %d, want 0", client.unblockCalls)
			}
		})
	}
}
