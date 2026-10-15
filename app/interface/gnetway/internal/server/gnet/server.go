// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
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

package gnet

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/teamgram/marmota/pkg/cache"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/codec"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/svc"

	"github.com/panjf2000/gnet/v2"
	"github.com/panjf2000/gnet/v2/pkg/logging"
	"github.com/panjf2000/gnet/v2/pkg/pool/goroutine"
	"github.com/zeromicro/go-zero/core/logx"
)

var (
	//etcdPrefix is a etcd globe key prefix
	endpoints string
)

type Server struct {
	gnet.BuiltinEventEngine
	eng            gnet.Engine
	pool           *goroutine.Pool
	admission      *connectionAdmission
	cache          *cache.LRUCache
	c              *config.Config
	handshake      *handshake
	authSessionMgr *authSessionManager
	svcCtx         *svc.ServiceContext
	writeMu        sync.Mutex
	writeQueues    map[int64]*gatewayWriteQueue
	draining       atomic.Bool
	closeOnce      sync.Once
	tickNumber     int64
	cachedNow      atomic.Int64 // cached unix timestamp, updated every OnTick
	timeoutWheel   *timeoutWheel
}

// CachedNow returns the cached unix timestamp (updated every OnTick interval).
// Falls back to time.Now() if not yet initialized.
func (s *Server) CachedNow() int64 {
	if n := s.cachedNow.Load(); n > 0 {
		return n
	}
	return time.Now().Unix()
}

func New(svcCtx *svc.ServiceContext, c config.Config) *Server {
	var (
		s = new(Server)
	)

	s.authSessionMgr = NewAuthSessionManager()
	s.timeoutWheel = newTimeoutWheel()
	s.writeQueues = make(map[int64]*gatewayWriteQueue)
	if c.Gnetway == nil {
		panic("gnetway: Gnetway config is required")
	}
	c.Gnetway.Normalize()
	s.admission = newConnectionAdmission(*c.Gnetway)

	s.handshake = mustNewHandshake(c.RSAKey)

	cacheSizeMB := c.Gnetway.AuthKeyCacheM
	if cacheSizeMB <= 0 {
		cacheSizeMB = 10
	}
	s.cache = cache.NewLRUCache(int64(cacheSizeMB) * 1024 * 1024)

	pool, err := ants.NewPool(c.Gnetway.WorkerPoolSize, ants.WithNonblocking(true))
	if err != nil {
		panic(err)
	}
	s.pool = pool

	s.c = &c
	s.svcCtx = svcCtx

	go func() {
		s.Serve()
	}()
	return s
}

// classifyCodecError maps codec errors to a stable reason label for metrics.
func classifyCodecError(err error) string {
	switch {
	case errors.Is(err, codec.ErrProtoBadMagic):
		return "bad_magic"
	case errors.Is(err, codec.ErrProtoBadLength):
		return "bad_len"
	case errors.Is(err, codec.ErrProtoBadCRC):
		return "bad_crc"
	case errors.Is(err, codec.ErrProtoBadSeq):
		return "bad_seq"
	case errors.Is(err, codec.ErrProtoDecrypt):
		return "decrypt"
	case errors.Is(err, codec.ErrTransportNotSupported):
		return "transport_unsupported"
	case errors.Is(err, codec.ErrUnexpectedEOF):
		return "unexpected_eof"
	default:
		return "other"
	}
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.draining.Store(true)
		metricDrain.Set(1)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if s.eng.Validate() == nil {
			logx.Debugf("stop engine... error: %v", s.eng.Stop(ctx))
		}
		if s.pool != nil {
			s.pool.Release()
		}
	})
}

func (s *Server) Serve() {
	// addrs := strings.Join(s.c.Gnetway.ToAddresses(), ",")
	logx.Debugf("addrs: %s", s.c.Gnetway.ToAddresses())

	options := []gnet.Option{
		gnet.WithMulticore(s.c.Gnetway.Multicore),
		gnet.WithSocketRecvBuffer(s.c.Gnetway.ReceiveBuf),
		gnet.WithSocketSendBuffer(s.c.Gnetway.SendBuf),
		gnet.WithLockOSThread(true),
		gnet.WithReuseAddr(true),
		gnet.WithTCPNoDelay(func() gnet.TCPSocketOpt {
			if s.c.Gnetway.TCPNoDelay {
				return gnet.TCPNoDelay
			}
			return gnet.TCPDelay
		}()),
		gnet.WithTicker(true),
		gnet.WithLogLevel(logging.DebugLevel),
		gnet.WithLogger(NewLogger()),
	}
	if s.c.Gnetway.TCPKeepAliveSeconds > 0 {
		options = append(options, gnet.WithTCPKeepAlive(time.Duration(s.c.Gnetway.TCPKeepAliveSeconds)*time.Second))
	}
	err := gnet.Rotate(
		s,
		s.c.Gnetway.ToAddresses(),
		options...)
	if err != nil {
		logx.Error(err)
		panic(err)
	}
}
