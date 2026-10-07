// Copyright 2025 Teamgram Authors
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

package core

import (
	"fmt"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const searchPostsFloodKeyPrefix = "channels:search_posts_flood"

// searchPostsFloodScript increments a user's daily counter and sets the
// expiry only when the counter is created. Keeping both operations in Redis
// makes the quota atomic across all BFF instances.
const searchPostsFloodScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`

func searchPostsFloodKey(userID int64, now time.Time) string {
	return fmt.Sprintf("%s:%d:%s", searchPostsFloodKeyPrefix, userID, now.UTC().Format("20060102"))
}

func searchPostsFloodWindow(now time.Time) int {
	nextDay := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	seconds := int(time.Until(nextDay).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

func searchPostsFloodCount(c *MessagesCore, key string, expiresIn int) (int64, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.KV == nil {
		return 0, mtproto.ErrInternalServerError
	}
	value, err := c.svcCtx.Dao.KV.EvalCtx(c.ctx, searchPostsFloodScript, key, expiresIn)
	if err != nil {
		return 0, err
	}
	count, ok := redisInteger(value)
	if !ok || count < 1 {
		return 0, fmt.Errorf("invalid search-posts flood counter response: %T", value)
	}
	return count, nil
}

func redisInteger(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case uint64:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// ChannelsCheckSearchPostsFlood
// channels.checkSearchPostsFlood#22567115 flags:# query:flags.0?string = SearchPostsFlood;
func (c *MessagesCore) ChannelsCheckSearchPostsFlood(in *mtproto.TLChannelsCheckSearchPostsFlood) (*mtproto.SearchPostsFlood, error) {
	_ = in
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil {
		return nil, mtproto.ErrInternalServerError
	}
	settings := c.svcCtx.Config.SearchPostsFlood
	if settings.TotalDaily <= 0 {
		return nil, mtproto.ErrInternalServerError
	}
	now := time.Now().UTC()
	count, err := searchPostsFloodCount(c, searchPostsFloodKey(c.MD.UserId, now), searchPostsFloodWindow(now))
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("channels.checkSearchPostsFlood - error: %v", err)
		}
		return nil, err
	}
	remains := int64(settings.TotalDaily) - count
	if remains < 0 {
		remains = 0
	}
	queryIsFree := count <= int64(settings.TotalDaily)
	waitTill := int32(0)
	if !queryIsFree {
		waitTill = int32(now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour).Unix())
	}
	return mtproto.MakeTLSearchPostsFlood(&mtproto.SearchPostsFlood{
		QueryIsFree: queryIsFree,
		TotalDaily:  int32(settings.TotalDaily),
		Remains:     int32(remains),
		WaitTill:    wrapperspb.Int32(waitTill),
		StarsAmount: 0,
	}).To_SearchPostsFlood(), nil
}
