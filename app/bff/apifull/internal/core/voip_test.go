package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type voipUserClient struct {
	user_client.UserClient
	users map[int64]*mtproto.UserData
}

func (c *voipUserClient) UserGetImmutableUserV2(_ context.Context, in *userpb.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	if in == nil || c.users[in.GetId()] == nil {
		return nil, nil
	}
	return &mtproto.ImmutableUser{User: c.users[in.GetId()]}, nil
}

type voipSyncClient struct {
	sync_client.SyncClient
	updates []*syncpb.TLSyncPushUpdates
}

func (c *voipSyncClient) SyncPushUpdates(_ context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	c.updates = append(c.updates, in)
	return mtproto.EmptyVoid, nil
}

func voipCore(userID int64, users map[int64]*mtproto.UserData, syncer *voipSyncClient) *ApiFullCore {
	return &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: userID},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient: &voipUserClient{users: users},
			SyncClient: syncer,
		}},
	}
}

func voipUser(id, accessHash int64) *mtproto.UserData {
	return &mtproto.UserData{Id: id, AccessHash: accessHash, FirstName: "Call"}
}

func voipPeer(id, accessHash int64) *mtproto.InputPhoneCall {
	return mtproto.MakeTLInputPhoneCall(&mtproto.InputPhoneCall{Id: id, AccessHash: accessHash}).To_InputPhoneCall()
}

func TestPhoneRequestCallAuthed(t *testing.T) {
	dsn := isolatedAuditDSN(t)
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })
	previousRelay := domain.Relay
	domain.SetRelay("turn.example.test", 3478)
	domain.SetRelayCredentials("fixture-user", "fixture-password")
	t.Cleanup(func() { domain.Relay = previousRelay })

	adminID := time.Now().UnixNano()
	participantID := adminID + 1
	adminHash, participantHash := int64(810001), int64(810002)
	users := map[int64]*mtproto.UserData{
		adminID: voipUser(adminID, adminHash), participantID: voipUser(participantID, participantHash),
	}
	syncer := &voipSyncClient{}
	ga := []byte("call-ga")
	digest := sha256.Sum256(ga)
	request := &mtproto.TLPhoneRequestCall{
		UserId:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: participantID, AccessHash: participantHash}).To_InputUser(),
		RandomId: int32(time.Now().UnixNano()),
		GAHash:   digest[:],
		Protocol: &mtproto.PhoneCallProtocol{MinLayer: 65, MaxLayer: 92},
	}
	requested, err := voipCore(adminID, users, syncer).PhoneRequestCall(request)
	if err != nil {
		t.Fatal(err)
	}
	if requested == nil || requested.GetPhoneCall().GetId() == 0 || requested.GetPhoneCall().GetAccessHash() == 0 {
		t.Fatalf("requested call: %+v", requested)
	}
	callID, accessHash := requested.GetPhoneCall().GetId(), requested.GetPhoneCall().GetAccessHash()
	t.Cleanup(func() {
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_call_artifact WHERE call_id=$1`, callID)
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_call WHERE id=$1`, callID)
	})
	if len(syncer.updates) != 1 || syncer.updates[0].GetUserId() != participantID {
		t.Fatalf("request update: %+v", syncer.updates)
	}
	peer := voipPeer(callID, accessHash)
	participant := voipCore(participantID, users, syncer)
	if _, err = participant.PhoneReceivedCall(&mtproto.TLPhoneReceivedCall{Peer: peer}); err != nil {
		t.Fatal(err)
	}
	accepted, err := participant.PhoneAcceptCall(&mtproto.TLPhoneAcceptCall{Peer: peer, GB: []byte("call-gb"), Protocol: request.Protocol})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.GetPhoneCall().GetId() != callID || accepted.GetPhoneCall().GetPredicateName() != "phoneCallAccepted" || string(accepted.GetPhoneCall().GetGB()) != "call-gb" {
		t.Fatalf("accepted call: %+v", accepted)
	}
	admin := voipCore(adminID, users, syncer)
	confirmed, err := admin.PhoneConfirmCall(&mtproto.TLPhoneConfirmCall{
		Peer: peer, GA: ga, KeyFingerprint: 17, Protocol: request.Protocol,
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.GetPhoneCall().GetPredicateName() != "phoneCall" || string(confirmed.GetPhoneCall().GetGAOrB()) != "call-gb" || confirmed.GetPhoneCall().GetKeyFingerprint() != 17 || len(confirmed.GetPhoneCall().GetConnections()) == 0 {
		t.Fatalf("confirmed call: %+v", confirmed)
	}
	pushed := syncer.updates[2].GetUpdates().GetUpdates()[0].GetPhoneCall()
	if pushed == nil || pushed.GetPredicateName() != "phoneCall" || string(pushed.GetGAOrB()) != string(ga) || pushed.GetKeyFingerprint() != 17 {
		t.Fatalf("confirmed update: %+v", syncer.updates[2])
	}
	reason := mtproto.MakeTLPhoneCallDiscardReasonHangup(&mtproto.PhoneCallDiscardReason{}).To_PhoneCallDiscardReason()
	if _, err = participant.PhoneDiscardCall(&mtproto.TLPhoneDiscardCall{Peer: peer, Duration: 12, Reason: reason}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := domain.LoadCall(callID)
	if err != nil || !ok {
		t.Fatalf("load call: %+v ok=%v err=%v", got, ok, err)
	}
	if got.AdminID != adminID || got.ParticipantID != participantID || got.State != "discarded" || string(got.GA) != string(ga) || string(got.GB) != "call-gb" || got.Duration != 12 {
		t.Fatalf("persisted call: %+v", got)
	}
	if len(syncer.updates) != 4 {
		t.Fatalf("updates=%d, want request/accept/confirm/discard", len(syncer.updates))
	}
	if err = domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("reopen domain: %v", err)
	}
	afterRestart, ok, err := domain.LoadCall(callID)
	if err != nil || !ok {
		t.Fatalf("load after restart: %+v ok=%v err=%v", afterRestart, ok, err)
	}
	if afterRestart.State != "discarded" || string(afterRestart.GA) != string(ga) || string(afterRestart.GB) != "call-gb" || afterRestart.Duration != 12 || afterRestart.DiscardedBy != participantID {
		t.Fatalf("call after restart: %+v", afterRestart)
	}
}

func TestPhoneCallRejectsWrongPeerAndState(t *testing.T) {
	cleanupDB, err := persist.OpenPostgresDB(isolatedAuditDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })
	previousRelay := domain.Relay
	domain.SetRelay("turn.example.test", 3478)
	domain.SetRelayCredentials("fixture-user", "fixture-password")
	t.Cleanup(func() { domain.Relay = previousRelay })
	adminID := time.Now().UnixNano()
	participantID := adminID + 1
	participantHash := int64(810012)
	users := map[int64]*mtproto.UserData{
		adminID: voipUser(adminID, 810011), participantID: voipUser(participantID, participantHash),
	}
	syncer := &voipSyncClient{}
	request := &mtproto.TLPhoneRequestCall{
		UserId:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: participantID, AccessHash: participantHash}).To_InputUser(),
		RandomId: 10011,
		GAHash:   []byte("hash"),
	}
	requested, err := voipCore(adminID, users, syncer).PhoneRequestCall(request)
	if err != nil {
		t.Fatal(err)
	}
	callID, accessHash := requested.GetPhoneCall().GetId(), requested.GetPhoneCall().GetAccessHash()
	t.Cleanup(func() {
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_call_artifact WHERE call_id=$1`, callID)
		_, _ = cleanupDB.Exec(`DELETE FROM apifull_call WHERE id=$1`, callID)
	})
	peer := voipPeer(callID, accessHash)
	if _, err = voipCore(adminID, users, syncer).PhoneAcceptCall(&mtproto.TLPhoneAcceptCall{Peer: peer, GB: []byte("gb")}); !errors.Is(err, mtproto.ErrCallPeerInvalid) {
		t.Fatalf("admin accept err=%v, want CALL_PEER_INVALID", err)
	}
	if _, err = voipCore(participantID, users, syncer).PhoneAcceptCall(&mtproto.TLPhoneAcceptCall{Peer: voipPeer(callID, accessHash+1), GB: []byte("gb")}); !errors.Is(err, mtproto.ErrCallPeerInvalid) {
		t.Fatalf("wrong hash err=%v, want CALL_PEER_INVALID", err)
	}
	if _, err = voipCore(participantID, users, syncer).PhoneConfirmCall(&mtproto.TLPhoneConfirmCall{Peer: peer, GA: []byte("ga")}); !errors.Is(err, mtproto.ErrCallAlreadyDeclined) {
		t.Fatalf("confirm before accept err=%v, want CALL_ALREADY_DECLINED", err)
	}
}
