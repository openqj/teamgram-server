package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestResolveCustomVerificationBotFollowsLayer229CallerFlags(t *testing.T) {
	const (
		ownerID       = int64(81021001)
		botID         = int64(81021002)
		botAccessHash = int64(81021003)
		outsiderID    = int64(81021004)
	)
	bot := miniBotProfile(botID, botAccessHash, true)
	bot.GetUser().GetBot().Creator = ownerID
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		ownerID:    miniBotProfile(ownerID, ownerID+1, false),
		botID:      bot,
		outsiderID: miniBotProfile(outsiderID, outsiderID+1, false),
	}}
	coreFor := func(callerID int64) *ApiFullCore {
		return &ApiFullCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
			MD:     &metadata.RpcMetadata{UserId: callerID},
		}
	}

	if got, err := coreFor(ownerID).resolveCustomVerificationBot(ownerID, miniBotInput(botID, botAccessHash)); err != nil || got != botID {
		t.Fatalf("owner bot input = (%d, %v), want bot %d", got, err, botID)
	}
	if _, err := coreFor(ownerID).resolveCustomVerificationBot(ownerID, nil); !errors.Is(err, mtproto.ErrBotInvalid) {
		t.Fatalf("human caller without bot input = %v, want BOT_INVALID", err)
	}
	if got, err := coreFor(botID).resolveCustomVerificationBot(botID, nil); err != nil || got != botID {
		t.Fatalf("bot caller with omitted bot input = (%d, %v), want bot %d", got, err, botID)
	}
	if _, err := coreFor(botID).resolveCustomVerificationBot(botID, miniBotInput(botID, botAccessHash)); !errors.Is(err, mtproto.ErrBotInvalid) {
		t.Fatalf("bot caller with explicit bot input = %v, want BOT_INVALID", err)
	}
	if _, err := coreFor(outsiderID).resolveCustomVerificationBot(outsiderID, miniBotInput(botID, botAccessHash)); !errors.Is(err, errBotVerifierForbidden) {
		t.Fatalf("non-owner bot input = %v, want BOT_VERIFIER_FORBIDDEN", err)
	}
}

func TestResolveCustomVerificationPeerChecksAuthoritativeUser(t *testing.T) {
	const (
		callerID   = int64(81022001)
		targetID   = int64(81022002)
		targetHash = int64(81022003)
	)
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		callerID: miniBotProfile(callerID, callerID+1, true),
		targetID: miniBotProfile(targetID, targetHash, false),
	}}
	core := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID, AccessHash: targetHash}).To_InputPeer()
	if gotType, gotID, err := core.resolveCustomVerificationPeer(callerID, peer); err != nil || gotType != mtproto.PEER_USER || gotID != targetID {
		t.Fatalf("valid user peer = (%d, %d, %v)", gotType, gotID, err)
	}
	wrongHash := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID, AccessHash: targetHash + 1}).To_InputPeer()
	if _, _, err := core.resolveCustomVerificationPeer(callerID, wrongHash); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("wrong user peer hash = %v, want USER_ID_INVALID", err)
	}
	unknown := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: targetID + 1, AccessHash: targetHash}).To_InputPeer()
	if _, _, err := core.resolveCustomVerificationPeer(callerID, unknown); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("unknown user peer = %v, want USER_ID_INVALID", err)
	}
	self := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	if gotType, gotID, err := core.resolveCustomVerificationPeer(callerID, self); err != nil || gotType != mtproto.PEER_SELF || gotID != callerID {
		t.Fatalf("self peer = (%d, %d, %v)", gotType, gotID, err)
	}
}

func TestBotsSetCustomVerificationUsesConfiguredDescriptionLimit(t *testing.T) {
	const botID = int64(81023001)
	accessHash := botID + 1
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	core := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: botID},
	}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	for _, size := range []int{70, 71} {
		request := &mtproto.TLBotsSetCustomVerification{
			Enabled:           true,
			Peer:              peer,
			CustomDescription: mtproto.MakeFlagsString(strings.Repeat("x", size)),
		}
		result, err := core.BotsSetCustomVerification(request)
		if size == 70 {
			if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("70-byte description = (%v, %v), want fail-closed METHOD_NOT_IMPL", result, err)
			}
		} else if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
			t.Fatalf("71-byte description = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
		}
	}
}

func TestBotsSetCustomVerificationOwnerUsesSignedProvider(t *testing.T) {
	const (
		ownerID       = int64(42)
		botID         = int64(81024001)
		botAccessHash = int64(81024002)
	)
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		ownerID: miniBotProfile(ownerID, ownerID+1, false),
		botID: func() *mtproto.ImmutableUser {
			profile := miniBotProfile(botID, botAccessHash, true)
			profile.GetUser().GetBot().Creator = ownerID
			return profile
		}(),
	}}
	core := newBusinessProviderGapCore(t, "bots.setCustomVerification", map[string]any{"value": true})
	core.svcCtx.Dao = &apifullDao.Dao{UserClient: client}
	core.MD = &metadata.RpcMetadata{UserId: ownerID}
	result, err := core.BotsSetCustomVerification(&mtproto.TLBotsSetCustomVerification{
		Enabled: true,
		Bot:     miniBotInput(botID, botAccessHash),
		Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	})
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("owner signed-provider verification = (%v, %v), want BoolTrue", result, err)
	}
}

func TestBotsSetCustomVerificationBotCallerOmitsBot(t *testing.T) {
	const botID = int64(42)
	accessHash := botID + 1
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID: miniBotProfile(botID, accessHash, true),
	}}
	core := newBusinessProviderGapCore(t, "bots.setCustomVerification", map[string]any{"value": true})
	core.svcCtx.Dao = &apifullDao.Dao{UserClient: client}
	result, err := core.BotsSetCustomVerification(&mtproto.TLBotsSetCustomVerification{
		Enabled: true,
		Peer:    mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	})
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("bot omitted-bot signed-provider verification = (%v, %v), want BoolTrue", result, err)
	}
}
