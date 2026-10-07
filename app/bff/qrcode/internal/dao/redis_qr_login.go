// Copyright 2022 Teamgram Authors
//  All rights reserved.
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
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// qrCodeTimeout     int64 = 30 // salt timeout
	cacheQRCodePrefix = "qr_codes"
)

// The code hash changes atomically with immutable QR metadata, including except_ids.
// Comparing it prevents accepting a stale transaction after a QR refresh.
const acceptQRCodeScript = `
if redis.call('HGET', KEYS[1], 'code_hash') ~= ARGV[1] then return -1 end
local expire_at = tonumber(redis.call('HGET', KEYS[1], 'expire_at') or '0')
if expire_at < tonumber(ARGV[2]) then return -2 end
local state = redis.call('HGET', KEYS[1], 'state')
local user_id = redis.call('HGET', KEYS[1], 'user_id') or '0'
if state == ARGV[3] then
  redis.call('HSET', KEYS[1], 'user_id', ARGV[4], 'state', ARGV[5])
  return 1
end
if (state == ARGV[5] or state == ARGV[6]) and user_id == ARGV[4] then return 2 end
if state == ARGV[5] or state == ARGV[6] then return 0 end
return -1
`

// acceptQRCodeAtDCScript is the multi-DC variant of acceptQRCodeScript. The
// scanner DC is written in the same compare-and-set as the state transition,
// so a follow-up export cannot observe a partially moved transaction.
const acceptQRCodeAtDCScript = `
if redis.call('HGET', KEYS[1], 'code_hash') ~= ARGV[1] then return -1 end
local expire_at = tonumber(redis.call('HGET', KEYS[1], 'expire_at') or '0')
if expire_at < tonumber(ARGV[2]) then return -2 end
local state = redis.call('HGET', KEYS[1], 'state')
local user_id = redis.call('HGET', KEYS[1], 'user_id') or '0'
if state == ARGV[3] then
  redis.call('HSET', KEYS[1], 'user_id', ARGV[4], 'state', ARGV[5], 'dc_id', ARGV[7])
  return 1
end
if (state == ARGV[5] or state == ARGV[6]) and user_id == ARGV[4] then
  -- Accepted rows created before multi-DC ownership was introduced have no
  -- dc_id. Backfill that field during the idempotent retry, but never replace
  -- an existing owner selected by the first claim.
  local dc_id = redis.call('HGET', KEYS[1], 'dc_id') or '0'
  if dc_id == '0' then redis.call('HSET', KEYS[1], 'dc_id', ARGV[7]) end
  return 2
end
if state == ARGV[5] or state == ARGV[6] then return 0 end
return -1
`

const commitQRCodeScript = `
if redis.call('HGET', KEYS[1], 'code_hash') ~= ARGV[1] then return -1 end
local state = redis.call('HGET', KEYS[1], 'state')
local user_id = redis.call('HGET', KEYS[1], 'user_id') or '0'
if user_id ~= ARGV[2] then return 0 end
if state == ARGV[3] then
  redis.call('HSET', KEYS[1], 'state', ARGV[4])
  return 1
end
if state == ARGV[4] then return 2 end
return -1
`

const createQRCodeScript = `
if redis.call('EXISTS', KEYS[1]) ~= 0 then return 0 end
redis.call('HSET', KEYS[1], unpack(ARGV, 2))
local ttl = tonumber(ARGV[1])
if ttl and ttl > 0 then redis.call('EXPIRE', KEYS[1], ttl) end
return 1
`

const rotateQRCodeScript = `
if redis.call('HGET', KEYS[1], 'code_hash') ~= ARGV[1] then return -1 end
if redis.call('HGET', KEYS[1], 'state') ~= ARGV[2] then return 0 end
redis.call('HSET', KEYS[1], unpack(ARGV, 4))
local ttl = tonumber(ARGV[3])
if ttl and ttl > 0 then redis.call('EXPIRE', KEYS[1], ttl) end
return 1
`

func genQRLoginCodeKey(authKeyId int64) string {
	return fmt.Sprintf("%s_%d", cacheQRCodePrefix, authKeyId)
}

func (d *Dao) GetCacheQRLoginCode(ctx context.Context, keyId int64) (code *model.QRCodeTransaction, err error) {
	var (
		key    = genQRLoginCodeKey(keyId)
		values map[string]string
	)

	values, err = d.kv.HgetallCtx(ctx, key)
	if err != nil {
		logx.WithContext(ctx).Errorf("conn.Do(HGETALL %s) error(%v)", key, err)
		return nil, err
	} else if len(values) == 0 {
		return
	}

	code = new(model.QRCodeTransaction)
	for k, v := range values {
		switch k {
		case "perm_auth_key_id":
			code.PermAuthKeyId, _ = strconv.ParseInt(v, 10, 64)
		case "dc_id":
			value, _ := strconv.ParseInt(v, 10, 32)
			code.DcId = int32(value)
		case "session_id":
			code.SessionId, _ = strconv.ParseInt(v, 10, 64)
		case "auth_key_id":
			code.AuthKeyId, _ = strconv.ParseInt(v, 10, 64)
		case "server_id":
			code.ServerId = v
		case "api_id":
			v, _ := strconv.ParseInt(v, 10, 64)
			code.ApiId = int32(v)
		case "api_hash":
			code.ApiHash = v
		case "except_ids":
			if err = json.Unmarshal([]byte(v), &code.ExceptIDs); err != nil {
				return nil, fmt.Errorf("decode qr code except_ids: %w", err)
			}
		case "code_hash":
			code.CodeHash = v
		case "expire_at":
			code.ExpireAt, _ = strconv.ParseInt(v, 10, 64)
		case "user_id":
			code.UserId, _ = strconv.ParseInt(v, 10, 64)
		case "state":
			v, _ := strconv.ParseInt(v, 10, 64)
			code.State = int(v)
		}
	}

	return
}

func qrCodeCacheFields(qrCode *model.QRCodeTransaction) (map[string]string, error) {
	exceptIDs, err := json.Marshal(qrCode.ExceptIDs)
	if err != nil {
		return nil, fmt.Errorf("encode qr code except_ids: %w", err)
	}

	return map[string]string{
		"perm_auth_key_id": strconv.FormatInt(qrCode.PermAuthKeyId, 10),
		"dc_id":            strconv.Itoa(int(qrCode.DcId)),
		"session_id":       strconv.FormatInt(qrCode.SessionId, 10),
		"auth_key_id":      strconv.FormatInt(qrCode.AuthKeyId, 10),
		"server_id":        qrCode.ServerId,
		"api_id":           strconv.Itoa(int(qrCode.ApiId)),
		"api_hash":         qrCode.ApiHash,
		"except_ids":       string(exceptIDs),
		"code_hash":        qrCode.CodeHash,
		"expire_at":        strconv.FormatInt(qrCode.ExpireAt, 10),
		"state":            strconv.Itoa(qrCode.State),
		"user_id":          strconv.FormatInt(qrCode.UserId, 10),
	}, nil
}

func qrCodeWriteArgs(fields map[string]string) []any {
	args := make([]any, 0, len(fields)*2)
	for _, field := range []string{
		"perm_auth_key_id", "dc_id", "session_id", "auth_key_id", "server_id", "api_id", "api_hash",
		"except_ids", "code_hash", "expire_at", "state", "user_id",
	} {
		args = append(args, field, fields[field])
	}
	return args
}

func qrCodeCacheTTL(expiredIn int) int {
	if expiredIn <= 0 {
		return 0
	}
	// Keep cache entries for two seconds beyond the requested token lifetime.
	return expiredIn + 2
}

func (d *Dao) CreateCacheQRLoginCode(ctx context.Context, keyId int64, qrCode *model.QRCodeTransaction, expiredIn int) (int64, error) {
	fields, err := qrCodeCacheFields(qrCode)
	if err != nil {
		return 0, err
	}
	args := append([]any{strconv.Itoa(qrCodeCacheTTL(expiredIn))}, qrCodeWriteArgs(fields)...)
	return d.evalQRCodeResult(ctx, createQRCodeScript, genQRLoginCodeKey(keyId), args...)
}

// RotateCacheQRLoginCode replaces a New token only if its generation is still current.
func (d *Dao) RotateCacheQRLoginCode(ctx context.Context, keyId int64, oldCodeHash string, qrCode *model.QRCodeTransaction, expiredIn int) (int64, error) {
	fields, err := qrCodeCacheFields(qrCode)
	if err != nil {
		return 0, err
	}
	args := append([]any{oldCodeHash, strconv.Itoa(model.QRCodeStateNew), strconv.Itoa(qrCodeCacheTTL(expiredIn))}, qrCodeWriteArgs(fields)...)
	return d.evalQRCodeResult(ctx, rotateQRCodeScript, genQRLoginCodeKey(keyId), args...)
}

// AcceptCacheQRLoginCode atomically moves one unexpired token generation from
// New to Accepted. The same user may resume an Accepted/Success generation.
func (d *Dao) AcceptCacheQRLoginCode(ctx context.Context, keyId int64, codeHash string, userID int64, now int64) (int64, error) {
	return d.evalQRCodeResult(
		ctx, acceptQRCodeScript, genQRLoginCodeKey(keyId),
		codeHash,
		strconv.FormatInt(now, 10),
		strconv.Itoa(model.QRCodeStateNew),
		strconv.FormatInt(userID, 10),
		strconv.Itoa(model.QRCodeStateAccepted),
		strconv.Itoa(model.QRCodeStateSuccess),
	)
}

// AcceptCacheQRLoginCodeAtDC atomically claims a token and records the DC of
// the authorized scanner. The exported account can then migrate its pending
// login session to that DC on the follow-up export call.
func (d *Dao) AcceptCacheQRLoginCodeAtDC(ctx context.Context, keyId int64, codeHash string, userID int64, now int64, dcID int32) (int64, error) {
	if dcID <= 0 {
		return 0, fmt.Errorf("invalid qr dc id %d", dcID)
	}
	return d.evalQRCodeResult(
		ctx, acceptQRCodeAtDCScript, genQRLoginCodeKey(keyId),
		codeHash,
		strconv.FormatInt(now, 10),
		strconv.Itoa(model.QRCodeStateNew),
		strconv.FormatInt(userID, 10),
		strconv.Itoa(model.QRCodeStateAccepted),
		strconv.Itoa(model.QRCodeStateSuccess),
		strconv.Itoa(int(dcID)),
	)
}

// CommitCacheQRLoginCode marks a claim successful only after authsession
// confirms that the QR auth key belongs to the claimed user.
func (d *Dao) CommitCacheQRLoginCode(ctx context.Context, keyId int64, codeHash string, userID int64) (int64, error) {
	return d.evalQRCodeResult(
		ctx, commitQRCodeScript, genQRLoginCodeKey(keyId),
		codeHash,
		strconv.FormatInt(userID, 10),
		strconv.Itoa(model.QRCodeStateAccepted),
		strconv.Itoa(model.QRCodeStateSuccess),
	)
}

func (d *Dao) evalQRCodeResult(ctx context.Context, script, key string, args ...any) (int64, error) {
	result, err := d.kv.EvalCtx(ctx, script, key, args...)
	if err != nil {
		return 0, err
	}
	switch n := result.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("unexpected qr accept result %T", result)
	}
}

func (d *Dao) DeleteCacheQRLoginCode(ctx context.Context, authKeyId int64) (err error) {
	key := genQRLoginCodeKey(authKeyId)

	if _, err = d.kv.DelCtx(ctx, key); err != nil {
		logx.WithContext(ctx).Errorf("conn.DEL(%s) error(%v)", key, err)
	}

	return
}
