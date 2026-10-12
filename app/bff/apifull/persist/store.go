package persist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/teamgram/proto/mtproto"
	internal "github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

var (
	ErrSponsoredChannelMissing       = internal.ErrSponsoredChannelMissing
	ErrSponsoredChannelAccessHash    = internal.ErrSponsoredChannelAccessHash
	ErrSponsoredChannelUnauthorized  = internal.ErrSponsoredChannelUnauthorized
	ErrSponsoredInventoryInvalid     = internal.ErrSponsoredInventoryInvalid
	ErrSponsoredInventoryUnavailable = internal.ErrSponsoredInventoryUnavailable
	ErrBotPaymentQueryNotFound       = internal.ErrBotPaymentQueryNotFound
	ErrBotPaymentQueryConflict       = internal.ErrBotPaymentQueryConflict
	ErrBotPaymentQueryExpired        = internal.ErrBotPaymentQueryExpired
	ErrBotPaymentQueryOwner          = internal.ErrBotPaymentQueryOwner
	ErrBotPaymentQueryState          = internal.ErrBotPaymentQueryState
)

type BotPaymentQuery = internal.BotPaymentQuery

// BotCustomVerification is the legacy record shape used by negative tests and
// migration inspection. Production bots.setCustomVerification no longer
// writes this local KV because it is not an authoritative verifier source.
type BotCustomVerification struct {
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

const (
	BotPaymentStageShippingPending     = internal.BotPaymentStageShippingPending
	BotPaymentStageShippingAnswered    = internal.BotPaymentStageShippingAnswered
	BotPaymentStagePrecheckoutPending  = internal.BotPaymentStagePrecheckoutPending
	BotPaymentStagePrecheckoutAnswered = internal.BotPaymentStagePrecheckoutAnswered
)

// Store is the process-wide blob store shared with apifull.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Default follows the apifull store, including a later Use or database open.
var Default Store = live{}

// OpenPostgres connects the shared process store to the deployment-owned
// PostgreSQL 18 APIFull schema. BFFs that keep authentication state in this
// store call it during startup so password and recovery state survives
// process restarts.
func OpenPostgres(dsn string) error {
	return internal.OpenPostgres(dsn)
}

// OpenPostgresReadOnly connects to an already-migrated APIFull schema without
// issuing DDL. Production services should use this startup gate.
func OpenPostgresReadOnly(dsn string) error {
	return internal.OpenPostgresReadOnly(dsn)
}

// OpenPostgresRequired validates and opens the shared PostgreSQL 18 store for
// a BFF service that owns handlers backed by apifull persistence.
func OpenPostgresRequired(service, dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("%s: PostgresDSN is required", service)
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		return fmt.Errorf("%s: open PostgreSQL store: %w", service, err)
	}
	return nil
}

// PostgresEnabled reports whether the shared process store is backed by the
// deployment-owned PostgreSQL schema. Standalone BFF services use this as a
// startup invariant so they cannot silently fall back to memory.
func PostgresEnabled() bool {
	return internal.PostgresEnabled()
}

// ClosePostgres releases the shared process store during standalone service
// shutdown. The monolithic BFF uses the same hook from its top-level server.
func ClosePostgres() error {
	return internal.ClosePostgres()
}

// SaveAppLogEvents appends help.saveAppLog events to the shared PostgreSQL
// store. The batch is committed atomically by the internal persistence layer.
func SaveAppLogEvents(ctx context.Context, userID int64, events []*mtproto.InputAppEvent) error {
	return internal.SaveAppLogEvents(ctx, userID, events)
}

func SetSponsoredMessagesEnabled(ctx context.Context, userID int64, enabled bool) error {
	return internal.SetSponsoredMessagesEnabled(ctx, userID, enabled)
}

func GetSponsoredMessagesEnabled(ctx context.Context, userID int64) (enabled, found bool, err error) {
	return internal.GetSponsoredMessagesEnabled(ctx, userID)
}

func RecordSponsoredEvent(ctx context.Context, userID int64, eventType string, randomID []byte, payload any) error {
	return internal.RecordSponsoredEvent(ctx, userID, eventType, randomID, payload)
}

func SetSponsoredChannelRestriction(ctx context.Context, actorID, channelID, accessHash int64, restricted bool) error {
	return internal.SetSponsoredChannelRestriction(ctx, actorID, channelID, accessHash, restricted)
}

func LoadPremiumPromo(ctx context.Context) (*mtproto.Help_PremiumPromo, bool, error) {
	return internal.LoadPremiumPromo(ctx)
}

func StorePremiumPromo(ctx context.Context, promo *mtproto.Help_PremiumPromo) error {
	return internal.StorePremiumPromo(ctx, promo)
}

func DeletePremiumPromo(ctx context.Context) error {
	return internal.DeletePremiumPromo(ctx)
}

func CreateBotPaymentQuery(query BotPaymentQuery) error {
	return internal.CreateBotPaymentQuery(query)
}

func AdvanceBotPaymentQuery(queryID, botUserID int64, stage string) error {
	return internal.AdvanceBotPaymentQuery(queryID, botUserID, stage)
}

func SetBotShippingResults(botUserID, queryID int64, errorText string, options []byte) error {
	return internal.SetBotShippingResults(botUserID, queryID, errorText, options)
}

func SetBotPrecheckoutResults(botUserID, queryID int64, success bool, errorText string) error {
	return internal.SetBotPrecheckoutResults(botUserID, queryID, success, errorText)
}

func LoadBotPaymentQuery(queryID int64) (BotPaymentQuery, bool, error) {
	return internal.LoadBotPaymentQuery(queryID)
}

// BotCustomVerificationKey identifies legacy records so migration checks can
// prove that an untrusted local row is never exposed as Layer 229 data.
func BotCustomVerificationKey(botID int64, peerType int32, peerID int64) string {
	return fmt.Sprintf("bot-custom-verification:%d:%d:%d", botID, peerType, peerID)
}

// LoadBotCustomVerification reads a legacy bot/peer record for migration
// inspection. No production read path should use it as verifier authority.
func LoadBotCustomVerification(botID int64, peerType int32, peerID int64) (*BotCustomVerification, bool, error) {
	if botID <= 0 || peerID <= 0 {
		return nil, false, fmt.Errorf("apifull: invalid bot custom verification scope")
	}
	raw, err := Default.Get(BotCustomVerificationKey(botID, peerType, peerID))
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	var state BotCustomVerification
	if err = json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, false, fmt.Errorf("apifull: decode bot custom verification: %w", err)
	}
	if state.UpdatedAt <= 0 || len(state.Description) > 512 {
		return nil, false, fmt.Errorf("apifull: invalid bot custom verification state")
	}
	return &state, true, nil
}

func LoadSponsoredPeers(ctx context.Context, query string) (*mtproto.Contacts_SponsoredPeers, bool, error) {
	return internal.LoadSponsoredPeers(ctx, query)
}

func StoreSponsoredPeers(ctx context.Context, query string, peers *mtproto.Contacts_SponsoredPeers) error {
	return internal.StoreSponsoredPeers(ctx, query, peers)
}

func LoadSponsoredMessages(ctx context.Context, peerKey string) (*mtproto.Messages_SponsoredMessages, bool, error) {
	return internal.LoadSponsoredMessages(ctx, peerKey)
}

func StoreSponsoredMessages(ctx context.Context, peerKey string, messages *mtproto.Messages_SponsoredMessages) error {
	return internal.StoreSponsoredMessages(ctx, peerKey, messages)
}

func DeleteSponsoredInventory(ctx context.Context, query, peerKey string) error {
	return internal.DeleteSponsoredInventory(ctx, query, peerKey)
}

type live struct{}

func (live) Get(key string) (string, error) { return internal.Default.Get(key) }
func (live) Set(key, value string) error    { return internal.Default.Set(key, value) }
func (live) CompareAndSwap(key, expected, replacement string) (bool, error) {
	return internal.CompareAndSwap(key, expected, replacement)
}
func (live) Update(key string, fn func(string) (string, error)) error {
	return internal.Update(key, fn)
}

// CompareAndSwap delegates to the shared APIFull store and is used for
// state transitions that must not lose a concurrent update.
func CompareAndSwap(key, expected, replacement string) (bool, error) {
	if s, ok := Default.(interface {
		CompareAndSwap(string, string, string) (bool, error)
	}); ok {
		return s.CompareAndSwap(key, expected, replacement)
	}
	current, err := Default.Get(key)
	if err != nil {
		return false, err
	}
	if current != expected {
		return false, nil
	}
	return true, Default.Set(key, replacement)
}

// Update executes a read/modify/write operation under the store's transaction
// boundary. PostgreSQL uses its row lock and transaction; test stores fall
// back to the compare-and-swap implementation.
func Update(key string, fn func(string) (string, error)) error {
	if s, ok := Default.(interface {
		Update(string, func(string) (string, error)) error
	}); ok {
		return s.Update(key, fn)
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := Default.Get(key)
		if err != nil {
			return err
		}
		next, err := fn(current)
		if err != nil {
			return err
		}
		ok, err := CompareAndSwap(key, current, next)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return errors.New("apifull: concurrent state update failed")
}
