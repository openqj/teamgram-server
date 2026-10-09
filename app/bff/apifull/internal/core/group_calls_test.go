package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type groupCallUserClient struct {
	user_client.UserClient
	users map[int64]*mtproto.UserData
}

func (c *groupCallUserClient) UserGetImmutableUserV2(_ context.Context, in *userpb.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	if in == nil || c.users[in.GetId()] == nil {
		return nil, nil
	}
	return &mtproto.ImmutableUser{User: c.users[in.GetId()]}, nil
}

func groupCallMediaFixture(t *testing.T) config.Config {
	t.Helper()
	const signingKey = "group-call-fixture-signing-key-0000000000"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || r.Header.Get("Authorization") != "Bearer fixture-media" ||
			!verifyGroupCallMediaSignature(signingKey, r.Header.Get("X-Teamgram-Group-Call-Request-Signature"), body) {
			t.Error("media request must have valid authorization and signature")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var request groupCallMediaRequest
		if err = json.Unmarshal(body, &request); err != nil || request.RequestID == "" {
			t.Error("media request must contain valid JSON and request ID")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Operation != "join_group_call" && request.Operation != "leave_group_call" && request.Operation != "discard_group_call" {
			t.Errorf("unexpected media operation: %q", request.Operation)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		response := groupCallMediaResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			UserID: request.UserID, CallID: request.CallID, ChannelID: request.ChannelID,
			MediaSource: int32(request.UserID%1_000_000) + 1,
		}
		body, err = json.Marshal(response)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Teamgram-Group-Call-Signature", groupCallMediaSignature(signingKey, body))
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return config.Config{
		GroupCallMediaEndpoint: server.URL, GroupCallMediaAPIKey: "fixture-media",
		GroupCallMediaSigningKey: signingKey,
	}
}

func TestPhoneCreateGroupCallAuthed(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	up, err := c.PhoneCreateGroupCall(nil)
	if err != nil {
		t.Fatal(err)
	}
	if up == nil || len(up.GetUpdates()) == 0 || up.GetUpdates()[0].GetCall_GROUPCALL() == nil {
		t.Fatal("missing group call")
	}
	id := up.GetUpdates()[0].GetCall_GROUPCALL().GetId()
	if id == 0 {
		t.Fatal("group call id")
	}
	got, err := c.PhoneGetGroupCall(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetCall().GetId() != id {
		t.Fatalf("get id %d want %d", got.GetCall().GetId(), id)
	}
}

func TestGroupCallPostgres(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 4}}
	up, err := c.PhoneCreateGroupCall(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := up.GetUpdates()[0].GetCall_GROUPCALL().GetId()
	parts, ok, err := domain.LoadGroupCall(id)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !strings.Contains(parts, "4") {
		t.Fatalf("ok=%v participants=%q", ok, parts)
	}
}

func TestGroupCallChannelMembershipAndAccessHash(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner := int64(1_900_000_000 + time.Now().UnixNano()%100_000)
	member := owner + 1
	outsider := owner + 2
	if err := domain.SaveChannel(domain.Channel{
		ID:         channelID,
		AccessHash: channelID,
		Creator:    owner,
		Title:      "group-call-access-test",
		Megagroup:  true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, channelID); err != nil {
			t.Errorf("delete test channel: %v", err)
		}
	})
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}

	peer := &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerChannel,
		ChannelId:     channelID,
		AccessHash:    channelID,
	}
	mediaConfig := groupCallMediaFixture(t)
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}, svcCtx: &svc.ServiceContext{Config: mediaConfig}}
	created, err := ownerCore.PhoneCreateGroupCall(&mtproto.TLPhoneCreateGroupCall{Peer: peer})
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create group call: result=%+v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	if call == nil || call.GetId() == 0 || call.GetAccessHash() == 0 {
		t.Fatalf("missing group call: %+v", call)
	}
	stored, ok, err := domain.LoadGroupCallRecord(call.GetId())
	if err != nil || !ok || stored.Creator != owner || stored.ChannelID != channelID || stored.AccessHash != call.GetAccessHash() {
		t.Fatalf("stored group call = %+v, ok=%v, err=%v", stored, ok, err)
	}
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}
	memberCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: member}, svcCtx: &svc.ServiceContext{Config: mediaConfig}}
	if _, err = memberCore.PhoneGetGroupCall(&mtproto.TLPhoneGetGroupCall{Call: input, Limit: 10}); err != nil {
		t.Fatalf("member get group call: %v", err)
	}
	if _, err = ownerCore.PhoneJoinGroupCall(&mtproto.TLPhoneJoinGroupCall{Call: input,
		JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Params: &mtproto.DataJSON{Data: `{}`}}); err != nil {
		t.Fatalf("owner join group call: %v", err)
	}
	if _, err = memberCore.PhoneJoinGroupCall(&mtproto.TLPhoneJoinGroupCall{Call: input, Muted: true,
		JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Params: &mtproto.DataJSON{Data: `{}`}}); err != nil {
		t.Fatalf("member join group call: %v", err)
	}
	participants, err := ownerCore.PhoneGetGroupParticipants(&mtproto.TLPhoneGetGroupParticipants{Call: input, Limit: 10})
	if err != nil || participants.GetCount() != 2 {
		t.Fatalf("participants after join: result=%+v err=%v", participants, err)
	}
	checked, err := memberCore.PhoneCheckGroupCall(&mtproto.TLPhoneCheckGroupCall{
		Call:    input,
		Sources: []int32{int32(owner%1_000_000) + 1, int32(member%1_000_000) + 1},
	})
	if err != nil || len(checked.GetDatas()) != 2 {
		t.Fatalf("check group call: result=%+v err=%v", checked, err)
	}

	wrongHash := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash() + 1}
	if _, err = ownerCore.PhoneGetGroupCall(&mtproto.TLPhoneGetGroupCall{Call: wrongHash}); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("wrong hash error = %v, want GROUPCALL_INVALID", err)
	}
	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsider}}
	if _, err = outsiderCore.PhoneGetGroupCall(&mtproto.TLPhoneGetGroupCall{Call: input}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider error = %v, want USER_NOT_PARTICIPANT", err)
	}

	if _, err = memberCore.PhoneLeaveGroupCall(&mtproto.TLPhoneLeaveGroupCall{Call: input}); err != nil {
		t.Fatalf("member leave group call: %v", err)
	}
	participants, err = ownerCore.PhoneGetGroupParticipants(&mtproto.TLPhoneGetGroupParticipants{Call: input, Limit: 10})
	if err != nil || participants.GetCount() != 1 {
		t.Fatalf("participants after leave: result=%+v err=%v", participants, err)
	}
}

func TestGroupCallMutationsPersistAndAuthorize(t *testing.T) {
	base := int64(1_400_000_000 + time.Now().UnixNano()%100_000_000)
	owner, invitee, outsider, deleted := base, base+1, base+2, base+3
	inviteeHash := invitee + 100
	userClient := &groupCallUserClient{users: map[int64]*mtproto.UserData{
		owner:    {Id: owner, AccessHash: owner + 100},
		invitee:  {Id: invitee, AccessHash: inviteeHash},
		outsider: {Id: outsider, AccessHash: outsider + 100},
		deleted:  {Id: deleted, AccessHash: deleted + 100, Deleted: true},
	}}
	mediaConfig := groupCallMediaFixture(t)
	coreFor := func(uid int64) *ApiFullCore {
		return &ApiFullCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Config: mediaConfig, Dao: &apifullDao.Dao{UserClient: userClient}},
			MD:     &metadata.RpcMetadata{UserId: uid},
		}
	}
	ownerCore := coreFor(owner)
	scheduleDate := int32(time.Now().Add(time.Hour).Unix())
	created, err := ownerCore.PhoneCreateGroupCall(&mtproto.TLPhoneCreateGroupCall{
		RtmpStream:   true,
		Title:        wrapperspb.String("created title"),
		ScheduleDate: wrapperspb.Int32(scheduleDate),
	})
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create group call: result=%+v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	if call == nil {
		t.Fatal("create group call returned no call")
	}
	t.Cleanup(func() { _, _ = domain.DeleteGroupCall(call.GetId()) })
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}

	if _, err = ownerCore.PhoneEditGroupCallTitle(&mtproto.TLPhoneEditGroupCallTitle{Call: input, Title: "edited title"}); err != nil {
		t.Fatalf("edit title: %v", err)
	}
	stored, ok, err := domain.LoadGroupCallRecord(call.GetId())
	if err != nil || !ok || stored.Title != "edited title" || !stored.RtmpStream || stored.ScheduleDate == nil || *stored.ScheduleDate != scheduleDate {
		t.Fatalf("stored title = %+v, ok=%v, err=%v", stored, ok, err)
	}
	got, err := ownerCore.PhoneGetGroupCall(&mtproto.TLPhoneGetGroupCall{Call: input, Limit: 10})
	if err != nil || got.GetCall().GetTitle().GetValue() != "edited title" || !got.GetCall().GetRtmpStream() || got.GetCall().GetScheduleDate().GetValue() != scheduleDate {
		t.Fatalf("title readback: result=%+v err=%v", got, err)
	}

	if _, err = ownerCore.PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input}); !errors.Is(err, mtproto.ErrUsersTooFew) {
		t.Fatalf("empty invitation: %v", err)
	}
	if _, err = ownerCore.PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input, Users: []*mtproto.InputUser{{}}}); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("malformed invitation: %v", err)
	}
	missing := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: deleted + 1, AccessHash: deleted + 101}).To_InputUser()
	if _, err = ownerCore.PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input, Users: []*mtproto.InputUser{missing}}); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("unknown invitee: %v", err)
	}
	deactivated := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: deleted, AccessHash: deleted + 100}).To_InputUser()
	if _, err = ownerCore.PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input, Users: []*mtproto.InputUser{deactivated}}); !errors.Is(err, mtproto.ErrInputUserDeactivated) {
		t.Fatalf("deactivated invitee: %v", err)
	}

	badInvitee := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: invitee, AccessHash: inviteeHash + 1}).To_InputUser()
	if _, err = ownerCore.PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input, Users: []*mtproto.InputUser{badInvitee}}); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("wrong invitee hash: %v", err)
	}
	stored, ok, err = domain.LoadGroupCallRecord(call.GetId())
	if err != nil || !ok || len(decodeGroupUserIDs(stored.Participants)) != 1 {
		t.Fatalf("wrong invitee hash changed participants: %+v, ok=%v, err=%v", stored, ok, err)
	}

	validInvitee := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: invitee, AccessHash: inviteeHash}).To_InputUser()
	if _, err = ownerCore.PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input, Users: []*mtproto.InputUser{validInvitee}}); err != nil {
		t.Fatalf("invite user: %v", err)
	}
	inviteeCore := coreFor(invitee)
	if _, err = inviteeCore.PhoneGetGroupCall(&mtproto.TLPhoneGetGroupCall{Call: input, Limit: 10}); err != nil {
		t.Fatalf("invited user get group call: %v", err)
	}
	if _, err = inviteeCore.PhoneJoinGroupCall(&mtproto.TLPhoneJoinGroupCall{
		Call:         input,
		JoinAs:       mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Muted:        true,
		VideoStopped: true,
		Params:       &mtproto.DataJSON{Data: "{\"audio_source\":42}"},
	}); err != nil {
		t.Fatalf("invited user join group call: %v", err)
	}
	participantState, participantOK, err := domain.LoadGroupCallParticipant(call.GetId(), invitee)
	if err != nil || !participantOK || !participantState.VideoStopped || participantState.JoinParams != "{\"audio_source\":42}" || participantState.MediaSource != int32(invitee%1_000_000)+1 {
		t.Fatalf("joined participant state = %+v, ok=%v, err=%v", participantState, participantOK, err)
	}
	participants, err := ownerCore.PhoneGetGroupParticipants(&mtproto.TLPhoneGetGroupParticipants{Call: input, Limit: 10})
	if err != nil || participants.GetCount() != 2 {
		t.Fatalf("participants after invite: result=%+v err=%v", participants, err)
	}

	if _, err = inviteeCore.PhoneEditGroupCallTitle(&mtproto.TLPhoneEditGroupCallTitle{Call: input, Title: "blocked"}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-creator title edit: %v", err)
	}
	if _, err = coreFor(outsider).PhoneInviteToGroupCall(&mtproto.TLPhoneInviteToGroupCall{Call: input, Users: []*mtproto.InputUser{validInvitee}}); !errors.Is(err, mtproto.ErrGroupcallForbidden) {
		t.Fatalf("non-member invitation: %v", err)
	}
	wrongCallHash := &mtproto.InputGroupCall{Id: input.GetId(), AccessHash: input.GetAccessHash() + 1}
	if _, err = ownerCore.PhoneEditGroupCallTitle(&mtproto.TLPhoneEditGroupCallTitle{Call: wrongCallHash, Title: "blocked"}); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("wrong group call hash: %v", err)
	}
	if err = domain.SaveGroupCallSubscription(call.GetId(), owner, true); err != nil {
		t.Fatalf("save group call subscription: %v", err)
	}
	if err = domain.SaveGroupCallSendAs(call.GetId(), owner, `{"predicate_name":"inputPeerSelf"}`); err != nil {
		t.Fatalf("save group call send-as: %v", err)
	}
	message, inserted, err := domain.CreateGroupCallMessage(call.GetId(), owner, time.Now().UnixNano(), `{"text":"cleanup"}`, "", nil)
	if err != nil || !inserted {
		t.Fatalf("save group call message: message=%+v inserted=%v err=%v", message, inserted, err)
	}

	if _, err = ownerCore.PhoneDiscardGroupCall(&mtproto.TLPhoneDiscardGroupCall{Call: input}); err != nil {
		t.Fatalf("discard group call: %v", err)
	}
	if _, err = ownerCore.PhoneGetGroupCall(&mtproto.TLPhoneGetGroupCall{Call: input}); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("get after discard: %v", err)
	}
	if _, err = inviteeCore.PhoneJoinGroupCall(&mtproto.TLPhoneJoinGroupCall{Call: input,
		JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Params: &mtproto.DataJSON{Data: `{}`}}); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("join after discard: %v", err)
	}
	if _, ok, err := domain.LoadGroupCallParticipant(call.GetId(), invitee); err != nil || ok {
		t.Fatalf("participant after discard: ok=%v err=%v", ok, err)
	}
	if subscribed, err := domain.LoadGroupCallSubscription(call.GetId(), owner); err != nil || subscribed {
		t.Fatalf("subscription after discard: subscribed=%v err=%v", subscribed, err)
	}
	if _, ok, err := domain.LoadGroupCallSendAs(call.GetId(), owner); err != nil || ok {
		t.Fatalf("send-as after discard: ok=%v err=%v", ok, err)
	}
	if _, ok, err := domain.LoadGroupCallMessage(call.GetId(), message.RandomID); err != nil || ok {
		t.Fatalf("message after discard: ok=%v err=%v", ok, err)
	}
}

func TestPhoneSaveDefaultGroupCallJoinAsReadbackIsolation(t *testing.T) {
	uid := time.Now().UnixNano()
	otherUserID := uid + 4
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	peer := &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerChannel,
		ChannelId:     uid + 1,
		AccessHash:    uid + 1,
	}
	joinAs := &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerChannel,
		ChannelId:     uid + 2,
		AccessHash:    uid + 2,
	}
	for _, channelID := range []int64{uid + 1, uid + 2, uid + 3} {
		if err := domain.SaveChannel(domain.Channel{
			ID: channelID, AccessHash: channelID, Creator: uid, Title: fmt.Sprintf("join-as-%d", channelID), Megagroup: true,
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := domain.DeleteChannel(uid, channelID); err != nil {
				t.Errorf("delete join-as test channel: %v", err)
			}
		})
	}
	if err := domain.JoinChannel(peer.GetChannelId(), otherUserID); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("gcall:%d:PhoneSaveDefaultGroupCallJoinAs", uid)
	t.Cleanup(func() {
		raw, err := persist.Default.Get(key)
		if err != nil {
			t.Errorf("read saved preference for cleanup: %v", err)
			return
		}
		if raw != "" {
			if _, err := persist.CompareAndDelete(key, raw); err != nil {
				t.Errorf("clear saved preference: %v", err)
			}
		}
	})
	invalid := []*mtproto.TLPhoneSaveDefaultGroupCallJoinAs{
		nil,
		{},
		{Peer: peer, JoinAs: nil},
		{Peer: &mtproto.InputPeer{}, JoinAs: joinAs},
		{Peer: peer, JoinAs: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: uid + 5, ChannelId: uid + 6}},
	}
	for i, request := range invalid {
		if result, err := c.PhoneSaveDefaultGroupCallJoinAs(request); result != nil || err != mtproto.ErrPeerIdInvalid {
			t.Fatalf("invalid request %d: result=%v err=%v", i, result, err)
		}
		if raw, err := persist.Default.Get(key); err != nil || raw != "" {
			t.Fatalf("invalid request %d persisted state: raw=%q err=%v", i, raw, err)
		}
	}

	if _, err := c.PhoneSaveDefaultGroupCallJoinAs(&mtproto.TLPhoneSaveDefaultGroupCallJoinAs{
		Peer: peer, JoinAs: joinAs,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.PhoneGetGroupCallJoinAs(&mtproto.TLPhoneGetGroupCallJoinAs{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	if !hasPeerChannel(got.GetPeers(), joinAs.GetChannelId()) {
		t.Fatalf("saved join-as peer missing from readback: %+v", got.GetPeers())
	}

	otherPeer, err := c.PhoneGetGroupCallJoinAs(&mtproto.TLPhoneGetGroupCallJoinAs{
		Peer: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChannel, ChannelId: uid + 3, AccessHash: uid + 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasPeerChannel(otherPeer.GetPeers(), joinAs.GetChannelId()) {
		t.Fatalf("saved join-as leaked to another peer: %+v", otherPeer.GetPeers())
	}

	otherUser := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: otherUserID}}
	otherUserPeers, err := otherUser.PhoneGetGroupCallJoinAs(&mtproto.TLPhoneGetGroupCallJoinAs{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	if hasPeerChannel(otherUserPeers.GetPeers(), joinAs.GetChannelId()) {
		t.Fatalf("saved join-as leaked to another user: %+v", otherUserPeers.GetPeers())
	}
}

func TestGroupCallMediaAndStarsFailClosedWithoutProviders(t *testing.T) {
	uid := time.Now().UnixNano()
	channelID := uid + 1
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: uid, Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(uid, channelID) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: channelID}).To_InputPeer()
	created, err := c.PhoneCreateGroupCall(&mtproto.TLPhoneCreateGroupCall{Peer: peer, RtmpStream: true})
	if err != nil {
		t.Fatal(err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}
	checks := []struct {
		name string
		call func() error
	}{
		{"stream channels", func() error {
			_, err := c.PhoneGetGroupCallStreamChannels(&mtproto.TLPhoneGetGroupCallStreamChannels{Call: input})
			return err
		}},
		{"rtmp URL", func() error {
			_, err := c.PhoneGetGroupCallStreamRtmpUrl(&mtproto.TLPhoneGetGroupCallStreamRtmpUrl{Peer: peer, Revoke: mtproto.BoolFalse})
			return err
		}},
		{"stars", func() error { _, err := c.PhoneGetGroupCallStars(nil); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("error = %v, want METHOD_NOT_IMPL", err)
			}
		})
	}
	if _, err = c.PhoneGetGroupCallStreamChannels(nil); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("nil stream request: %v, want GROUPCALL_INVALID", err)
	}
	if _, err = c.PhoneGetGroupCallStreamRtmpUrl(nil); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("nil RTMP request: %v, want PEER_ID_INVALID", err)
	}
}

func TestGroupCallRecordingStatePersistsWithoutRecorder(t *testing.T) {
	uid := int64(1_700_000_000 + time.Now().UnixNano()%100_000_000)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	created, err := c.PhoneCreateGroupCall(nil)
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create group call: result=%+v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	t.Cleanup(func() { _, _ = domain.DeleteGroupCall(call.GetId()) })
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}
	result, err := c.PhoneToggleGroupCallRecord(&mtproto.TLPhoneToggleGroupCallRecord{
		Call: input, Start: true, Video: true,
		Title: wrapperspb.String("weekly sync"), VideoPortrait: mtproto.BoolTrue,
	})
	if err != nil || result == nil {
		t.Fatalf("toggle recording: result=%v err=%v", result, err)
	}
	settings, err := domain.LoadGroupCallSettings(call.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if !settings.RecordActive || !settings.RecordVideo || settings.RecordTitle != "weekly sync" || !settings.RecordVideoPortrait {
		t.Fatalf("recording settings = %+v", settings)
	}
	if _, err = c.PhoneToggleGroupCallRecord(&mtproto.TLPhoneToggleGroupCallRecord{Call: input, Start: true, Video: true, Title: wrapperspb.String("weekly sync"), VideoPortrait: mtproto.BoolTrue}); !errors.Is(err, mtproto.ErrGroupcallNotModified) {
		t.Fatalf("unchanged recording update: %v", err)
	}
	if _, err = c.PhoneToggleGroupCallRecord(&mtproto.TLPhoneToggleGroupCallRecord{Call: input, Start: false}); err != nil {
		t.Fatalf("stop recording: %v", err)
	}
	settings, err = domain.LoadGroupCallSettings(call.GetId())
	if err != nil || settings.RecordActive || settings.RecordVideo {
		t.Fatalf("stopped recording settings = %+v, err=%v", settings, err)
	}
}

func TestGroupCallControlMutationsPersistAndAuthorize(t *testing.T) {
	uid := int64(1_800_000_000 + time.Now().UnixNano()%100_000_000)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}, svcCtx: &svc.ServiceContext{Config: groupCallMediaFixture(t)}}
	scheduleDate := int32(time.Now().Add(time.Hour).Unix())
	created, err := c.PhoneCreateGroupCall(&mtproto.TLPhoneCreateGroupCall{ScheduleDate: wrapperspb.Int32(scheduleDate)})
	if err != nil || created == nil || len(created.GetUpdates()) == 0 {
		t.Fatalf("create group call: result=%+v err=%v", created, err)
	}
	call := created.GetUpdates()[0].GetCall_GROUPCALL()
	if call == nil {
		t.Fatal("missing group call")
	}
	t.Cleanup(func() { _, _ = domain.DeleteGroupCall(call.GetId()) })
	input := &mtproto.InputGroupCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}

	if _, err = c.PhoneToggleGroupCallSettings(&mtproto.TLPhoneToggleGroupCallSettings{
		Call: input, JoinMuted: mtproto.BoolTrue, MessagesEnabled: mtproto.BoolFalse,
	}); err != nil {
		t.Fatalf("toggle settings: %v", err)
	}
	settings, err := domain.LoadGroupCallSettings(call.GetId())
	if err != nil || !settings.JoinMuted || settings.MessagesEnabled {
		t.Fatalf("settings = %+v, err=%v", settings, err)
	}
	if _, err = c.PhoneToggleGroupCallSettings(&mtproto.TLPhoneToggleGroupCallSettings{Call: input}); !errors.Is(err, mtproto.ErrGroupcallNotModified) {
		t.Fatalf("empty settings update: %v", err)
	}

	if _, err = c.PhoneJoinGroupCall(&mtproto.TLPhoneJoinGroupCall{Call: input,
		JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), Params: &mtproto.DataJSON{Data: `{}`}}); err != nil {
		t.Fatalf("join group call before participant edit: %v", err)
	}
	volume := wrapperspb.Int32(75)
	if _, err = c.PhoneEditGroupCallParticipant(&mtproto.TLPhoneEditGroupCallParticipant{
		Call: input, Participant: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerSelf}, Volume: volume,
	}); err != nil {
		t.Fatalf("edit participant: %v", err)
	}
	state, ok, err := domain.LoadGroupCallParticipant(call.GetId(), uid)
	if err != nil || !ok || state.Volume == nil || *state.Volume != 75 {
		t.Fatalf("participant state = %+v, ok=%v, err=%v", state, ok, err)
	}
	if _, err = c.PhoneEditGroupCallParticipant(&mtproto.TLPhoneEditGroupCallParticipant{Call: input, Participant: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerSelf}}); !errors.Is(err, mtproto.ErrGroupcallNotModified) {
		t.Fatalf("empty participant update: %v", err)
	}

	if _, err = c.PhoneToggleGroupCallStartSubscription(&mtproto.TLPhoneToggleGroupCallStartSubscription{Call: input, Subscribed: mtproto.BoolTrue}); err != nil {
		t.Fatalf("toggle subscription: %v", err)
	}
	if subscribed, err := domain.LoadGroupCallSubscription(call.GetId(), uid); err != nil || !subscribed {
		t.Fatalf("subscription = %v, err=%v", subscribed, err)
	}

	if _, err = c.PhoneJoinGroupCallPresentation(&mtproto.TLPhoneJoinGroupCallPresentation{
		Call: input, Params: &mtproto.DataJSON{Data: `{"endpoint":"provider-owned"}`},
	}); err != nil {
		t.Fatalf("join presentation: %v", err)
	}
	state, ok, err = domain.LoadGroupCallParticipant(call.GetId(), uid)
	if err != nil || !ok || !state.PresentationActive {
		t.Fatalf("presentation state after join = %+v, ok=%v, err=%v", state, ok, err)
	}
	if _, err = c.PhoneLeaveGroupCallPresentation(&mtproto.TLPhoneLeaveGroupCallPresentation{Call: input}); err != nil {
		t.Fatalf("leave presentation: %v", err)
	}
	state, ok, err = domain.LoadGroupCallParticipant(call.GetId(), uid)
	if err != nil || !ok || state.PresentationActive || state.PresentationPaused || state.PresentationParams != "" {
		t.Fatalf("presentation state after leave = %+v, ok=%v, err=%v", state, ok, err)
	}

	if _, err = c.PhoneStartScheduledGroupCall(&mtproto.TLPhoneStartScheduledGroupCall{Call: input}); err != nil {
		t.Fatalf("start scheduled group call: %v", err)
	}
	if _, err = c.PhoneStartScheduledGroupCall(&mtproto.TLPhoneStartScheduledGroupCall{Call: input}); !errors.Is(err, mtproto.ErrGroupcallAlreadyStarted) {
		t.Fatalf("second scheduled start: %v", err)
	}
	exported, err := c.PhoneExportGroupCallInvite(&mtproto.TLPhoneExportGroupCallInvite{Call: input})
	if err != nil || exported == nil || exported.GetLink() == "" {
		t.Fatalf("export invite: result=%v err=%v", exported, err)
	}
	marker := "invite_hash="
	idx := strings.Index(exported.GetLink(), marker)
	if idx < 0 {
		t.Fatalf("exported invite link missing hash: %q", exported.GetLink())
	}
	token := exported.GetLink()[idx+len(marker):]
	valid, _, err := domain.CheckGroupCallInvite(call.GetId(), token)
	if err != nil || !valid {
		t.Fatalf("exported invite validation: valid=%v err=%v", valid, err)
	}
	if _, err = c.PhoneToggleGroupCallSettings(&mtproto.TLPhoneToggleGroupCallSettings{Call: input, ResetInviteHash: true}); err != nil {
		t.Fatalf("reset invite hash: %v", err)
	}
	valid, _, err = domain.CheckGroupCallInvite(call.GetId(), token)
	if err != nil || valid {
		t.Fatalf("revoked invite validation: valid=%v err=%v", valid, err)
	}
}

func hasPeerChannel(peers []*mtproto.Peer, channelID int64) bool {
	for _, peer := range peers {
		if peer.GetPredicateName() == mtproto.Predicate_peerChannel && peer.GetChannelId() == channelID {
			return true
		}
	}
	return false
}
