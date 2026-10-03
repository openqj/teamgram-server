package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestSecretChatProtocolObjects(t *testing.T) {
	chat := domain.SecretChat{ID: 7, AccessHash: 8, AdminID: 9, ParticipantID: 10, GA: []byte{2}, GB: []byte{3}, KeyFingerprint: 11, CreatedAt: 12}
	waiting := secretChatWaiting(chat)
	if waiting.GetPredicateName() != mtproto.Predicate_encryptedChatWaiting || waiting.GetAccessHash() != 8 {
		t.Fatalf("waiting = %+v", waiting)
	}
	requested := secretChatRequested(chat)
	if requested.GetPredicateName() != mtproto.Predicate_encryptedChatRequested || string(requested.GetGA()) != string(chat.GA) {
		t.Fatalf("requested = %+v", requested)
	}
	admin := secretChatActive(chat, chat.AdminID)
	participant := secretChatActive(chat, chat.ParticipantID)
	if string(admin.GetGAOrB()) != string(chat.GB) || string(participant.GetGAOrB()) != string(chat.GA) {
		t.Fatalf("active public values = (%x, %x)", admin.GetGAOrB(), participant.GetGAOrB())
	}
	chat.HistoryDeleted = true
	discarded := secretChatDiscarded(chat)
	if discarded.GetPredicateName() != mtproto.Predicate_encryptedChatDiscarded || !discarded.GetHistoryDeleted() {
		t.Fatalf("discarded = %+v", discarded)
	}
}

func TestSecretMessageWithoutUploadUsesEmptyEncryptedFile(t *testing.T) {
	message := secretEncryptedMessage(domain.SecretMessage{
		ChatID: 7, RandomID: 8, Date: 9, Data: []byte{10},
	})
	if message.GetPredicateName() != mtproto.Predicate_encryptedMessage || message.GetFile().GetPredicateName() != mtproto.Predicate_encryptedFileEmpty {
		t.Fatalf("message = %+v", message)
	}
	if err := message.Encode(mtproto.NewEncodeBuf(128), 229); err != nil {
		t.Fatalf("encode Layer 229 encrypted message: %v", err)
	}
}

func TestSecretChatInputValidation(t *testing.T) {
	anon := &ApiFullCore{}
	if _, err := anon.MessagesRequestEncryption(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("anonymous request = %v", err)
	}
	authenticated := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 5}}
	if _, err := authenticated.MessagesRequestEncryption(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("empty request = %v", err)
	}
	if _, err := authenticated.MessagesRequestEncryption(&mtproto.TLMessagesRequestEncryption{RandomId: 1, GA: []byte{1}}); !errors.Is(err, mtproto.ErrDhGAInvalid) {
		t.Fatalf("invalid g_a = %v", err)
	}
	if _, err := authenticated.MessagesAcceptEncryption(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("empty accept = %v", err)
	}
	if _, err := authenticated.MessagesReceivedQueue(&mtproto.TLMessagesReceivedQueue{}); !errors.Is(err, mtproto.ErrMaxQtsInvalid) {
		t.Fatalf("empty queue confirmation = %v", err)
	}
	if _, err := authenticated.MessagesGetDhConfig(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("empty DH config request = %v", err)
	}
}

func TestMessagesGetDhConfigVersionAndRandom(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 5}}
	got, err := c.MessagesGetDhConfig(&mtproto.TLMessagesGetDhConfig{RandomLength: 32, Version: 0})
	if err != nil {
		t.Fatalf("get DH config: %v", err)
	}
	config := got.To_MessagesDhConfig()
	if config.GetG() != 3 || config.GetVersion() != dhConfigVersion || len(config.GetP()) != len(telegramDHPrime) {
		t.Fatalf("unexpected DH config: %+v", config)
	}
	if len(config.GetRandom()) != 32 {
		t.Fatalf("random length = %d, want 32", len(config.GetRandom()))
	}
	notModified, err := c.MessagesGetDhConfig(&mtproto.TLMessagesGetDhConfig{RandomLength: 16, Version: dhConfigVersion})
	if err != nil {
		t.Fatalf("get unchanged DH config: %v", err)
	}
	if notModified.To_MessagesDhConfig().GetPredicateName() != mtproto.Predicate_messages_dhConfigNotModified {
		t.Fatalf("versioned response = %s", notModified.To_MessagesDhConfig().GetPredicateName())
	}
	if len(notModified.To_MessagesDhConfig().GetRandom()) != 16 {
		t.Fatalf("versioned random length = %d, want 16", len(notModified.To_MessagesDhConfig().GetRandom()))
	}
}

func TestNewSecretAccessHash(t *testing.T) {
	first, err := newSecretAccessHash()
	if err != nil || first == 0 {
		t.Fatalf("first = (%d, %v)", first, err)
	}
	second, err := newSecretAccessHash()
	if err != nil || second == 0 || second == first {
		t.Fatalf("second = (%d, %v), first = %d", second, err, first)
	}
}
