package domain

import (
	"database/sql"
	"errors"
	"time"
)

// CommunityPeerType is the wire peer namespace persisted by the community
// store. Keeping the type next to the durable record prevents a chat id and a
// channel id with the same numeric value from colliding.
type CommunityPeerType int16

const (
	CommunityPeerUser    CommunityPeerType = 1
	CommunityPeerChat    CommunityPeerType = 2
	CommunityPeerChannel CommunityPeerType = 3
)

var (
	ErrCommunityMissing     = errors.New("community does not exist")
	ErrCommunityNotOwner    = errors.New("community owner permission required")
	ErrCommunityPeerMissing = errors.New("community peer does not exist")
	ErrCommunityPeerInvalid = errors.New("invalid community peer")
	ErrCommunityPeerBanned  = errors.New("community peer is banned")
)

type Community struct {
	ID        int64
	OwnerID   int64
	Title     string
	About     string
	Hidden    bool
	Collapsed bool
	CreatedAt int64
}

type CommunityPeer struct {
	CommunityID    int64
	PeerType       CommunityPeerType
	PeerID         int64
	AccessHash     int64
	Visible        *bool
	Approved       bool
	RequestedBy    int64
	RequestedAt    int64
	Banned         bool
	CanViewHistory bool
}

func CreateCommunity(ch Channel, hidden bool) error {
	return createCommunity(ch, hidden, nil)
}

// CreateCommunityWithPeer creates the community channel, metadata, and its
// initial linked peer in one transaction. The create RPC must not expose a
// partially-created community if linking the requested peer fails.
func CreateCommunityWithPeer(ch Channel, hidden bool, peerType CommunityPeerType, peerID, accessHash int64) error {
	if peerID <= 0 || (peerType != CommunityPeerUser && peerType != CommunityPeerChat && peerType != CommunityPeerChannel) {
		return ErrCommunityPeerInvalid
	}
	visible := true
	return createCommunity(ch, hidden, &CommunityPeer{
		PeerType:   peerType,
		PeerID:     peerID,
		AccessHash: accessHash,
		Visible:    &visible,
		Approved:   true,
	})
}

func createCommunity(ch Channel, hidden bool, initialPeer *CommunityPeer) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if ch.ID <= 0 || ch.AccessHash == 0 || ch.Creator <= 0 || ch.Title == "" {
		return ErrCommunityPeerInvalid
	}
	if ch.CreatedAt <= 0 {
		ch.CreatedAt = time.Now().Unix()
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO apifull_channel
		(id, access_hash, migrated_from_chat_id, creator_user_id, title, about, broadcast, megagroup,
		signatures_enabled, signature_profiles_enabled, antispam, hidden_prehistory, participants_hidden,
		slowmode_seconds, location_lat, location_long, location_address, username, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,0,1,0,0,0,0,0,0,NULL,NULL,'','',$7)`,
		ch.ID, ch.AccessHash, nil, ch.Creator, ch.Title, ch.About, ch.CreatedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO apifull_community
		(community_id, owner_user_id, title, about, hidden, created_at)
		VALUES ($1,$2,$3,$4,$5,CURRENT_TIMESTAMP)`, ch.ID, ch.Creator, ch.Title, ch.About, hidden); err != nil {
		return err
	}
	if initialPeer != nil {
		if _, err = tx.Exec(`INSERT INTO apifull_community_peer
			(community_id, peer_type, peer_id, access_hash, visible, approved, requested_by, requested_at)
			VALUES ($1,$2,$3,$4,$5,TRUE,$6,$7)`, ch.ID, initialPeer.PeerType, initialPeer.PeerID,
			initialPeer.AccessHash, initialPeer.Visible, ch.Creator, ch.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func LoadCommunity(id int64) (Community, bool, error) {
	if db == nil {
		return Community{}, false, errors.New("domain PostgreSQL is not open")
	}
	var out Community
	err := db.QueryRow(`SELECT community_id, owner_user_id, title, about, hidden,
		EXTRACT(EPOCH FROM created_at)::bigint FROM apifull_community WHERE community_id=$1`, id).
		Scan(&out.ID, &out.OwnerID, &out.Title, &out.About, &out.Hidden, &out.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Community{}, false, nil
	}
	if err != nil {
		return Community{}, false, err
	}
	return out, true, nil
}

// CommunityCanManage keeps authorization in the same transaction-owned
// projection used by all other APIFull channel mutations. Community admins
// are deliberately restricted to the owner until a dedicated community
// admin-rights projection exists.
func CommunityCanManage(communityID, actorID int64) (bool, error) {
	c, ok, err := LoadCommunity(communityID)
	if err != nil {
		return false, err
	}
	return ok && c.OwnerID == actorID, nil
}

func CommunityCanView(communityID, userID int64) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	var allowed bool
	err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM apifull_community c WHERE c.community_id=$1 AND (
			c.owner_user_id=$2 OR EXISTS (
				SELECT 1 FROM apifull_community_peer cp
				WHERE cp.community_id=c.community_id AND cp.peer_type=$3 AND cp.approved=TRUE AND cp.banned=FALSE
				AND EXISTS (
					SELECT 1 FROM apifull_channel ch WHERE ch.id=cp.peer_id AND (
						ch.creator_user_id=$4 OR EXISTS (
							SELECT 1 FROM apifull_channel_member cm WHERE cm.channel_id=ch.id AND cm.user_id=$5))))))`,
		communityID, userID, CommunityPeerChannel, userID, userID).Scan(&allowed)
	return allowed, err
}

func ListCommunitiesForUser(userID int64) ([]Community, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return []Community{}, nil
	}
	rows, err := db.Query(`SELECT c.community_id, c.owner_user_id, c.title, c.about, c.hidden,
		COALESCE((SELECT s.collapsed FROM apifull_community_dialog_state s
			WHERE s.community_id=c.community_id AND s.user_id=$1), FALSE),
		EXTRACT(EPOCH FROM c.created_at)::bigint FROM apifull_community c
		WHERE c.owner_user_id=$2 OR EXISTS (
			SELECT 1 FROM apifull_community_peer cp
			WHERE cp.community_id=c.community_id AND cp.approved=TRUE AND cp.banned=FALSE
			  AND cp.peer_type=$3 AND (cp.peer_id IN (
				SELECT id FROM apifull_channel WHERE creator_user_id=$4 OR EXISTS (
					SELECT 1 FROM apifull_channel_member cm WHERE cm.channel_id=apifull_channel.id AND cm.user_id=$5)))
		) ORDER BY c.community_id`, userID, userID, CommunityPeerChannel, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	communities := make([]Community, 0)
	for rows.Next() {
		var c Community
		if err = rows.Scan(&c.ID, &c.OwnerID, &c.Title, &c.About, &c.Hidden, &c.Collapsed, &c.CreatedAt); err != nil {
			return nil, err
		}
		communities = append(communities, c)
	}
	return communities, rows.Err()
}

func ListCommunityPeers(communityID int64, pendingOnly bool) ([]CommunityPeer, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	query := `SELECT community_id, peer_type, peer_id, access_hash, visible, approved,
		requested_by, requested_at, banned, can_view_history FROM apifull_community_peer
		WHERE community_id=$1 AND banned=FALSE`
	args := []any{communityID}
	if pendingOnly {
		query += ` AND approved=FALSE`
	} else {
		query += ` AND approved=TRUE`
	}
	query += ` ORDER BY requested_at ASC, peer_type ASC, peer_id ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CommunityPeer, 0)
	for rows.Next() {
		var p CommunityPeer
		var visible sql.NullBool
		if err = rows.Scan(&p.CommunityID, &p.PeerType, &p.PeerID, &p.AccessHash, &visible, &p.Approved,
			&p.RequestedBy, &p.RequestedAt, &p.Banned, &p.CanViewHistory); err != nil {
			return nil, err
		}
		if visible.Valid {
			v := visible.Bool
			p.Visible = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func ToggleCommunityPeer(actorID, communityID int64, peerType CommunityPeerType, peerID, accessHash int64,
	visible *bool, deleted bool, approved bool, requestedAt int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if communityID <= 0 || peerID <= 0 || (peerType != CommunityPeerUser && peerType != CommunityPeerChat && peerType != CommunityPeerChannel) {
		return ErrCommunityPeerInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner int64
	if err = tx.QueryRow(`SELECT owner_user_id FROM apifull_community WHERE community_id=$1 FOR UPDATE`, communityID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
		return ErrCommunityMissing
	} else if err != nil {
		return err
	}
	if owner != actorID {
		return ErrCommunityNotOwner
	}
	if deleted {
		if _, err = tx.Exec(`DELETE FROM apifull_community_peer WHERE community_id=$1 AND peer_type=$2 AND peer_id=$3`, communityID, peerType, peerID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if requestedAt <= 0 {
		requestedAt = time.Now().Unix()
	}
	if _, err = tx.Exec(`INSERT INTO apifull_community_peer
		(community_id, peer_type, peer_id, access_hash, visible, approved, requested_by, requested_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (community_id, peer_type, peer_id) DO UPDATE SET access_hash=EXCLUDED.access_hash,
		visible=CASE WHEN EXCLUDED.visible IS NULL THEN apifull_community_peer.visible ELSE EXCLUDED.visible END,
		approved=EXCLUDED.approved, requested_by=EXCLUDED.requested_by,
		requested_at=EXCLUDED.requested_at, banned=FALSE`, communityID, peerType, peerID, accessHash,
		visible, approved, actorID, requestedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func ApproveCommunityPeer(actorID, communityID int64, peerType CommunityPeerType, peerID int64, reject bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner int64
	if err = tx.QueryRow(`SELECT owner_user_id FROM apifull_community WHERE community_id=$1 FOR UPDATE`, communityID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
		return ErrCommunityMissing
	} else if err != nil {
		return err
	}
	if owner != actorID {
		return ErrCommunityNotOwner
	}
	if reject {
		res, execErr := tx.Exec(`DELETE FROM apifull_community_peer WHERE community_id=$1 AND peer_type=$2 AND peer_id=$3 AND approved=FALSE`, communityID, peerType, peerID)
		if execErr != nil {
			return execErr
		}
		if n, rowsErr := res.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if n == 0 {
			return ErrCommunityPeerMissing
		}
	} else {
		res, execErr := tx.Exec(`UPDATE apifull_community_peer SET approved=TRUE WHERE community_id=$1 AND peer_type=$2 AND peer_id=$3 AND approved=FALSE`, communityID, peerType, peerID)
		if execErr != nil {
			return execErr
		}
		if n, rowsErr := res.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if n == 0 {
			return ErrCommunityPeerMissing
		}
	}
	return tx.Commit()
}

func ApproveAllCommunityPeers(actorID, communityID int64, reject bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner int64
	if err = tx.QueryRow(`SELECT owner_user_id FROM apifull_community WHERE community_id=$1 FOR UPDATE`, communityID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
		return ErrCommunityMissing
	} else if err != nil {
		return err
	}
	if owner != actorID {
		return ErrCommunityNotOwner
	}
	if reject {
		_, err = tx.Exec(`DELETE FROM apifull_community_peer WHERE community_id=$1 AND approved=FALSE`, communityID)
	} else {
		_, err = tx.Exec(`UPDATE apifull_community_peer SET approved=TRUE WHERE community_id=$1 AND approved=FALSE`, communityID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func BanCommunityPeer(actorID, communityID int64, peerType CommunityPeerType, peerID int64, unban bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner int64
	if err = tx.QueryRow(`SELECT owner_user_id FROM apifull_community WHERE community_id=$1 FOR UPDATE`, communityID).Scan(&owner); errors.Is(err, sql.ErrNoRows) {
		return ErrCommunityMissing
	} else if err != nil {
		return err
	}
	if owner != actorID {
		return ErrCommunityNotOwner
	}
	if unban {
		_, err = tx.Exec(`UPDATE apifull_community_peer SET banned=FALSE WHERE community_id=$1 AND peer_type=$2 AND peer_id=$3`, communityID, peerType, peerID)
	} else {
		_, err = tx.Exec(`INSERT INTO apifull_community_peer
			(community_id, peer_type, peer_id, approved, requested_by, requested_at, banned)
			VALUES ($1,$2,$3,TRUE,$4,$5,TRUE)
			ON CONFLICT (community_id, peer_type, peer_id) DO UPDATE SET banned=TRUE`,
			communityID, peerType, peerID, actorID, time.Now().Unix())
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func SetCommunityCollapsed(userID, communityID int64, collapsed bool) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 || communityID <= 0 {
		return ErrCommunityPeerInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM apifull_community WHERE community_id=$1)`, communityID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrCommunityMissing
	}
	if _, err = tx.Exec(`INSERT INTO apifull_community_dialog_state (user_id, community_id, collapsed)
		VALUES ($1,$2,$3) ON CONFLICT (user_id, community_id) DO UPDATE SET collapsed=EXCLUDED.collapsed,
		updated_at=CURRENT_TIMESTAMP`, userID, communityID, collapsed); err != nil {
		return err
	}
	return tx.Commit()
}

func CommunityCollapsed(userID, communityID int64) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	var collapsed bool
	err := db.QueryRow(`SELECT collapsed FROM apifull_community_dialog_state WHERE user_id=$1 AND community_id=$2`, userID, communityID).Scan(&collapsed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return collapsed, err
}
