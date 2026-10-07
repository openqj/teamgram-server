package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

type searchPostsFloodKV struct {
	kv.Store
	count  int64
	script string
	key    string
	args   []any
}

func (s *searchPostsFloodKV) EvalCtx(_ context.Context, script, key string, args ...any) (any, error) {
	s.script, s.key, s.args = script, key, args
	if !strings.Contains(script, "INCR") || !strings.Contains(script, "EXPIRE") {
		return nil, fmt.Errorf("unexpected script")
	}
	s.count++
	return s.count, nil
}

func TestChannelsCheckSearchPostsFloodUsesAtomicDailyCounter(t *testing.T) {
	store := &searchPostsFloodKV{}
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{Config: config.Config{SearchPostsFlood: config.SearchPostsFloodConfig{TotalDaily: 2}}, Dao: &dao.Dao{KV: store}}}
	first, err := core.ChannelsCheckSearchPostsFlood(&mtproto.TLChannelsCheckSearchPostsFlood{})
	if err != nil || first == nil || first.GetRemains() != 1 || !first.GetQueryIsFree() {
		t.Fatalf("first=(%v,%v)", first, err)
	}
	second, err := core.ChannelsCheckSearchPostsFlood(&mtproto.TLChannelsCheckSearchPostsFlood{})
	if err != nil || second == nil || second.GetRemains() != 0 || !second.GetQueryIsFree() {
		t.Fatalf("second=(%v,%v)", second, err)
	}
	third, err := core.ChannelsCheckSearchPostsFlood(&mtproto.TLChannelsCheckSearchPostsFlood{})
	if err != nil || third == nil || third.GetRemains() != 0 || third.GetQueryIsFree() || int64(third.GetWaitTill().GetValue()) <= time.Now().Unix() {
		t.Fatalf("third=(%v,%v)", third, err)
	}
	if store.key != "channels:search_posts_flood:42:"+time.Now().UTC().Format("20060102") {
		t.Fatalf("key=%q", store.key)
	}
	if len(store.args) != 1 {
		t.Fatalf("args=%v", store.args)
	}
}

func TestChannelsCheckSearchPostsFloodFailsClosedWithoutConfiguredStore(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}, svcCtx: &svc.ServiceContext{Config: config.Config{SearchPostsFlood: config.SearchPostsFloodConfig{TotalDaily: 2}}, Dao: &dao.Dao{}}}
	if _, err := core.ChannelsCheckSearchPostsFlood(&mtproto.TLChannelsCheckSearchPostsFlood{}); err != mtproto.ErrInternalServerError {
		t.Fatalf("err=%v", err)
	}
}
