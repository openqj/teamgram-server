package persist

import (
	"os"
	"testing"
	"time"
)

func TestStarReferralPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() {
		if store, ok := Default.(*postgresStore); ok && store != nil && store.db != nil {
			_, _ = store.db.Exec(`DELETE FROM apifull_star_ref_connection WHERE owner_user_id = $1`, int64(9824001))
			_, _ = store.db.Exec(`DELETE FROM apifull_star_ref_program WHERE bot_user_id IN ($1, $2)`, int64(9824002), int64(9824003))
		}
		_ = ClosePostgres()
		Default = previous
	})

	now := time.Now().UTC()
	duration := int32(3)
	program, err := UpsertStarRefProgram(9824002, 125, &duration, now)
	if err != nil {
		t.Fatal(err)
	}
	if program.BotID != 9824002 || program.CommissionPermille != 125 || program.DurationMonths == nil || *program.DurationMonths != 3 || program.EndDate == nil {
		t.Fatalf("program = %+v", program)
	}
	active, found, err := LoadActiveStarRefProgram(9824002)
	if err != nil || !found || active.CommissionPermille != 125 {
		t.Fatalf("active program = %+v, found=%v, err=%v", active, found, err)
	}
	if _, err = UpsertStarRefProgram(9824003, 200, nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	page, count, next, err := ListActiveStarRefPrograms("", 1, true, false)
	if err != nil || count < 2 || len(page) != 1 || next == "" {
		t.Fatalf("first program page = %+v, count=%d, next=%q, err=%v", page, count, next, err)
	}
	secondPage, _, _, err := ListActiveStarRefPrograms(next, 1, true, false)
	if err != nil || len(secondPage) != 1 || secondPage[0].BotID == page[0].BotID {
		t.Fatalf("second program page = %+v, first=%+v, err=%v", secondPage, page, err)
	}
	link, err := NewStarReferralURL("audit_bot", 9824002)
	if err != nil {
		t.Fatal(err)
	}
	connected, err := ConnectStarRefBot(9824001, 2, 7001, 9824002, link, now)
	if err != nil {
		t.Fatal(err)
	}
	if connected.URL != link || connected.BotID != 9824002 || connected.Revoked {
		t.Fatalf("connected = %+v", connected)
	}
	connectedAgain, err := ConnectStarRefBot(9824001, 2, 7001, 9824002, link, now.Add(time.Minute))
	if err != nil || connectedAgain.URL != link {
		t.Fatalf("idempotent connect = %+v, err=%v", connectedAgain, err)
	}
	rows, count, err := ListStarRefConnections(9824001, 2, 7001, 0, "", 10)
	if err != nil || count != 1 || len(rows) != 1 {
		t.Fatalf("connections = %+v, count=%d, err=%v", rows, count, err)
	}
	updated, err := EditStarRefConnection(9824001, 2, 7001, link, true)
	if err != nil || !updated.Revoked {
		t.Fatalf("revoked connection = %+v, err=%v", updated, err)
	}
	if _, err = UpsertStarRefProgram(9824002, 0, nil, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, found, err = LoadActiveStarRefProgram(9824002); err != nil || found {
		t.Fatalf("inactive program = found=%v, err=%v", found, err)
	}
}
