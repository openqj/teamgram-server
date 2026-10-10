package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestConferenceCallsFailClosedWithoutMediaBackend(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81006}}
	checks := []struct {
		name string
		call func() error
		want error
	}{
		{"create", func() error { _, err := c.PhoneCreateConferenceCall7D0444BB(nil); return err }, mtproto.ErrMethodNotImpl},
		{"chain blocks", func() error { _, err := c.PhoneGetGroupCallChainBlocks(nil); return err }, mtproto.ErrMethodNotImpl},
		{"encrypted message", func() error { _, err := c.PhoneSendGroupCallEncryptedMessage(nil); return err }, mtproto.ErrGroupCallInvalid},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, check.want) {
				t.Fatalf("error = %v, want %v", err, check.want)
			}
		})
	}
}

func TestConferenceBroadcastAndChainReadback(t *testing.T) {
	uid := time.Now().UnixNano()
	randomID := int32(uid)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	created, err := c.PhoneCreateConferenceCall7D0444BB(&mtproto.TLPhoneCreateConferenceCall7D0444BB{
		RandomId:  randomID,
		PublicKey: []byte("conference-public-key"),
		Block:     []byte("initial-block"),
	})
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create conference: result=%#v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	if call == nil || call.GetId() == 0 || call.GetAccessHash() == 0 {
		t.Fatalf("create conference returned no call: %#v", call)
	}
	t.Cleanup(func() { _, _ = domain.DeleteGroupCall(call.GetId()) })
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}

	broadcast, err := c.PhoneSendConferenceCallBroadcast(&mtproto.TLPhoneSendConferenceCallBroadcast{
		Call:  input,
		Block: []byte("broadcast-block"),
	})
	if err != nil || broadcast == nil || len(broadcast.GetUpdates()) != 1 {
		t.Fatalf("broadcast: result=%#v err=%v", broadcast, err)
	}
	if got := broadcast.GetUpdates()[0].GetBlocks(); len(got) != 1 || string(got[0]) != "broadcast-block" {
		t.Fatalf("broadcast blocks = %#v, want persisted block", got)
	}
	// A client may retry a broadcast after a transport failure. Replaying the
	// same block must remain successful even when MySQL reports zero changed rows.
	replay, err := c.PhoneSendConferenceCallBroadcast(&mtproto.TLPhoneSendConferenceCallBroadcast{
		Call:  input,
		Block: []byte("broadcast-block"),
	})
	if err != nil || replay == nil || len(replay.GetUpdates()) != 1 {
		t.Fatalf("broadcast replay: result=%#v err=%v", replay, err)
	}

	read, err := c.PhoneGetGroupCallChainBlocks(&mtproto.TLPhoneGetGroupCallChainBlocks{
		Call:  input,
		Limit: 1,
	})
	if err != nil || read == nil || len(read.GetUpdates()) != 1 {
		t.Fatalf("chain read: result=%#v err=%v", read, err)
	}
	if got := read.GetUpdates()[0].GetBlocks(); len(got) != 1 || string(got[0]) != "broadcast-block" {
		t.Fatalf("chain read blocks = %#v, want broadcast block", got)
	}
	encrypted, err := c.PhoneSendGroupCallEncryptedMessage(&mtproto.TLPhoneSendGroupCallEncryptedMessage{
		Call: input, EncryptedMessage: []byte("opaque-ciphertext"),
	})
	if err != nil || encrypted != mtproto.BoolTrue {
		t.Fatalf("encrypted group-call message: result=%v err=%v", encrypted, err)
	}

	if _, err = c.PhoneGetGroupCallChainBlocks(&mtproto.TLPhoneGetGroupCallChainBlocks{
		Call: &mtproto.InputGroupCall{Id: input.Id, AccessHash: input.AccessHash + 1},
	}); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("wrong access hash error = %v, want GROUPCALL_INVALID", err)
	}
}

func TestConferenceParticipantInviteAndDeleteReadback(t *testing.T) {
	owner := time.Now().UnixNano()
	target := owner + 1
	userClient := &groupCallUserClient{users: map[int64]*mtproto.UserData{
		target: {Id: target, AccessHash: target + 100},
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: userClient}},
		MD:     &metadata.RpcMetadata{UserId: owner},
	}
	created, err := c.PhoneCreateConferenceCall7D0444BB(&mtproto.TLPhoneCreateConferenceCall7D0444BB{
		RandomId:  int32(owner),
		PublicKey: []byte("conference-public-key"),
		Block:     []byte("initial-block"),
	})
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create conference: result=%#v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	if call == nil {
		t.Fatalf("create conference returned no call: %#v", created)
	}
	t.Cleanup(func() { _, _ = domain.DeleteGroupCall(call.GetId()) })
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}

	invite, err := c.PhoneInviteConferenceCallParticipant(&mtproto.TLPhoneInviteConferenceCallParticipant{
		Call:   input,
		UserId: &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: target, AccessHash: target + 100},
	})
	if err != nil || invite == nil || len(invite.GetUpdates()) != 1 {
		t.Fatalf("invite conference participant: result=%#v err=%v", invite, err)
	}
	participants := invite.GetUpdates()[0].GetParticipants_VECTORGROUPCALLPARTICIPANT()
	if len(participants) != 2 {
		t.Fatalf("invite update participants = %d, want creator and target", len(participants))
	}
	stored, ok, err := domain.LoadGroupCallRecord(call.GetId())
	if err != nil || !ok || !groupCallHasUser(decodeGroupUserIDs(stored.Participants), target) {
		t.Fatalf("invited participant not persisted: record=%#v ok=%v err=%v", stored, ok, err)
	}

	removed, err := c.PhoneDeleteConferenceCallParticipants(&mtproto.TLPhoneDeleteConferenceCallParticipants{
		Call: input, Ids: []int64{target},
	})
	if err != nil || removed == nil || len(removed.GetUpdates()) != 1 {
		t.Fatalf("delete conference participant: result=%#v err=%v", removed, err)
	}
	participants = removed.GetUpdates()[0].GetParticipants_VECTORGROUPCALLPARTICIPANT()
	if len(participants) != 1 || participants[0].GetPeer().GetUserId() != owner {
		t.Fatalf("delete update participants = %+v, want creator only", participants)
	}
}

func TestConferenceParticipantDeclineRemovesInvitee(t *testing.T) {
	owner := time.Now().UnixNano()
	target := owner + 1
	userClient := &groupCallUserClient{users: map[int64]*mtproto.UserData{
		target: {Id: target, AccessHash: target + 100},
	}}
	ownerCore := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: userClient}},
		MD:     &metadata.RpcMetadata{UserId: owner},
	}
	created, err := ownerCore.PhoneCreateConferenceCall7D0444BB(&mtproto.TLPhoneCreateConferenceCall7D0444BB{
		RandomId:  int32(owner),
		PublicKey: []byte("conference-public-key"),
		Block:     []byte("initial-block"),
	})
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create conference: result=%#v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	if call == nil {
		t.Fatalf("create conference returned no call: %#v", created)
	}
	t.Cleanup(func() { _, _ = domain.DeleteGroupCall(call.GetId()) })
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}
	if _, err = ownerCore.PhoneInviteConferenceCallParticipant(&mtproto.TLPhoneInviteConferenceCallParticipant{
		Call:   input,
		UserId: &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: target, AccessHash: target + 100},
	}); err != nil {
		t.Fatalf("invite conference participant: %v", err)
	}

	const msgID int32 = 77
	action := mtproto.MakeTLMessageActionConferenceCall(&mtproto.MessageAction{Call: input}).To_MessageAction()
	reader := &pollMessageReaderStub{boxes: map[int64]*mtproto.MessageBox{
		target: mtproto.MakeTLMessageBox(&mtproto.MessageBox{
			MessageId: msgID,
			Message:   mtproto.MakeTLMessage(&mtproto.Message{Id: msgID, Action: action}).To_Message(),
		}).To_MessageBox(),
	}}
	targetCore := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{PollMessageReader: reader}},
		MD:     &metadata.RpcMetadata{UserId: target},
	}
	declined, err := targetCore.PhoneDeclineConferenceCallInvite(&mtproto.TLPhoneDeclineConferenceCallInvite{MsgId: msgID})
	if err != nil || declined == nil || len(declined.GetUpdates()) != 1 {
		t.Fatalf("decline conference invite: result=%#v err=%v", declined, err)
	}
	participants := declined.GetUpdates()[0].GetParticipants_VECTORGROUPCALLPARTICIPANT()
	if len(participants) != 1 || participants[0].GetPeer().GetUserId() != owner {
		t.Fatalf("decline update participants = %+v, want creator only", participants)
	}
	stored, ok, err := domain.LoadGroupCallRecord(call.GetId())
	if err != nil || !ok || groupCallHasUser(decodeGroupUserIDs(stored.Participants), target) {
		t.Fatalf("declined participant still persisted: record=%#v ok=%v err=%v", stored, ok, err)
	}
}
