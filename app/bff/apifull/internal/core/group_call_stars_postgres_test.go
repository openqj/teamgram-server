package core

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestPhoneGetGroupCallStarsReadsAuthoritativePostgresLedger(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is required")
	}
	callID := time.Now().UnixNano()
	creatorID := callID + 1
	senderID := callID + 2
	if err := domain.SaveGroupCall(callID, callID+3, creatorID, 0, "stars", "[]"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = domain.DeleteGroupCallMessages(callID, nil)
		_, _ = domain.DeleteGroupCall(callID)
		// Test-only identities have no production rows outside the Stars ledger.
	})
	if _, err := domain.ApplyStars(senderID, 20, "group-call-core-seed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := domain.CreatePaidGroupCallMessage(callID, senderID, callID+4, `{"text":"paid"}`, "", 7); err != nil {
		t.Fatal(err)
	}

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: creatorID}}
	got, err := core.PhoneGetGroupCallStars(&mtproto.TLPhoneGetGroupCallStars{
		Call: &mtproto.InputGroupCall{Id: callID, AccessHash: callID + 3},
	})
	if err != nil || got == nil || got.GetTotalStars() != 7 || len(got.GetTopDonors()) != 1 {
		t.Fatalf("group call stars = (%#v, %v)", got, err)
	}
	if donor := got.GetTopDonors()[0]; donor.GetPeerId() == nil || donor.GetPeerId().GetUserId() != senderID || donor.GetStars() != 7 || donor.GetMy() {
		t.Fatalf("group call donor = %#v", got.GetTopDonors()[0])
	}
	if _, err = core.PhoneGetGroupCallStars(&mtproto.TLPhoneGetGroupCallStars{
		Call: &mtproto.InputGroupCall{Id: callID, AccessHash: callID + 4},
	}); !errors.Is(err, mtproto.ErrGroupCallInvalid) {
		t.Fatalf("wrong access hash error = %v", err)
	}
}
