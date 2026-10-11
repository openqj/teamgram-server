package persist

import (
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPremiumPromoPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = DeletePremiumPromo(t.Context())
		_ = ClosePostgres()
		Default = previous
	})
	promo := mtproto.MakeTLHelpPremiumPromo(&mtproto.Help_PremiumPromo{
		Currency: "USD", MonthlyAmount: 499,
		PeriodOptions: []*mtproto.PremiumSubscriptionOption{{Months: 1, Currency: "USD", Amount: 499}},
	}).To_Help_PremiumPromo()
	if err := StorePremiumPromo(t.Context(), promo); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadPremiumPromo(t.Context())
	if err != nil || !found || got == nil || got.GetCurrency() != "USD" || len(got.GetPeriodOptions()) != 1 || got.GetPeriodOptions()[0].GetAmount() != 499 {
		t.Fatalf("premium promo = (%v, %v, %v)", got, found, err)
	}
}
