package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// loadLocalBusinessResult reads a result published by the PostgreSQL business
// control plane. A missing row is deliberately different from a malformed
// row: the former lets a configured external provider remain usable, while
// the latter fails closed instead of exposing corrupted TL data.
func (c *ApiFullCore) loadLocalBusinessResult(operation, scopeKey string, result any) (bool, error) {
	return c.loadLocalBusinessResultWithScope(operation, scopeKey, result, false)
}

// loadLocalBusinessResultForCaller is for results whose meaning is bound to
// the authenticated user. A published global row is valid for catalogues but
// cannot authorize a callback, WebView response, mutation, or update.
func (c *ApiFullCore) loadLocalBusinessResultForCaller(operation, scopeKey string, result any) (bool, error) {
	return c.loadLocalBusinessResultWithScope(operation, scopeKey, result, true)
}

func (c *ApiFullCore) loadLocalBusinessResultWithScope(operation, scopeKey string, result any, callerOnly bool) (bool, error) {
	if !persist.PostgresEnabled() {
		return false, nil
	}
	if c == nil || c.MD == nil || c.MD.GetUserId() <= 0 || strings.TrimSpace(operation) == "" || strings.TrimSpace(scopeKey) == "" || result == nil {
		return false, mtproto.ErrMethodNotImpl
	}
	var raw []byte
	var found bool
	var err error
	if callerOnly {
		raw, found, err = persist.LoadBusinessControlResultForCaller(operation, scopeKey, c.MD.GetUserId())
	} else {
		raw, found, err = persist.LoadBusinessControlResult(operation, scopeKey, c.MD.GetUserId())
	}
	if err != nil {
		if err == persist.ErrBusinessControlPlaneUnavailable {
			return false, mtproto.ErrMethodNotImpl
		}
		return false, mtproto.ErrInternalServerError
	}
	if !found {
		return false, nil
	}
	if err = json.Unmarshal(raw, result); err != nil {
		return true, mtproto.ErrInternalServerError
	}
	return true, nil
}

func businessControlHash(prefix string, value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return prefix + ":invalid"
	}
	digest := sha256.Sum256(raw)
	return prefix + ":sha256:" + hex.EncodeToString(digest[:])
}

func businessControlPeerKey(userID int64, peer *mtproto.InputPeer) string {
	peerType, peerID := apifullPeerTypeID(userID, peer)
	if peerType == 0 || peerID == 0 {
		return "peer:invalid"
	}
	return fmt.Sprintf("peer:%d:%d", peerType, peerID)
}

func businessControlQueryKey(queryID int64) string {
	return "query:" + strconv.FormatInt(queryID, 10)
}
