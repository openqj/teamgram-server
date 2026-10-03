package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func TestApplyDialogFullChatFields(t *testing.T) {
	full := &mtproto.ChatFull{TtlPeriod: mtproto.MakeFlagsInt32(60)}
	ext := dialog.MakeTLDialogExt(&dialog.DialogExt{
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			FolderId: mtproto.MakeFlagsInt32(7),
		}).To_Dialog(),
		TtlPeriod:     86400,
		ThemeEmoticon: "\U0001F525",
	}).To_DialogExt()

	applyDialogFullChatFields(full, ext)

	if got := full.GetFolderId().GetValue(); got != 7 {
		t.Fatalf("folder id: got %d, want 7", got)
	}
	if got := full.GetTtlPeriod().GetValue(); got != 86400 {
		t.Fatalf("ttl period: got %d, want 86400", got)
	}
	if got := full.GetThemeEmoticon().GetValue(); got != "\U0001F525" {
		t.Fatalf("theme emoticon: got %q", got)
	}
}

func TestSetFullChatPinnedMessage(t *testing.T) {
	full := &mtproto.ChatFull{}
	setFullChatPinnedMessage(full, &mtproto.Int32{V: 42})
	if got := full.GetPinnedMsgId().GetValue(); got != 42 {
		t.Fatalf("pinned message id: got %d, want 42", got)
	}

	setFullChatPinnedMessage(full, &mtproto.Int32{V: 0})
	if got := full.GetPinnedMsgId().GetValue(); got != 42 {
		t.Fatalf("zero pinned message id cleared value: got %d, want 42", got)
	}
}

func TestValidateFullChatMutableChat(t *testing.T) {
	if err := validateFullChatMutableChat(nil); err != mtproto.ErrInternalServerError {
		t.Fatalf("nil chat: got %v, want internal server error", err)
	}
	if err := validateFullChatMutableChat(&mtproto.MutableChat{}); err != mtproto.ErrInternalServerError {
		t.Fatalf("missing immutable chat: got %v, want internal server error", err)
	}
	if err := validateFullChatMutableChat(&mtproto.MutableChat{
		Chat:             &mtproto.ImmutableChat{},
		ChatParticipants: []*mtproto.ImmutableChatParticipant{nil},
	}); err != mtproto.ErrInternalServerError {
		t.Fatalf("nil participant: got %v, want internal server error", err)
	}
}

func TestHasFullChatMembershipRequiresAnActiveParticipant(t *testing.T) {
	chat := &mtproto.MutableChat{
		ChatParticipants: []*mtproto.ImmutableChatParticipant{
			{UserId: 1, State: mtproto.ChatMemberStateNormal},
			{UserId: 2, State: mtproto.ChatMemberStateLeft},
			{UserId: 3, State: mtproto.ChatMemberStateKicked},
		},
	}

	if _, ok := getFullChatMember(chat, 1); !ok {
		t.Fatal("active participant is not a full-chat member")
	}
	if _, ok := getFullChatMember(chat, 2); ok {
		t.Fatal("left participant can still read full chat")
	}
	if _, ok := getFullChatMember(chat, 3); ok {
		t.Fatal("kicked participant can still read full chat")
	}
	if _, ok := getFullChatMember(chat, 4); ok {
		t.Fatal("unknown participant can still read full chat")
	}
}

func TestGetFullChatDialogValidatesResponse(t *testing.T) {
	if _, err := getFullChatDialog(nil); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("nil dialog response: got %v, want peer invalid", err)
	}
	if _, err := getFullChatDialog(&dialog.Vector_DialogExt{}); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("empty dialog response: got %v, want peer invalid", err)
	}
	if _, err := getFullChatDialog(&dialog.Vector_DialogExt{Datas: []*dialog.DialogExt{nil}}); err != mtproto.ErrInternalServerError {
		t.Fatalf("nil dialog entity: got %v, want internal server error", err)
	}
	if _, err := getFullChatDialog(&dialog.Vector_DialogExt{Datas: []*dialog.DialogExt{{}}}); err != mtproto.ErrInternalServerError {
		t.Fatalf("missing nested dialog: got %v, want internal server error", err)
	}
	got, err := getFullChatDialog(&dialog.Vector_DialogExt{Datas: []*dialog.DialogExt{{
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{}).To_Dialog(),
	}}})
	if err != nil || got == nil {
		t.Fatalf("valid dialog response: got (%v, %v), want non-nil dialog", got, err)
	}
}
