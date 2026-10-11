package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/teamgram/proto/mtproto"
)

var (
	ErrSponsoredInventoryInvalid     = errors.New("apifull: invalid sponsored inventory payload")
	ErrSponsoredInventoryUnavailable = errors.New("apifull: sponsored inventory requires PostgreSQL")
)

// LoadSponsoredPeers reads the deployment-owned peers constructor. A missing
// row is distinct from an invalid row so callers can fail closed when no ad
// inventory has been provisioned.
func LoadSponsoredPeers(ctx context.Context, query string) (*mtproto.Contacts_SponsoredPeers, bool, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, false, ErrSponsoredInventoryUnavailable
	}
	var raw []byte
	err := store.db.QueryRowContext(ctx, `SELECT payload FROM apifull_sponsored_inventory
		WHERE inventory_type = 'peers' AND query = $1 AND active
		AND (starts_at IS NULL OR starts_at <= CURRENT_TIMESTAMP)
		AND (ends_at IS NULL OR ends_at > CURRENT_TIMESTAMP)
		ORDER BY id DESC LIMIT 1`, query).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var result mtproto.Contacts_SponsoredPeers
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, &result) != nil {
		return nil, true, ErrSponsoredInventoryInvalid
	}
	if result.GetPredicateName() != "contacts_sponsoredPeers" && result.GetPredicateName() != "contacts_sponsoredPeersEmpty" {
		return nil, true, ErrSponsoredInventoryInvalid
	}
	return &result, true, nil
}

// StoreSponsoredPeers provisions the deployment-owned peers inventory.
func StoreSponsoredPeers(ctx context.Context, query string, peers *mtproto.Contacts_SponsoredPeers) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return ErrSponsoredInventoryUnavailable
	}
	if peers == nil || (peers.GetPredicateName() != "contacts_sponsoredPeers" && peers.GetPredicateName() != "contacts_sponsoredPeersEmpty") {
		return ErrSponsoredInventoryInvalid
	}
	raw, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO apifull_sponsored_inventory (inventory_type, query, payload)
		VALUES ('peers', $1, $2::jsonb)
		ON CONFLICT (inventory_type, query) WHERE inventory_type = 'peers'
		DO UPDATE SET payload = EXCLUDED.payload, active = TRUE, updated_at = CURRENT_TIMESTAMP`, query, string(raw))
	return err
}

// LoadSponsoredMessages reads the deployment-owned messages constructor for
// one canonical InputPeer key. Missing rows are not converted into empty ads.
func LoadSponsoredMessages(ctx context.Context, peerKey string) (*mtproto.Messages_SponsoredMessages, bool, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, false, ErrSponsoredInventoryUnavailable
	}
	var raw []byte
	err := store.db.QueryRowContext(ctx, `SELECT payload FROM apifull_sponsored_inventory
		WHERE inventory_type = 'messages' AND peer_key = $1 AND active
		AND (starts_at IS NULL OR starts_at <= CURRENT_TIMESTAMP)
		AND (ends_at IS NULL OR ends_at > CURRENT_TIMESTAMP)
		ORDER BY id DESC LIMIT 1`, peerKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var result mtproto.Messages_SponsoredMessages
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, &result) != nil {
		return nil, true, ErrSponsoredInventoryInvalid
	}
	if result.GetPredicateName() != "messages_sponsoredMessages" && result.GetPredicateName() != "messages_sponsoredMessagesEmpty" {
		return nil, true, ErrSponsoredInventoryInvalid
	}
	return &result, true, nil
}

// StoreSponsoredMessages provisions the deployment-owned messages inventory.
func StoreSponsoredMessages(ctx context.Context, peerKey string, messages *mtproto.Messages_SponsoredMessages) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return ErrSponsoredInventoryUnavailable
	}
	if messages == nil || (messages.GetPredicateName() != "messages_sponsoredMessages" && messages.GetPredicateName() != "messages_sponsoredMessagesEmpty") {
		return ErrSponsoredInventoryInvalid
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO apifull_sponsored_inventory (inventory_type, peer_key, payload)
		VALUES ('messages', $1, $2::jsonb)
		ON CONFLICT (inventory_type, peer_key) WHERE inventory_type = 'messages'
		DO UPDATE SET payload = EXCLUDED.payload, active = TRUE, updated_at = CURRENT_TIMESTAMP`, peerKey, string(raw))
	return err
}

func DeleteSponsoredInventory(ctx context.Context, query, peerKey string) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return ErrSponsoredInventoryUnavailable
	}
	_, err := store.db.ExecContext(ctx, `DELETE FROM apifull_sponsored_inventory
		WHERE (inventory_type = 'peers' AND query = $1)
		   OR (inventory_type = 'messages' AND peer_key = $2)`, query, peerKey)
	return err
}
