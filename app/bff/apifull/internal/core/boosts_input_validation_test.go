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

func TestPremiumGetUserBoostsValidatesInputUser(t *testing.T) {
	const (
		callerID   = int64(81032001)
		targetID   = int64(81032002)
		targetHash = int64(91032002)
	)
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		targetID: miniBotProfile(targetID, targetHash, false),
	}}
	core := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}

	if got, err := core.boostInputUserID(callerID, miniBotInput(targetID, targetHash)); err != nil || got != targetID {
		t.Fatalf("valid input user = (%d, %v), want (%d, nil)", got, err, targetID)
	}
	if got, err := core.boostInputUserID(callerID, mtproto.MakeTLInputUserSelf(nil).To_InputUser()); err != nil || got != callerID {
		t.Fatalf("self input user = (%d, %v), want (%d, nil)", got, err, callerID)
	}

	tests := []struct {
		name  string
		input *mtproto.InputUser
		want  error
	}{
		{
			name:  "stale access hash",
			input: miniBotInput(targetID, targetHash+1),
			want:  mtproto.ErrUserIdInvalid,
		},
		{
			name:  "missing access hash",
			input: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID}).To_InputUser(),
			want:  mtproto.ErrUserIdInvalid,
		},
		{
			name:  "unknown user",
			input: miniBotInput(targetID+1, targetHash),
			want:  mtproto.ErrUserIdInvalid,
		},
		{
			name:  "unknown constructor",
			input: &mtproto.InputUser{UserId: targetID, AccessHash: targetHash},
			want:  mtproto.ErrInputConstructorInvalid,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := core.boostInputUserID(callerID, tc.input); got != 0 || !errors.Is(err, tc.want) {
				t.Fatalf("input user = (%d, %v), want (0, %v)", got, err, tc.want)
			}
		})
	}
}

func TestBoostInputChannelRejectsMissingAccessHash(t *testing.T) {
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 81033001}).To_InputChannel()
	if _, err := boostInputChannel(81033000, input); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("missing channel access hash = %v, want CHANNEL_INVALID", err)
	}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 81033001}).To_InputPeer()
	if _, err := boostPeerRefFromInput(81033000, domain.BoostScopePremium, peer); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("missing peer access hash = %v, want CHANNEL_INVALID", err)
	}
}

func TestBoostInputChannelRejectsStaleOrUnknownChannel(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	accessHash := channelID + 10
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: ownerID, Title: "boost-channel-validation", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(ownerID, channelID) })

	stale := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash + 1,
	}).To_InputChannel()
	if _, err := boostInputChannel(ownerID, stale); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("stale channel access hash = %v, want CHANNEL_INVALID", err)
	}
	stalePeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: accessHash + 1,
	}).To_InputPeer()
	if _, err := boostPeerRefFromInput(ownerID, domain.BoostScopePremium, stalePeer); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("stale peer access hash = %v, want CHANNEL_INVALID", err)
	}

	unknown := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID + 1, AccessHash: accessHash,
	}).To_InputChannel()
	if _, err := boostInputChannel(ownerID, unknown); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("unknown channel = %v, want CHANNEL_INVALID", err)
	}
	unknownPeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID + 1, AccessHash: accessHash,
	}).To_InputPeer()
	if _, err := boostPeerRefFromInput(ownerID, domain.BoostScopePremium, unknownPeer); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("unknown peer channel = %v, want CHANNEL_INVALID", err)
	}
}
