// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package dao

import (
	"context"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/hash"
)

const (
	futureAuthTokenTimeout = 7 * 60 * 60 * 24 // salt timeout
	futureAuthTokenPrefix  = "future_auth_token_"
)

func (d *Dao) PutFutureAuthToken(ctx context.Context, futureAuthToken []byte, userID int64) error {
	digest := hash.Md5Hex(futureAuthToken)
	k := futureAuthTokenPrefix + digest
	// Keep the bounded token index and token write in one Redis script. The
	// index stores digests only, so eviction never exposes token material.
	if eval, ok := d.kv.(interface {
		EvalCtx(context.Context, string, string, ...any) (any, error)
	}); ok {
		_, err := eval.EvalCtx(ctx, `
			local index = ARGV[4]
			redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
			redis.call('ZADD', index, ARGV[3], ARGV[5])
			local count = redis.call('ZCARD', index)
			if count > 20 then
				local stale = redis.call('ZRANGE', index, 0, count - 21)
				for _, digest in ipairs(stale) do
					redis.call('DEL', 'future_auth_token_' .. digest)
					redis.call('ZREM', index, digest)
				end
			end
			return 1
		`, k, strconv.FormatInt(userID, 10), futureAuthTokenTimeout, strconv.FormatInt(time.Now().Unix(), 10), futureAuthTokenIndex(userID), digest)
		return err
	}
	// Keep lightweight test stores and non-Redis adapters compatible. The
	// production go-zero KV implementation always exposes EvalCtx.
	return d.kv.SetexCtx(ctx, k, strconv.FormatInt(userID, 10), futureAuthTokenTimeout)
}

func futureAuthTokenIndex(userID int64) string {
	return "future_auth_tokens_user_" + strconv.FormatInt(userID, 10)
}

func (d *Dao) GetFutureAuthToken(ctx context.Context, futureAuthToken []byte) (int64, error) {
	k := futureAuthTokenPrefix + hash.Md5Hex(futureAuthToken)
	rV, err := d.kv.GetCtx(ctx, k)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(rV, 10, 64)
}

func (d *Dao) DelFutureAuthToken(ctx context.Context, futureAuthToken []byte) error {
	digest := hash.Md5Hex(futureAuthToken)
	k := futureAuthTokenPrefix + digest
	if eval, ok := d.kv.(interface {
		EvalCtx(context.Context, string, string, ...any) (any, error)
	}); ok {
		_, err := eval.EvalCtx(ctx, `
			local userID = redis.call('GET', KEYS[1])
			if userID then
				redis.call('DEL', KEYS[1])
				redis.call('ZREM', ARGV[1] .. userID, ARGV[2])
			end
			return 1
		`, k, "future_auth_tokens_user_", digest)
		return err
	}
	_, err := d.kv.DelCtx(ctx, k)
	return err
}
