package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var errStarsSubscriptionNotProvisioned = errors.New("Stars subscription is not provisioned")

func isStarsSubscriptionMutationOperation(operation string) bool {
	switch operation {
	case "change_stars_subscription", "fulfill_stars_subscription", "bot_cancel_stars_subscription":
		return true
	default:
		return false
	}
}

func rawBool(fields map[string]json.RawMessage, name string) bool {
	raw := fields[name]
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return false
	}
	if predicate := rawString(object, "predicate_name"); predicate != "" {
		return predicate == "boolTrue"
	}
	return objectInt64(object, "constructor") == -1720552011
}

func rawPeer(fields map[string]json.RawMessage) []byte {
	peer := fields["peer"]
	if len(peer) == 0 || string(peer) == "null" {
		return nil
	}
	return append([]byte(nil), peer...)
}

func (p *controlPlane) starsSubscriptionResponse(ctx context.Context, fields map[string]json.RawMessage) ([]byte, error) {
	if p == nil || p.db == nil {
		return nil, errors.New("payment provider PostgreSQL is unavailable")
	}
	userID := rawInt64(fields, "user_id")
	if userID <= 0 {
		return nil, errors.New("invalid Stars subscription user")
	}
	var balance int64
	if err := p.db.QueryRowContext(ctx, `
		SELECT balance FROM apifull_payment_provider_stars_account WHERE user_id=$1`, userID).Scan(&balance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errStarsSubscriptionNotProvisioned
		}
		return nil, err
	}

	offset := strings.TrimSpace(rawString(fields, "offset"))
	if offset != "" {
		if offset != "0" {
			return nil, errors.New("unsupported Stars subscription offset")
		}
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT subscription_id, peer, until_date, pricing_period, pricing_amount,
		       canceled, can_refulfill, missing_balance, bot_canceled,
		       chat_invite_hash, title, photo, invoice_slug
		  FROM apifull_payment_provider_stars_subscription
		 WHERE user_id=$1
		 ORDER BY subscription_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subscriptions := make([]any, 0)
	for rows.Next() {
		var (
			subscriptionID, chatInviteHash, title, invoiceSlug  sql.NullString
			peer, photo                                         []byte
			untilDate, pricingPeriod                            int32
			pricingAmount                                       int64
			canceled, canRefulfill, missingBalance, botCanceled bool
		)
		if err := rows.Scan(&subscriptionID, &peer, &untilDate, &pricingPeriod, &pricingAmount,
			&canceled, &canRefulfill, &missingBalance, &botCanceled,
			&chatInviteHash, &title, &photo, &invoiceSlug); err != nil {
			return nil, err
		}
		if !subscriptionID.Valid || len(peer) == 0 || pricingPeriod <= 0 || pricingAmount < 0 || untilDate <= 0 {
			return nil, errors.New("stored Stars subscription is invalid")
		}
		var peerValue any
		if err := json.Unmarshal(peer, &peerValue); err != nil {
			return nil, errors.New("stored Stars subscription peer is invalid")
		}
		value := map[string]any{
			"predicate_name":  "starsSubscription",
			"constructor":     779004698,
			"canceled":        canceled,
			"can_refulfill":   canRefulfill,
			"missing_balance": missingBalance,
			"bot_canceled":    botCanceled,
			"id":              subscriptionID.String,
			"peer":            peerValue,
			"until_date":      untilDate,
			"pricing": map[string]any{
				"predicate_name": "starsSubscriptionPricing",
				"constructor":    88173912,
				"period":         pricingPeriod,
				"amount":         pricingAmount,
			},
		}
		if chatInviteHash.Valid {
			value["chat_invite_hash"] = map[string]any{"value": chatInviteHash.String}
		}
		if title.Valid {
			value["title"] = map[string]any{"value": title.String}
		}
		if len(photo) > 0 {
			var photoValue any
			if err := json.Unmarshal(photo, &photoValue); err != nil {
				return nil, errors.New("stored Stars subscription photo is invalid")
			}
			value["photo"] = photoValue
		}
		if invoiceSlug.Valid {
			value["invoice_slug"] = map[string]any{"value": invoiceSlug.String}
		}
		subscriptions = append(subscriptions, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"verified":      true,
		"user_id":       userID,
		"balance":       balance,
		"subscriptions": subscriptions,
	})
}

func (p *controlPlane) starsSubscriptionMutationResponse(ctx context.Context, fields map[string]json.RawMessage) ([]byte, error) {
	callerID := rawInt64(fields, "caller_user_id")
	userID := rawInt64(fields, "user_id")
	subscriptionID := strings.TrimSpace(rawString(fields, "subscription_id"))
	if subscriptionID == "" {
		subscriptionID = strings.TrimSpace(rawString(fields, "charge_id"))
	}
	if callerID <= 0 || userID <= 0 || subscriptionID == "" {
		return nil, errors.New("invalid Stars subscription mutation")
	}
	if p == nil || p.db == nil {
		return nil, errors.New("payment provider PostgreSQL is unavailable")
	}
	var exists bool
	if err := p.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM apifull_payment_provider_stars_subscription
			 WHERE user_id=$1 AND subscription_id=$2
		)`, userID, subscriptionID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, errStarsSubscriptionNotProvisioned
	}
	return json.Marshal(map[string]any{
		"verified":        true,
		"caller_user_id":  callerID,
		"user_id":         userID,
		"subscription_id": subscriptionID,
		"result":          map[string]any{"predicate_name": "boolTrue", "constructor": -1720552011},
	})
}

func (p *controlPlane) applyStarsSubscriptionMutationTx(tx *sql.Tx, operation string, payload []byte) error {
	if tx == nil {
		return errors.New("payment provider transaction is unavailable")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return fmt.Errorf("invalid Stars subscription payload: %w", err)
	}
	userID := rawInt64(fields, "user_id")
	subscriptionID := strings.TrimSpace(rawString(fields, "subscription_id"))
	if subscriptionID == "" {
		subscriptionID = strings.TrimSpace(rawString(fields, "charge_id"))
	}
	if userID <= 0 || subscriptionID == "" {
		return errors.New("invalid Stars subscription mutation")
	}
	var period int32
	if err := tx.QueryRow(`
		SELECT pricing_period
		  FROM apifull_payment_provider_stars_subscription
		 WHERE user_id=$1 AND subscription_id=$2 FOR UPDATE`, userID, subscriptionID).Scan(&period); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errStarsSubscriptionNotProvisioned
		}
		return err
	}
	var query string
	var args []any
	switch operation {
	case "change_stars_subscription":
		canceled := rawBool(fields, "canceled")
		query = `UPDATE apifull_payment_provider_stars_subscription
		            SET canceled=$3, can_refulfill=$3, bot_canceled=FALSE, updated_at=CURRENT_TIMESTAMP
		          WHERE user_id=$1 AND subscription_id=$2`
		args = []any{userID, subscriptionID, canceled}
	case "fulfill_stars_subscription":
		query = `UPDATE apifull_payment_provider_stars_subscription
		            SET canceled=FALSE, can_refulfill=FALSE, missing_balance=FALSE, bot_canceled=FALSE,
		                until_date=GREATEST(until_date, EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INTEGER) + $3,
		                updated_at=CURRENT_TIMESTAMP
		          WHERE user_id=$1 AND subscription_id=$2`
		args = []any{userID, subscriptionID, period}
	case "bot_cancel_stars_subscription":
		restore := rawBool(fields, "restore")
		query = `UPDATE apifull_payment_provider_stars_subscription
		            SET canceled=$3, can_refulfill=$3, bot_canceled=$3, updated_at=CURRENT_TIMESTAMP
		          WHERE user_id=$1 AND subscription_id=$2`
		args = []any{userID, subscriptionID, !restore}
	default:
		return nil
	}
	result, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return errStarsSubscriptionNotProvisioned
	}
	return nil
}

func staticStarsSubscriptionResponse(fields map[string]json.RawMessage) ([]byte, error) {
	userID := rawInt64(fields, "user_id")
	if userID <= 0 {
		return nil, errors.New("invalid Stars subscription user")
	}
	return json.Marshal(map[string]any{
		"verified": true,
		"user_id":  userID,
		"balance":  100,
		"subscriptions": []any{map[string]any{
			"predicate_name": "starsSubscription",
			"constructor":    779004698,
			"id":             fmt.Sprintf("fixture-subscription-%d", userID),
			"peer":           map[string]any{"predicate_name": "peerUser", "constructor": 1498486562, "user_id": userID},
			"until_date":     time.Now().Add(30 * 24 * time.Hour).Unix(),
			"pricing":        map[string]any{"predicate_name": "starsSubscriptionPricing", "constructor": 88173912, "period": 2592000, "amount": 100},
		}},
	})
}
