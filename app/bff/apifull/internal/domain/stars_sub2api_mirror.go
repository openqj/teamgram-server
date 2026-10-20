// Copyright 2026 Teamgram Authors
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package domain

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"time"
)

var (
	ErrStarsUsageConflict       = errors.New("Sub2API usage event conflicts with an existing event")
	ErrStarsMirrorClaimLost     = errors.New("Stars mirror outbox claim was replaced")
	ErrStarsMirrorSchemaMissing = errors.New("Stars mirror PostgreSQL schema is unavailable")
)

const (
	starsPerUSDNumerator   int64 = 36
	starsPerUSDDenominator int64 = 5
	usdSnapshotScale             = 8
)

type StarsMirrorOutboxEvent struct {
	EventID    string
	UserID     int64
	DeltaStars int64
	AmountUSD  string
	Attempts   int
	ClaimToken string
}

type Sub2APIStarsUsageEvent struct {
	EventID   string
	AccountID string
	UserID    int64
	AmountUSD string
	Currency  string
}

// ApplySub2APIStarsUsage records an external signed USD balance delta and
// applies the matching integer Stars delta in one transaction. Each USD event
// is rounded away from zero independently at the fixed 7.2 Stars per USD rate.
func ApplySub2APIStarsUsage(ctx context.Context, event Sub2APIStarsUsageEvent) (int64, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	event.EventID = strings.TrimSpace(event.EventID)
	event.AccountID = strings.TrimSpace(event.AccountID)
	event.Currency = strings.ToUpper(strings.TrimSpace(event.Currency))
	amount, amountText, err := parseStarsMirrorDecimal(event.AmountUSD, 18)
	if err != nil || amount.Sign() == 0 || event.EventID == "" || len(event.EventID) > 191 || !validStarsMirrorAccountID(event.AccountID) || event.UserID <= 0 || event.Currency != "USD" {
		return 0, ErrPaymentRecoveryInvalid
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if existingStars, found, readErr := findSub2APIStarsUsage(ctx, tx, event); readErr != nil {
		return 0, readErr
	} else if found {
		if err = tx.Commit(); err != nil {
			return 0, err
		}
		return existingStars, nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO apifull_sub2api_stars_account (account_id, user_id)
		VALUES ($1,$2) ON CONFLICT (account_id) DO NOTHING`, event.AccountID, event.UserID); err != nil {
		return 0, err
	}
	var mappedUserID int64
	if err = tx.QueryRowContext(ctx, `SELECT user_id FROM apifull_sub2api_stars_account
		WHERE account_id=$1 FOR UPDATE`, event.AccountID).Scan(&mappedUserID); err != nil {
		return 0, err
	}
	if mappedUserID != event.UserID {
		return 0, ErrStarsUsageConflict
	}
	// A concurrent replay can have committed while this transaction waited for
	// the account mapping lock. Re-read the event before applying its delta.
	if existingStars, found, readErr := findSub2APIStarsUsage(ctx, tx, event); readErr != nil {
		return 0, readErr
	} else if found {
		if err = tx.Commit(); err != nil {
			return 0, err
		}
		return existingStars, nil
	}
	stars, err := starsFromSub2APIUSD(amount)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO apifull_sub2api_stars_usage_event
		(event_id, account_id, user_id, amount_usd, currency, applied_stars_delta, created_at)
		VALUES ($1,$2,$3,$4::numeric,$5,$6,CURRENT_TIMESTAMP) ON CONFLICT (event_id) DO NOTHING`,
		event.EventID, event.AccountID, event.UserID, amountText, event.Currency, stars)
	if err != nil {
		return 0, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if inserted == 0 {
		existingStars, found, readErr := findSub2APIStarsUsage(ctx, tx, event)
		if readErr != nil {
			return 0, readErr
		}
		if !found {
			return 0, ErrStarsUsageConflict
		}
		if err = tx.Commit(); err != nil {
			return 0, err
		}
		return existingStars, nil
	}
	if stars != 0 {
		if _, err = applyStarsTxWithoutMirror(tx, event.UserID, stars, sub2APIUsageStarsIdem(event.EventID)); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return stars, nil
}

func findSub2APIStarsUsage(ctx context.Context, tx *sql.Tx, event Sub2APIStarsUsageEvent) (int64, bool, error) {
	var accountID, amountText, currency string
	var userID, stars int64
	err := tx.QueryRowContext(ctx, `SELECT account_id, user_id, amount_usd::text, currency, applied_stars_delta
		FROM apifull_sub2api_stars_usage_event WHERE event_id=$1 FOR UPDATE`, event.EventID).
		Scan(&accountID, &userID, &amountText, &currency, &stars)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	storedAmount, _, parseErr := parseStarsMirrorDecimal(amountText, 18)
	inputAmount, _, inputErr := parseStarsMirrorDecimal(event.AmountUSD, 18)
	if parseErr != nil || inputErr != nil || accountID != event.AccountID || userID != event.UserID || currency != event.Currency || storedAmount.Cmp(inputAmount) != 0 {
		return 0, false, ErrStarsUsageConflict
	}
	return stars, true, nil
}

func starsFromSub2APIUSD(amount *big.Rat) (int64, error) {
	if amount == nil || amount.Sign() == 0 {
		return 0, ErrPaymentRecoveryInvalid
	}
	numerator := new(big.Int).Mul(new(big.Int).Abs(new(big.Int).Set(amount.Num())), big.NewInt(starsPerUSDNumerator))
	denominator := new(big.Int).Mul(new(big.Int).Set(amount.Denom()), big.NewInt(starsPerUSDDenominator))
	stars, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if remainder.Sign() > 0 {
		stars.Add(stars, big.NewInt(1))
	}
	if amount.Sign() < 0 {
		stars.Neg(stars)
	}
	if !stars.IsInt64() {
		return 0, ErrInvalidStarsTransaction
	}
	return stars.Int64(), nil
}

func usdFromStars(delta int64) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(usdSnapshotScale), nil)
	numerator := new(big.Int).Mul(new(big.Int).Abs(big.NewInt(delta)), big.NewInt(starsPerUSDDenominator))
	numerator.Mul(numerator, scale)
	denominator := big.NewInt(starsPerUSDNumerator)
	units, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(denominator) >= 0 {
		units.Add(units, big.NewInt(1))
	}
	digits := units.String()
	if len(digits) <= usdSnapshotScale {
		digits = strings.Repeat("0", usdSnapshotScale+1-len(digits)) + digits
	}
	return digits[:len(digits)-usdSnapshotScale] + "." + digits[len(digits)-usdSnapshotScale:]
}

func parseStarsMirrorDecimal(raw string, maxScale int) (*big.Rat, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 64 {
		return nil, "", ErrPaymentRecoveryInvalid
	}
	negative := strings.HasPrefix(value, "-")
	unsigned := strings.TrimPrefix(value, "-")
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (parts[1] == "" || len(parts[1]) > maxScale)) {
		return nil, "", ErrPaymentRecoveryInvalid
	}
	for _, part := range parts {
		for _, char := range part {
			if char < '0' || char > '9' {
				return nil, "", ErrPaymentRecoveryInvalid
			}
		}
	}
	integer := strings.TrimLeft(parts[0], "0")
	if integer == "" {
		integer = "0"
	}
	canonical := integer
	if len(parts) == 2 {
		fraction := strings.TrimRight(parts[1], "0")
		if fraction != "" {
			canonical += "." + fraction
		}
	}
	if negative && canonical != "0" {
		canonical = "-" + canonical
	}
	rat, ok := new(big.Rat).SetString(canonical)
	if !ok {
		return nil, "", ErrPaymentRecoveryInvalid
	}
	return rat, canonical, nil
}

func validStarsMirrorAccountID(value string) bool {
	if len(value) != len("acct:00000000-0000-0000-0000-000000000000") || !strings.HasPrefix(value, "acct:") {
		return false
	}
	for index, char := range value[len("acct:"):] {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func starsMirrorEventID(userID int64, ledgerIdem string) string {
	digest := sha256.Sum256([]byte(big.NewInt(userID).String() + "\x00" + ledgerIdem))
	return "teamgram-stars:" + hex.EncodeToString(digest[:])
}

func sub2APIUsageStarsIdem(eventID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(eventID)))
	return "sub2api-usage:" + hex.EncodeToString(digest[:])
}

func ClaimStarsMirrorOutbox(ctx context.Context, limit int) ([]StarsMirrorOutboxEvent, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if limit <= 0 || limit > 1000 {
		return nil, ErrInvalidStarsTransaction
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT event_id, user_id, delta_stars, attempts
		FROM apifull_stars_sub2api_outbox
		WHERE (state='pending' AND next_attempt_at <= CURRENT_TIMESTAMP)
		   OR (state='inflight' AND lease_until <= CURRENT_TIMESTAMP)
		ORDER BY created_at, event_id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	type claimed struct {
		id       string
		userID   int64
		delta    int64
		attempts int
	}
	var selected []claimed
	for rows.Next() {
		var event claimed
		if err = rows.Scan(&event.id, &event.userID, &event.delta, &event.attempts); err != nil {
			_ = rows.Close()
			return nil, err
		}
		selected = append(selected, event)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	claimedEvents := make([]StarsMirrorOutboxEvent, 0, len(selected))
	for _, event := range selected {
		var token, amountUSD string
		amountSnapshot := usdFromStars(event.delta)
		if err = tx.QueryRowContext(ctx, `UPDATE apifull_stars_sub2api_outbox
			SET amount_usd=COALESCE(amount_usd, $1::numeric),
				state='inflight', attempts=attempts+1, claim_token=gen_random_uuid(),
				lease_until=CURRENT_TIMESTAMP + INTERVAL '60 seconds',
				next_attempt_at=CURRENT_TIMESTAMP + INTERVAL '60 seconds', updated_at=CURRENT_TIMESTAMP
			WHERE event_id=$2 RETURNING claim_token::text, amount_usd::text`, amountSnapshot, event.id).Scan(&token, &amountUSD); err != nil {
			return nil, err
		}
		claimedEvents = append(claimedEvents, StarsMirrorOutboxEvent{
			EventID: event.id, UserID: event.userID, DeltaStars: event.delta, AmountUSD: amountUSD,
			Attempts: event.attempts + 1, ClaimToken: token,
		})
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return claimedEvents, nil
}

func CompleteStarsMirrorOutbox(ctx context.Context, eventID, claimToken string) error {
	if db == nil || strings.TrimSpace(eventID) == "" || strings.TrimSpace(claimToken) == "" {
		return ErrStarsMirrorClaimLost
	}
	result, err := db.ExecContext(ctx, `UPDATE apifull_stars_sub2api_outbox
		SET state='complete', completed_at=CURRENT_TIMESTAMP, claim_token=NULL,
			lease_until=TO_TIMESTAMP(0), updated_at=CURRENT_TIMESTAMP, last_error=''
		WHERE event_id=$1 AND claim_token=$2::uuid AND state='inflight'`, eventID, claimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrStarsMirrorClaimLost
	}
	return nil
}

func RetryStarsMirrorOutbox(ctx context.Context, event StarsMirrorOutboxEvent, deliveryErr error) error {
	if db == nil || strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.ClaimToken) == "" {
		return ErrStarsMirrorClaimLost
	}
	message := "Sub2API balance mirror failed"
	if deliveryErr != nil {
		message = deliveryErr.Error()
	}
	if len(message) > 255 {
		message = message[:255]
	}
	delay := time.Duration(1<<min(event.Attempts, 11)) * time.Second
	if delay > time.Hour {
		delay = time.Hour
	}
	result, err := db.ExecContext(ctx, `UPDATE apifull_stars_sub2api_outbox
		SET state='pending', next_attempt_at=$1, lease_until=TO_TIMESTAMP(0), claim_token=NULL,
			last_error=$2, updated_at=CURRENT_TIMESTAMP
		WHERE event_id=$3 AND claim_token=$4::uuid AND state='inflight'`, time.Now().Add(delay), message, event.EventID, event.ClaimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrStarsMirrorClaimLost
	}
	return nil
}
