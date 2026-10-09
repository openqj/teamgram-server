package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

const (
	BoostScopePremium          = "premium"
	BoostScopeStories          = "stories"
	maxBoostSlots              = 4
	boostLifetime              = 30 * 24 * 60 * 60
	boostPeerTypeChannel int32 = 4
)

var (
	ErrBoostTargetNotFound  = errors.New("boost target does not exist")
	ErrBoostForbidden       = errors.New("boost target permission denied")
	ErrBoostSlotInvalid     = errors.New("invalid boost slot")
	ErrBoostSlotUnavailable = errors.New("no boost slot available")
)

// BoostTarget identifies one Layer 229 boostable peer. Scope keeps channel
// boosts and story boosts independent even when they point at the same peer.
type BoostTarget struct {
	Scope         string
	PeerType      int32
	PeerID        int64
	OwnerUserID   int64
	Boosts        int32
	BlockedBoosts int32
}

// BoostSlot is the durable assignment behind premium.myBoosts and boost rows.
type BoostSlot struct {
	UserID        int64
	Slot          int32
	Scope         string
	PeerType      int32
	PeerID        int64
	Gift          bool
	Giveaway      bool
	Unclaimed     bool
	Date          int32
	Expires       int32
	CooldownUntil int32
	UsedGiftSlug  string
	Multiplier    int32
	Stars         int64
}

func validBoostScope(scope string) bool {
	return scope == BoostScopePremium || scope == BoostScopeStories
}

func validBoostTarget(scope string, peerType int32, peerID int64) error {
	if !validBoostScope(scope) || peerType <= 0 || peerID <= 0 {
		return ErrBoostTargetNotFound
	}
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	return nil
}

func validBoostSlot(slot int32) bool { return slot > 0 && slot <= maxBoostSlots }

func normalizeBoostSlots(slots []int32) ([]int32, error) {
	if len(slots) == 0 {
		return nil, ErrBoostSlotUnavailable
	}
	seen := make(map[int32]struct{}, len(slots))
	out := make([]int32, 0, len(slots))
	for _, slot := range slots {
		if !validBoostSlot(slot) {
			return nil, ErrBoostSlotInvalid
		}
		if _, ok := seen[slot]; ok {
			return nil, ErrBoostSlotInvalid
		}
		seen[slot] = struct{}{}
		out = append(out, slot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func boostSlotID(userID int64, slot int32) string {
	return fmt.Sprintf("%d:%d", userID, slot)
}

func scanBoostSlot(row interface {
	Scan(...any) error
}) (BoostSlot, error) {
	var out BoostSlot
	var scope sql.NullString
	var peerType, peerID sql.NullInt64
	if err := row.Scan(&out.UserID, &out.Slot, &scope, &peerType, &peerID,
		&out.Gift, &out.Giveaway, &out.Unclaimed, &out.Date, &out.Expires,
		&out.CooldownUntil, &out.UsedGiftSlug, &out.Multiplier, &out.Stars); err != nil {
		return BoostSlot{}, err
	}
	if scope.Valid {
		out.Scope = scope.String
	}
	if peerType.Valid {
		out.PeerType = int32(peerType.Int64)
	}
	if peerID.Valid {
		out.PeerID = peerID.Int64
	}
	return out, nil
}

func boostSlotArgs(slot BoostSlot) []any {
	var scope any
	var peerType, peerID any
	if slot.Scope != "" {
		scope, peerType, peerID = slot.Scope, slot.PeerType, slot.PeerID
	}
	return []any{slot.UserID, slot.Slot, scope, peerType, peerID, slot.Gift, slot.Giveaway,
		slot.Unclaimed, slot.Date, slot.Expires, slot.CooldownUntil, slot.UsedGiftSlug,
		slot.Multiplier, slot.Stars}
}

// LoadBoostTarget returns a zero target when no boost has been applied yet.
func LoadBoostTarget(scope string, peerType int32, peerID int64) (BoostTarget, bool, error) {
	if err := validBoostTarget(scope, peerType, peerID); err != nil {
		return BoostTarget{}, false, err
	}
	var target BoostTarget
	err := db.QueryRow(`SELECT scope, peer_type, peer_id, owner_user_id, boosts, blocked_boosts
		FROM apifull_boost_target WHERE scope=$1 AND peer_type=$2 AND peer_id=$3`, scope, peerType, peerID).
		Scan(&target.Scope, &target.PeerType, &target.PeerID, &target.OwnerUserID, &target.Boosts, &target.BlockedBoosts)
	if errors.Is(err, sql.ErrNoRows) {
		return BoostTarget{Scope: scope, PeerType: peerType, PeerID: peerID}, false, nil
	}
	if err != nil {
		return BoostTarget{}, false, err
	}
	return target, true, nil
}

func ensureBoostTargetTx(tx *sql.Tx, scope string, peerType int32, peerID, ownerID int64) error {
	_, err := tx.Exec(`INSERT INTO apifull_boost_target
		(scope, peer_type, peer_id, owner_user_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (scope, peer_type, peer_id) DO UPDATE SET
		owner_user_id = CASE WHEN apifull_boost_target.owner_user_id=0 THEN EXCLUDED.owner_user_id
			ELSE apifull_boost_target.owner_user_id END,
		updated_at = CURRENT_TIMESTAMP`, scope, peerType, peerID, ownerID)
	return err
}

func loadBoostTargetTx(tx *sql.Tx, scope string, peerType int32, peerID int64, forUpdate bool) (BoostTarget, bool, error) {
	query := `SELECT scope, peer_type, peer_id, owner_user_id, boosts, blocked_boosts
		FROM apifull_boost_target WHERE scope=$1 AND peer_type=$2 AND peer_id=$3`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var target BoostTarget
	err := tx.QueryRow(query, scope, peerType, peerID).
		Scan(&target.Scope, &target.PeerType, &target.PeerID, &target.OwnerUserID, &target.Boosts, &target.BlockedBoosts)
	if errors.Is(err, sql.ErrNoRows) {
		return BoostTarget{}, false, nil
	}
	if err != nil {
		return BoostTarget{}, false, err
	}
	return target, true, nil
}

// ApplyBoost atomically assigns each requested slot to a target and adjusts
// both the previous and new target totals under row locks.
func ApplyBoost(userID int64, scope string, peerType int32, peerID int64, slots []int32) ([]BoostSlot, error) {
	if userID <= 0 {
		return nil, errors.New("invalid boost user")
	}
	if err := validBoostTarget(scope, peerType, peerID); err != nil {
		return nil, err
	}
	normalized, err := normalizeBoostSlots(slots)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// A slot row may be absent, so SELECT FOR UPDATE alone cannot serialize
	// two first assignments. A user-scoped advisory lock closes that race and
	// also gives multi-slot requests a consistent lock order across targets.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("apifull-boost-user:%d", userID)); err != nil {
		return nil, err
	}
	if err = applyBoostTx(tx, userID, scope, peerType, peerID, normalized); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ListUserBoostSlots(userID)
}

func applyBoostTx(tx *sql.Tx, userID int64, scope string, peerType int32, peerID int64, normalized []int32) error {
	if err := ensureBoostTargetTx(tx, scope, peerType, peerID, 0); err != nil {
		return err
	}
	if _, _, err := loadBoostTargetTx(tx, scope, peerType, peerID, true); err != nil {
		return err
	}
	now := int32(time.Now().Unix())
	for _, slot := range normalized {
		old, err := scanBoostSlot(tx.QueryRow(`SELECT user_id, slot, scope, peer_type, peer_id,
			gift, giveaway, unclaimed, boost_date, expires, cooldown_until, used_gift_slug, multiplier, stars
			FROM apifull_boost_slot WHERE user_id=$1 AND slot=$2 FOR UPDATE`, userID, slot))
		if errors.Is(err, sql.ErrNoRows) {
			old = BoostSlot{UserID: userID, Slot: slot}
		} else if err != nil {
			return err
		}
		activeOld := old.Scope != "" && (old.Expires == 0 || old.Expires > now)
		same := activeOld && old.Scope == scope && old.PeerType == peerType && old.PeerID == peerID
		if activeOld && !same {
			if _, err = tx.Exec(`UPDATE apifull_boost_target SET boosts=GREATEST(boosts-1,0), updated_at=CURRENT_TIMESTAMP
				WHERE scope=$1 AND peer_type=$2 AND peer_id=$3`, old.Scope, old.PeerType, old.PeerID); err != nil {
				return err
			}
		}
		if !same {
			if _, err = tx.Exec(`UPDATE apifull_boost_target SET boosts=boosts+1, updated_at=CURRENT_TIMESTAMP
				WHERE scope=$1 AND peer_type=$2 AND peer_id=$3`, scope, peerType, peerID); err != nil {
				return err
			}
		}
		assignment := BoostSlot{UserID: userID, Slot: slot, Scope: scope, PeerType: peerType, PeerID: peerID,
			Date: now, Expires: now + boostLifetime, Multiplier: 1}
		if err = upsertBoostSlotTx(tx, assignment); err != nil {
			return err
		}
	}
	return nil
}

func upsertBoostSlotTx(tx *sql.Tx, slot BoostSlot) error {
	args := boostSlotArgs(slot)
	_, err := tx.Exec(`INSERT INTO apifull_boost_slot
		(user_id, slot, scope, peer_type, peer_id, gift, giveaway, unclaimed, boost_date, expires,
		 cooldown_until, used_gift_slug, multiplier, stars)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (user_id, slot) DO UPDATE SET scope=EXCLUDED.scope, peer_type=EXCLUDED.peer_type,
		peer_id=EXCLUDED.peer_id, gift=EXCLUDED.gift, giveaway=EXCLUDED.giveaway,
		unclaimed=EXCLUDED.unclaimed, boost_date=EXCLUDED.boost_date, expires=EXCLUDED.expires,
		cooldown_until=EXCLUDED.cooldown_until, used_gift_slug=EXCLUDED.used_gift_slug,
		multiplier=EXCLUDED.multiplier, stars=EXCLUDED.stars, updated_at=CURRENT_TIMESTAMP`, args...)
	return err
}

func ListUserBoostSlots(userID int64) ([]BoostSlot, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return []BoostSlot{}, nil
	}
	rows, err := db.Query(`SELECT user_id, slot, scope, peer_type, peer_id, gift, giveaway, unclaimed,
		boost_date, expires, cooldown_until, used_gift_slug, multiplier, stars
		FROM apifull_boost_slot WHERE user_id=$1 AND scope IS NOT NULL
		ORDER BY slot`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BoostSlot, 0)
	for rows.Next() {
		slot, scanErr := scanBoostSlot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if slot.Expires == 0 || slot.Expires > int32(time.Now().Unix()) {
			out = append(out, slot)
		}
	}
	return out, rows.Err()
}

// FirstFreeBoostSlot returns the first available user slot. Slots are kept
// bounded so malformed clients cannot allocate unbounded inventory rows.
func FirstFreeBoostSlot(userID int64) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return 0, ErrBoostSlotUnavailable
	}
	used := make(map[int32]struct{}, maxBoostSlots)
	rows, err := db.Query(`SELECT slot FROM apifull_boost_slot WHERE user_id=$1 AND scope IS NOT NULL
		AND (expires=0 OR expires>$2)`, userID, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var slot int32
		if err = rows.Scan(&slot); err != nil {
			rows.Close()
			return 0, err
		}
		used[slot] = struct{}{}
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	for slot := int32(1); slot <= maxBoostSlots; slot++ {
		if _, ok := used[slot]; !ok {
			return slot, nil
		}
	}
	return 0, ErrBoostSlotUnavailable
}

// ApplyFirstFreeBoost combines free-slot selection and assignment in one
// transaction. This prevents two concurrent stories.applyBoost calls from
// both observing slot one as free and silently replacing each other.
func ApplyFirstFreeBoost(userID int64, scope string, peerType int32, peerID int64) ([]BoostSlot, error) {
	if userID <= 0 {
		return nil, ErrBoostSlotUnavailable
	}
	if err := validBoostTarget(scope, peerType, peerID); err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-boost-user:%d", userID)); err != nil {
		return nil, err
	}
	used := make(map[int32]struct{}, maxBoostSlots)
	rows, err := tx.Query(`SELECT slot FROM apifull_boost_slot WHERE user_id=$1 AND scope IS NOT NULL
		AND (expires=0 OR expires>$2) FOR UPDATE`, userID, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var slot int32
		if err = rows.Scan(&slot); err != nil {
			rows.Close()
			return nil, err
		}
		used[slot] = struct{}{}
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	var slot int32
	for slot = 1; slot <= maxBoostSlots; slot++ {
		if _, ok := used[slot]; !ok {
			break
		}
	}
	if slot > maxBoostSlots {
		return nil, ErrBoostSlotUnavailable
	}
	if err = applyBoostTx(tx, userID, scope, peerType, peerID, []int32{slot}); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return ListUserBoostSlots(userID)
}

// ListBoosts lists active boost rows and returns the unpaginated count.
func ListBoosts(scope string, peerType int32, peerID int64, userID *int64, gifts bool, offset, limit int32) ([]BoostSlot, int32, error) {
	if err := validBoostTarget(scope, peerType, peerID); err != nil {
		return nil, 0, err
	}
	if offset < 0 || limit < 0 {
		return nil, 0, ErrBoostSlotInvalid
	}
	if limit == 0 {
		limit = 100
	}
	args := []any{scope, peerType, peerID, time.Now().Unix()}
	where := `scope=$1 AND peer_type=$2 AND peer_id=$3 AND (expires=0 OR expires>$4)`
	if gifts {
		where += ` AND gift=TRUE`
	}
	if userID != nil {
		where += fmt.Sprintf(" AND user_id=$%d", len(args)+1)
		args = append(args, *userID)
	}
	var count int32
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_boost_slot WHERE `+where, args...).Scan(&count); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any{}, args...), offset, limit)
	rows, err := db.Query(`SELECT user_id, slot, scope, peer_type, peer_id, gift, giveaway, unclaimed,
		boost_date, expires, cooldown_until, used_gift_slug, multiplier, stars
		FROM apifull_boost_slot WHERE `+where+` ORDER BY slot, user_id OFFSET $`+fmt.Sprint(len(args)+1)+` LIMIT $`+fmt.Sprint(len(args)+2), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]BoostSlot, 0)
	for rows.Next() {
		slot, scanErr := scanBoostSlot(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, slot)
	}
	return out, count, rows.Err()
}

func SetBoostRestrictions(actorID int64, peerType int32, peerID int64, boosts int32) error {
	if actorID <= 0 || peerType <= 0 || peerID <= 0 || boosts < 0 {
		return ErrBoostTargetNotFound
	}
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	owner, err := authorizeBoostTargetTx(tx, actorID, peerType, peerID)
	if err != nil {
		return err
	}
	target, found, err := loadBoostTargetTx(tx, BoostScopePremium, peerType, peerID, true)
	if err != nil {
		return err
	}
	if !found || target.OwnerUserID == 0 {
		if !found {
			if err = ensureBoostTargetTx(tx, BoostScopePremium, peerType, peerID, owner); err != nil {
				return err
			}
		} else if _, err = tx.Exec(`UPDATE apifull_boost_target SET owner_user_id=$1 WHERE scope=$2 AND peer_type=$3 AND peer_id=$4`, owner, BoostScopePremium, peerType, peerID); err != nil {
			return err
		}
		target.OwnerUserID = owner
	} else if target.OwnerUserID != owner {
		return ErrBoostForbidden
	}
	if _, err = tx.Exec(`UPDATE apifull_boost_target SET blocked_boosts=$1, updated_at=CURRENT_TIMESTAMP
		WHERE scope=$2 AND peer_type=$3 AND peer_id=$4`, boosts, BoostScopePremium, peerType, peerID); err != nil {
		return err
	}
	return tx.Commit()
}

func authorizeBoostTargetTx(tx *sql.Tx, actorID int64, peerType int32, peerID int64) (int64, error) {
	if peerType != boostPeerTypeChannel {
		return 0, ErrBoostTargetNotFound
	}
	var owner int64
	if err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=$1 FOR UPDATE`, peerID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrBoostTargetNotFound
	} else if err != nil {
		return 0, err
	}
	if owner == actorID {
		return owner, nil
	}
	var adminJSON, bannedJSON string
	if err := tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member WHERE channel_id=$1 AND user_id=$2`, peerID, actorID).Scan(&adminJSON, &bannedJSON); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrBoostForbidden
	} else if err != nil {
		return 0, err
	}
	var admin ChannelAdminRights
	if json.Unmarshal([]byte(adminJSON), &admin) != nil || !admin.ChangeInfo {
		return 0, ErrBoostForbidden
	}
	var banned ChannelBannedRights
	if bannedJSON != "" && json.Unmarshal([]byte(bannedJSON), &banned) == nil && banned.Active(time.Now().Unix()) && banned.ChangeInfo {
		return 0, ErrBoostForbidden
	}
	return owner, nil
}

func BoostStatus(scope string, peerType int32, peerID int64) (BoostTarget, error) {
	target, _, err := LoadBoostTarget(scope, peerType, peerID)
	if err != nil {
		return BoostTarget{}, err
	}
	// Recompute active rows so expired assignments cannot leave a stale
	// aggregate visible to clients after a restart.
	var active int32
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_boost_slot
		WHERE scope=$1 AND peer_type=$2 AND peer_id=$3 AND (expires=0 OR expires>$4)`, scope, peerType, peerID, time.Now().Unix()).Scan(&active); err != nil {
		return BoostTarget{}, err
	}
	target.Boosts = active
	return target, nil
}
