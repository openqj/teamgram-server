// Copyright 2026 Teamgram Authors
// All rights reserved.

package dao

import (
	"context"
	"fmt"
	"strconv"

	"github.com/zeromicro/go-zero/core/hash"
)

const (
	ExportedAuthorizationTokenSize    = 32
	exportedAuthorizationTokenTimeout = 5 * 60
	exportedAuthorizationTokenPrefix  = "exported_authorization_"
	ExportedAuthorizationReady        = 1
	ExportedAuthorizationBinding      = 2
	ExportedAuthorizationComplete     = 3
)

const putExportedAuthorizationScript = `
if redis.call('EXISTS', KEYS[1]) ~= 0 then return 0 end
redis.call('HSET', KEYS[1],
  'id', ARGV[1],
  'user_id', ARGV[2],
  'source_auth_key_id', ARGV[3],
  'source_dc_id', ARGV[4],
  'target_dc_id', ARGV[5],
  'target_auth_key_id', '0',
  'state', ARGV[6])
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[7]))
return 1
`

const claimExportedAuthorizationScript = `
if redis.call('EXISTS', KEYS[1]) == 0 then return -1 end
if redis.call('HGET', KEYS[1], 'target_dc_id') ~= ARGV[2] then return -2 end
local state = redis.call('HGET', KEYS[1], 'state')
local target_auth_key_id = redis.call('HGET', KEYS[1], 'target_auth_key_id') or '0'
if state == ARGV[3] then
  redis.call('HSET', KEYS[1], 'state', ARGV[4], 'target_auth_key_id', ARGV[1])
  return 1
end
if state == ARGV[4] and target_auth_key_id == ARGV[1] then return 2 end
return 0
`

const completeExportedAuthorizationScript = `
if redis.call('EXISTS', KEYS[1]) == 0 then return -1 end
local state = redis.call('HGET', KEYS[1], 'state')
local target_auth_key_id = redis.call('HGET', KEYS[1], 'target_auth_key_id') or '0'
if state == ARGV[2] and target_auth_key_id == ARGV[1] then
  redis.call('HSET', KEYS[1], 'state', ARGV[3])
  return 1
end
if state == ARGV[3] and target_auth_key_id == ARGV[1] then return 2 end
return 0
`

type ExportedAuthorization struct {
	ID              int64 `json:"id"`
	UserID          int64 `json:"user_id"`
	SourceAuthKeyID int64 `json:"source_auth_key_id"`
	SourceDCID      int32 `json:"source_dc_id"`
	TargetDCID      int32 `json:"target_dc_id"`
	TargetAuthKeyID int64 `json:"target_auth_key_id"`
	State           int   `json:"state"`
}

func exportedAuthorizationKey(token []byte) string {
	return exportedAuthorizationTokenPrefix + hash.Md5Hex(token)
}

func (d *Dao) PutExportedAuthorization(ctx context.Context, token []byte, authorization *ExportedAuthorization) error {
	if len(token) != ExportedAuthorizationTokenSize || !validExportedAuthorization(authorization) || authorization.State != ExportedAuthorizationReady || authorization.TargetAuthKeyID != 0 {
		return fmt.Errorf("invalid exported authorization")
	}

	result, err := d.kv.EvalCtx(
		ctx, putExportedAuthorizationScript, exportedAuthorizationKey(token),
		strconv.FormatInt(authorization.ID, 10),
		strconv.FormatInt(authorization.UserID, 10),
		strconv.FormatInt(authorization.SourceAuthKeyID, 10),
		strconv.Itoa(int(authorization.SourceDCID)),
		strconv.Itoa(int(authorization.TargetDCID)),
		strconv.Itoa(authorization.State),
		strconv.Itoa(exportedAuthorizationTokenTimeout),
	)
	if err != nil {
		return err
	}
	created, ok := redisInteger(result)
	if !ok {
		return fmt.Errorf("unexpected exported authorization put result %T", result)
	}
	if created != 1 {
		return fmt.Errorf("exported authorization token collision")
	}
	return nil
}

// GetExportedAuthorization reads a credential without consuming it so callers
// can verify the source authorization before its one-time use is spent.
func (d *Dao) GetExportedAuthorization(ctx context.Context, token []byte) (*ExportedAuthorization, error) {
	if len(token) != ExportedAuthorizationTokenSize {
		return nil, nil
	}

	fields, err := d.kv.HgetallCtx(ctx, exportedAuthorizationKey(token))
	if err != nil {
		return nil, err
	}
	return parseExportedAuthorization(fields)
}

// ClaimExportedAuthorization binds a transfer to one target auth key. A
// Binding claim is recoverable only by that same target.
func (d *Dao) ClaimExportedAuthorization(ctx context.Context, token []byte, targetAuthKeyID int64, targetDCID int32) (*ExportedAuthorization, error) {
	if len(token) != ExportedAuthorizationTokenSize {
		return nil, nil
	}

	value, err := d.kv.EvalCtx(
		ctx, claimExportedAuthorizationScript, exportedAuthorizationKey(token),
		targetAuthKeyID,
		targetDCID,
		ExportedAuthorizationReady,
		ExportedAuthorizationBinding,
	)
	if err != nil {
		return nil, err
	}
	claimed, ok := redisInteger(value)
	if !ok {
		return nil, fmt.Errorf("unexpected exported authorization claim result %T", value)
	}
	if claimed != 1 && claimed != 2 {
		return nil, nil
	}
	return d.GetExportedAuthorization(ctx, token)
}

// CompleteExportedAuthorization marks a claim consumed after authsession
// confirms the target auth-key ownership.
func (d *Dao) CompleteExportedAuthorization(ctx context.Context, token []byte, targetAuthKeyID int64) (bool, error) {
	if len(token) != ExportedAuthorizationTokenSize {
		return false, nil
	}
	value, err := d.kv.EvalCtx(
		ctx, completeExportedAuthorizationScript, exportedAuthorizationKey(token),
		targetAuthKeyID,
		ExportedAuthorizationBinding,
		ExportedAuthorizationComplete,
	)
	if err != nil {
		return false, err
	}
	result, ok := redisInteger(value)
	if !ok {
		return false, fmt.Errorf("unexpected exported authorization complete result %T", value)
	}
	return result == 1 || result == 2, nil
}

func parseExportedAuthorization(fields map[string]string) (*ExportedAuthorization, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	parseInt64 := func(field string) (int64, error) {
		value, err := strconv.ParseInt(fields[field], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid exported authorization %s: %w", field, err)
		}
		return value, nil
	}
	id, err := parseInt64("id")
	if err != nil {
		return nil, err
	}
	userID, err := parseInt64("user_id")
	if err != nil {
		return nil, err
	}
	sourceAuthKeyID, err := parseInt64("source_auth_key_id")
	if err != nil {
		return nil, err
	}
	targetAuthKeyID, err := parseInt64("target_auth_key_id")
	if err != nil {
		return nil, err
	}
	sourceDCID, err := strconv.ParseInt(fields["source_dc_id"], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid exported authorization source_dc_id: %w", err)
	}
	targetDCID, err := strconv.ParseInt(fields["target_dc_id"], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid exported authorization target_dc_id: %w", err)
	}
	state, err := strconv.Atoi(fields["state"])
	if err != nil {
		return nil, fmt.Errorf("invalid exported authorization state: %w", err)
	}
	authorization := &ExportedAuthorization{
		ID:              id,
		UserID:          userID,
		SourceAuthKeyID: sourceAuthKeyID,
		SourceDCID:      int32(sourceDCID),
		TargetDCID:      int32(targetDCID),
		TargetAuthKeyID: targetAuthKeyID,
		State:           state,
	}
	if !validExportedAuthorization(authorization) {
		return nil, fmt.Errorf("invalid exported authorization")
	}

	return authorization, nil
}

func redisInteger(value any) (int64, bool) {
	switch result := value.(type) {
	case int64:
		return result, true
	case int:
		return int64(result), true
	default:
		return 0, false
	}
}

func validExportedAuthorization(authorization *ExportedAuthorization) bool {
	if authorization == nil || authorization.ID <= 0 || authorization.UserID <= 0 || authorization.SourceAuthKeyID == 0 || authorization.SourceDCID <= 0 || authorization.TargetDCID <= 0 {
		return false
	}
	switch authorization.State {
	case ExportedAuthorizationReady:
		return authorization.TargetAuthKeyID == 0
	case ExportedAuthorizationBinding, ExportedAuthorizationComplete:
		return authorization.TargetAuthKeyID != 0
	default:
		return false
	}
}
