package core

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/svc"
)

func TestHelpGetPremiumPromoUsesPostgresCatalogue(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := sharedpersist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = sharedpersist.DeletePremiumPromo(context.Background())
		_ = sharedpersist.ClosePostgres()
	})
	promo := mtproto.MakeTLHelpPremiumPromo(&mtproto.Help_PremiumPromo{
		Currency: "USD", MonthlyAmount: 499,
		PeriodOptions: []*mtproto.PremiumSubscriptionOption{{Months: 1, Currency: "USD", Amount: 499}},
	}).To_Help_PremiumPromo()
	if err := sharedpersist.StorePremiumPromo(context.Background(), promo); err != nil {
		t.Fatal(err)
	}
	c := New(context.Background(), &svc.ServiceContext{Config: config.Config{}})
	c.MD = &metadata.RpcMetadata{UserId: 42}
	got, err := c.HelpGetPremiumPromo(&mtproto.TLHelpGetPremiumPromo{})
	if err != nil || got == nil || got.GetCurrency() != "USD" || got.GetMonthlyAmount() != 499 {
		t.Fatalf("premium promo = (%v, %v)", got, err)
	}
}
