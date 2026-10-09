// Copyright 2026 Teamgram Authors
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package domain

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"
)

const smsJobRecentWindow = 30 * 24 * 60 * 60

var (
	ErrSmsJobIDInvalid = errors.New("sms job id is invalid")
	ErrSmsJobNotFound  = errors.New("sms job does not exist")
)

// SmsJobRecord is the durable assignment delivered to a Telegram client.
// UserID identifies the volunteer that may fetch and finish the assignment.
type SmsJobRecord struct {
	JobID       string
	UserID      int64
	PhoneNumber string
	Text        string
}

// SmsJobStatus is the account state exposed by smsjobs.getStatus.
type SmsJobStatus struct {
	AllowInternational bool
	RecentSent         int32
	RecentSince        int32
	RecentRemains      int32
	TotalSent          int32
	TotalSince         int32
	LastGiftSlug       string
}

// SmsjobsTermsURL is intentionally configurable so deployments can point
// clients at the terms accepted by their SMS provider.
func SmsjobsTermsURL() string {
	return strings.TrimSpace(os.Getenv("TEAMGRAM_SMSJOBS_TERMS_URL"))
}

func validateSmsJobUser(userID int64) error {
	if userID <= 0 {
		return errors.New("invalid sms job user")
	}
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	return nil
}

// SmsjobsEligibility returns the current monthly sent count. The Telegram
// layer uses this value to decide whether to display the opt-in prompt.
func SmsjobsEligibility(userID int64) (int32, error) {
	if err := validateSmsJobUser(userID); err != nil {
		return 0, err
	}
	var count int32
	err := db.QueryRow(`SELECT CASE WHEN recent_since=0 OR recent_since+$1<=EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
		THEN 0 ELSE recent_sent END
		FROM apifull_sms_job_member WHERE user_id=$2`, smsJobRecentWindow, userID).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return count, err
}

// SmsjobsJoin opts a user into SMS assignments. The upsert keeps repeated
// requests idempotent and preserves the user's settings and counters.
func SmsjobsJoin(userID int64) error {
	if err := validateSmsJobUser(userID); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	_, err = tx.Exec(`INSERT INTO apifull_sms_job_member
		(user_id, joined, recent_since, total_since, updated_at)
		VALUES ($1, TRUE, $2, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET joined=TRUE, updated_at=CURRENT_TIMESTAMP`, userID, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SmsjobsLeave removes a user from the assignment pool without deleting the
// historical counters or settings.
func SmsjobsLeave(userID int64) error {
	if err := validateSmsJobUser(userID); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`UPDATE apifull_sms_job_member SET joined=FALSE, updated_at=CURRENT_TIMESTAMP WHERE user_id=$1`, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SmsjobsUpdateSettings updates the optional allow-international flag.
func SmsjobsUpdateSettings(userID int64, allowInternational bool) error {
	if err := validateSmsJobUser(userID); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`INSERT INTO apifull_sms_job_member (user_id, allow_international, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET allow_international=EXCLUDED.allow_international,
		updated_at=CURRENT_TIMESTAMP`, userID, allowInternational)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SmsjobsStatus reads a user's counters. A missing member is a valid empty
// status, which lets clients render the opt-in state before joining.
func SmsjobsStatus(userID int64) (SmsJobStatus, error) {
	if err := validateSmsJobUser(userID); err != nil {
		return SmsJobStatus{}, err
	}
	var out SmsJobStatus
	var recentSent, recentSince, totalSent, totalSince int64
	var lastGift sql.NullString
	err := db.QueryRow(`SELECT allow_international, recent_sent, recent_since, total_sent,
		total_since, last_gift_slug FROM apifull_sms_job_member WHERE user_id=$1`, userID).
		Scan(&out.AllowInternational, &recentSent, &recentSince, &totalSent, &totalSince, &lastGift)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return SmsJobStatus{}, err
	}
	now := time.Now().Unix()
	if recentSince == 0 || now >= recentSince+smsJobRecentWindow {
		recentSent, recentSince = 0, now
	}
	if lastGift.Valid {
		out.LastGiftSlug = lastGift.String
	}
	out.RecentSent = int32(recentSent)
	out.RecentSince = int32(recentSince)
	out.RecentRemains = int32(maxSmsJobsPerMonth - recentSent)
	if out.RecentRemains < 0 {
		out.RecentRemains = 0
	}
	out.TotalSent = int32(totalSent)
	out.TotalSince = int32(totalSince)
	return out, nil
}

// maxSmsJobsPerMonth is deliberately conservative until a provider supplies a
// deployment-specific quota. It is also the value used by recent_remains.
const maxSmsJobsPerMonth int64 = 100

// GetSmsJob returns an assignment owned by the requesting user.
func GetSmsJob(userID int64, jobID string) (SmsJobRecord, error) {
	if err := validateSmsJobUser(userID); err != nil {
		return SmsJobRecord{}, err
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return SmsJobRecord{}, ErrSmsJobIDInvalid
	}
	var out SmsJobRecord
	err := db.QueryRow(`SELECT job_id, user_id, phone_number, text
		FROM apifull_sms_job WHERE job_id=$1 AND user_id=$2 AND state='pending'`, jobID, userID).
		Scan(&out.JobID, &out.UserID, &out.PhoneNumber, &out.Text)
	if errors.Is(err, sql.ErrNoRows) {
		return SmsJobRecord{}, ErrSmsJobNotFound
	}
	return out, err
}

// FinishSmsJob acknowledges an assignment exactly once. Successful sends
// increment both counters; failed sends retain the assignment history without
// crediting the volunteer.
func FinishSmsJob(userID int64, jobID, failure string) error {
	if err := validateSmsJobUser(userID); err != nil {
		return err
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ErrSmsJobIDInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	if err = tx.QueryRow(`SELECT state FROM apifull_sms_job WHERE job_id=$1 AND user_id=$2 FOR UPDATE`, jobID, userID).Scan(&state); errors.Is(err, sql.ErrNoRows) {
		return ErrSmsJobNotFound
	} else if err != nil {
		return err
	}
	if state != "pending" {
		return tx.Commit()
	}
	now := time.Now().Unix()
	if _, err = tx.Exec(`UPDATE apifull_sms_job SET state='finished', error_text=$1, finished_at=$2 WHERE job_id=$3 AND user_id=$4`, strings.TrimSpace(failure), now, jobID, userID); err != nil {
		return err
	}
	if strings.TrimSpace(failure) == "" {
		if _, err = tx.Exec(`INSERT INTO apifull_sms_job_member
			(user_id, joined, recent_sent, recent_since, total_sent, total_since, updated_at)
			VALUES ($1, TRUE, 1, $2, 1, $2, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id) DO UPDATE SET
			recent_sent=CASE WHEN apifull_sms_job_member.recent_since=0 OR apifull_sms_job_member.recent_since+$3<=$2
				THEN 1 ELSE apifull_sms_job_member.recent_sent+1 END,
			recent_since=CASE WHEN apifull_sms_job_member.recent_since=0 OR apifull_sms_job_member.recent_since+$3<=$2
				THEN $2 ELSE apifull_sms_job_member.recent_since END,
			total_sent=apifull_sms_job_member.total_sent+1,
			total_since=CASE WHEN apifull_sms_job_member.total_since=0 THEN $2 ELSE apifull_sms_job_member.total_since END,
			updated_at=CURRENT_TIMESTAMP`, userID, now, smsJobRecentWindow); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertSmsJob is the provider handoff used by workers that assign a job to a
// volunteer. It is idempotent by job ID and keeps assignment ownership explicit.
func UpsertSmsJob(job SmsJobRecord) error {
	if err := validateSmsJobUser(job.UserID); err != nil {
		return err
	}
	job.JobID = strings.TrimSpace(job.JobID)
	if job.JobID == "" {
		return ErrSmsJobIDInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`INSERT INTO apifull_sms_job (job_id, user_id, phone_number, text, assigned_at)
		VALUES ($1,$2,$3,$4,EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint)
		ON CONFLICT (job_id) DO UPDATE SET user_id=EXCLUDED.user_id,
		phone_number=EXCLUDED.phone_number, text=EXCLUDED.text,
		state='pending', error_text=NULL, assigned_at=EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint,
		finished_at=0 WHERE apifull_sms_job.state='pending'`, job.JobID, job.UserID, job.PhoneNumber, job.Text)
	if err != nil {
		return err
	}
	return tx.Commit()
}
