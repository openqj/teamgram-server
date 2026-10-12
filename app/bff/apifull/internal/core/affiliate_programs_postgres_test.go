package core

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (c *miniBotAppsUserClient) UserGetMutableUsersV2(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	ids := append([]int64(nil), in.GetId()...)
	ids = append(ids, in.GetTo()...)
	users := make([]*mtproto.ImmutableUser, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		user := c.users[id]
		if user == nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		users = append(users, user)
	}
	return &mtproto.MutableUsers{
		PredicateName: mtproto.Predicate_mutableUsers,
		Constructor:   mtproto.TLConstructor_CRC32_mutableUsers,
		Users:         users,
	}, nil
}

func TestStarReferralPostgresAPIRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	const botID = int64(9824102)
	const accessHash = int64(9824103)
	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_star_ref_connection WHERE owner_user_id = $1`, botID)
		_, _ = db.Exec(`DELETE FROM apifull_star_ref_program WHERE bot_user_id = $1`, botID)
		_ = db.Close()
	})
	profile := miniBotProfile(botID, accessHash, true)
	profile.GetUser().Username = "audit_referral_bot"
	client := &miniBotAppsUserClient{users: map[int64]*mtproto.ImmutableUser{botID: profile}}
	// Remove any stale rows before asserting that an unprovisioned control
	// plane fails closed instead of returning a synthetic empty result.
	if _, err = db.Exec(`DELETE FROM apifull_star_ref_connection WHERE owner_user_id = $1`, botID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM apifull_star_ref_program WHERE bot_user_id = $1`, botID); err != nil {
		t.Fatal(err)
	}
	core := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: botID},
	}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	if _, err = core.PaymentsGetSuggestedStarRefBots(&mtproto.TLPaymentsGetSuggestedStarRefBots{Limit: 10}); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("unprovisioned suggested referral catalogue error = %v, want METHOD_NOT_IMPL", err)
	}
	if _, err = core.PaymentsGetConnectedStarRefBot(&mtproto.TLPaymentsGetConnectedStarRefBot{Peer: peer, Bot: miniBotInput(botID, accessHash)}); !errors.Is(err, mtproto.ErrPeerIdInvalid) {
		t.Fatalf("unprovisioned connected referral error = %v, want PEER_ID_INVALID", err)
	}
	program, err := core.BotsUpdateStarRefProgram(&mtproto.TLBotsUpdateStarRefProgram{
		Bot: miniBotInput(botID, accessHash), CommissionPermille: 125, DurationMonths: wrapperspb.Int32(3),
	})
	if err != nil || program == nil || program.GetBotId() != botID || program.GetCommissionPermille() != 125 {
		t.Fatalf("update referral program = %v, %v", program, err)
	}
	connected, err := core.PaymentsConnectStarRefBot(&mtproto.TLPaymentsConnectStarRefBot{Peer: peer, Bot: miniBotInput(botID, accessHash)})
	if err != nil || connected == nil || connected.GetCount() != 1 || len(connected.GetConnectedBots()) != 1 {
		t.Fatalf("connect referral bot = %v, %v", connected, err)
	}
	link := connected.GetConnectedBots()[0].GetUrl()
	if link == "" {
		t.Fatalf("connected referral link is empty: %v", connected)
	}
	read, err := core.PaymentsGetConnectedStarRefBot(&mtproto.TLPaymentsGetConnectedStarRefBot{Peer: peer, Bot: miniBotInput(botID, accessHash)})
	if err != nil || read.GetCount() != 1 || read.GetConnectedBots()[0].GetUrl() != link {
		t.Fatalf("read connected referral = %v, %v", read, err)
	}
	edited, err := core.PaymentsEditConnectedStarRefBot(&mtproto.TLPaymentsEditConnectedStarRefBot{Peer: peer, Link: link, Revoked: true})
	if err != nil || edited.GetCount() != 1 || !edited.GetConnectedBots()[0].GetRevoked() {
		t.Fatalf("edit connected referral = %v, %v", edited, err)
	}
}
