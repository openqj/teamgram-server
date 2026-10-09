package domain

import (
	"os"
	"testing"
	"time"
)

func requireSmsjobsDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open SMS jobs database: %v", err)
	}
	t.Cleanup(func() { _ = Close() })
}

func TestSmsjobsPostgresLifecycle(t *testing.T) {
	requireSmsjobsDB(t)
	userID := time.Now().UnixNano()
	jobID := "sms-test-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_sms_job WHERE job_id LIKE $1`, jobID+"%")
		_, _ = db.Exec(`DELETE FROM apifull_sms_job_member WHERE user_id=$1`, userID)
	})

	if err := SmsjobsJoin(userID); err != nil {
		t.Fatal(err)
	}
	if err := SmsjobsUpdateSettings(userID, true); err != nil {
		t.Fatal(err)
	}
	status, err := SmsjobsStatus(userID)
	if err != nil || !status.AllowInternational || status.TotalSent != 0 {
		t.Fatalf("joined status = %+v, %v", status, err)
	}
	if monthly, err := SmsjobsEligibility(userID); err != nil || monthly != 0 {
		t.Fatalf("eligibility = %d, %v", monthly, err)
	}

	job := SmsJobRecord{JobID: jobID, UserID: userID, PhoneNumber: "+15555550123", Text: "hello"}
	if err := UpsertSmsJob(job); err != nil {
		t.Fatal(err)
	}
	got, err := GetSmsJob(userID, jobID)
	if err != nil || got != job {
		t.Fatalf("get job = %+v, %v", got, err)
	}
	if err := FinishSmsJob(userID, jobID, ""); err != nil {
		t.Fatal(err)
	}
	if err := FinishSmsJob(userID, jobID, "duplicate"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSmsJob(userID, jobID); err != ErrSmsJobNotFound {
		t.Fatalf("finished get error = %v, want ErrSmsJobNotFound", err)
	}
	status, err = SmsjobsStatus(userID)
	if err != nil || status.TotalSent != 1 || status.RecentSent != 1 {
		t.Fatalf("finished status = %+v, %v", status, err)
	}

	failureID := jobID + "-failure"
	if err := UpsertSmsJob(SmsJobRecord{JobID: failureID, UserID: userID, PhoneNumber: job.PhoneNumber, Text: job.Text}); err != nil {
		t.Fatal(err)
	}
	if err := FinishSmsJob(userID, failureID, "provider rejected"); err != nil {
		t.Fatal(err)
	}
	if err := FinishSmsJob(userID, failureID, "provider rejected"); err != nil {
		t.Fatal(err)
	}
	var state, failure string
	if err := db.QueryRow(`SELECT state, error_text FROM apifull_sms_job WHERE job_id=$1`, failureID).Scan(&state, &failure); err != nil {
		t.Fatal(err)
	}
	if state != "finished" || failure != "provider rejected" {
		t.Fatalf("failure job = state %q error %q", state, failure)
	}
}
