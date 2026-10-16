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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/teamgram/proto/mtproto"
	sessionclient "github.com/teamgram/teamgram-server/app/interface/session/client"
	"github.com/teamgram/teamgram-server/app/interface/session/session"

	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

type sessionDataCtx struct {
	ctx     context.Context
	updates any
	result  chan error
}

// SessionOptions comet options.
type SessionOptions struct {
	RoutineSize uint64
	RoutineChan uint64
}

// Session is a gateway.
type Session struct {
	serverId       string
	client         sessionclient.SessionClient
	conn           *grpc.ClientConn
	sessionChan    []chan sessionDataCtx
	sessionChanNum uint64
	options        SessionOptions
	ctx            context.Context
	cancel         context.CancelFunc
	unavailable    atomic.Bool // set when connection errors detected
	workers        sync.WaitGroup
}

func isSessionConnError(err error) bool {
	s, ok := grpcStatus.FromError(err)
	if !ok {
		return false
	}
	switch s.Code() {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	}
	return false
}

// sessionRedirectTarget extracts the authoritative session shard from the
// redirect returned by a session node. A sync worker can observe a slightly
// stale consistent-hash view while session nodes are joining or leaving; the
// redirect is safe to follow because the rejected request has not mutated
// session state yet.
func sessionRedirectTarget(err error) (string, bool) {
	status, ok := grpcStatus.FromError(err)
	if !ok || status.Code() != codes.Code(700) {
		return "", false
	}
	const prefix = "REDIRECT_TO_"
	message := status.Message()
	if !strings.HasPrefix(message, prefix) {
		return "", false
	}
	target := strings.TrimSpace(strings.TrimPrefix(message, prefix))
	return target, target != ""
}

// process
func (c *Session) process(sessionChan chan sessionDataCtx) {
	defer c.workers.Done()
	for {
		select {
		case sessionData, ok := <-sessionChan:
			if !ok {
				logx.Errorf("process error")
				return
			}

			err := c.deliver(sessionData.ctx, sessionData.updates)
			c.unavailable.Store(isSessionConnError(err))
			sessionData.result <- err
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Session) deliver(ctx context.Context, updates any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var reply *mtproto.Bool
	var err error
	switch in := updates.(type) {
	case *session.TLSessionPushSessionUpdatesData:
		reply, err = c.client.SessionPushSessionUpdatesData(ctx, in)
	case *session.TLSessionPushUpdatesData:
		reply, err = c.client.SessionPushUpdatesData(ctx, in)
	case *session.TLSessionPushRpcResultData:
		reply, err = c.client.SessionPushRpcResultData(ctx, in)
	default:
		return fmt.Errorf("session(%s) invalid push type %T", c.serverId, updates)
	}
	if err != nil {
		return err
	}
	if !mtproto.FromBool(reply) {
		return fmt.Errorf("session(%s) rejected push", c.serverId)
	}
	return nil
}

func (c *Session) Close() error {
	c.cancel()
	if c.conn != nil {
		_ = c.conn.Close()
	}
	finish := make(chan struct{})
	go func() {
		c.workers.Wait()
		close(finish)
	}()
	select {
	case <-finish:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("close session(%s) timeout", c.serverId)
	}
}

func (c *Session) enqueue(ctx context.Context, msg any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	result := make(chan error, 1)
	idx := atomic.AddUint64(&c.sessionChanNum, 1) % c.options.RoutineSize
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.sessionChan[idx] <- sessionDataCtx{ctx: ctx, updates: msg, result: result}:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-result:
		return err
	}
}

func (c *Session) PushUpdates(ctx context.Context, msg *session.TLSessionPushUpdatesData) error {
	return c.enqueue(ctx, msg)
}

func (c *Session) PushSessionUpdates(ctx context.Context, msg *session.TLSessionPushSessionUpdatesData) error {
	return c.enqueue(ctx, msg)
}

func (c *Session) PushRpcResult(ctx context.Context, msg *session.TLSessionPushRpcResultData) error {
	return c.enqueue(ctx, msg)
}

// NewSession new a comet.
func NewSession(c zrpc.RpcClientConf, options SessionOptions) (*Session, error) {
	if len(c.Endpoints) == 0 || options.RoutineSize == 0 {
		return nil, fmt.Errorf("session requires an endpoint and push workers")
	}
	sess := &Session{
		serverId:    c.Endpoints[0],
		sessionChan: make([]chan sessionDataCtx, options.RoutineSize),
		options:     options,
	}

	cli, err := zrpc.NewClient(c)
	if err != nil {
		logx.Errorf("watchComet NewClient(%+v) error(%v)", c, err)
		return nil, err
	}
	sess.client = sessionclient.NewSessionClient(cli)
	sess.conn = cli.Conn()
	sess.ctx, sess.cancel = context.WithCancel(context.Background())

	for i := uint64(0); i < options.RoutineSize; i++ {
		sess.sessionChan[i] = make(chan sessionDataCtx, options.RoutineChan)
		sess.workers.Add(1)
		go sess.process(sess.sessionChan[i])
	}
	return sess, nil
}

func (d *Dao) watch(ctx context.Context, c zrpc.RpcClientConf) {
	sub, err := discov.NewSubscriber(c.Etcd.Hosts, c.Etcd.Key)
	if err != nil {
		logx.Errorf("watchComet NewSubscriber(%+v) error(%v)", c.Etcd.Hosts, err)
		return
	}
	defer sub.Close()
	var updateMu sync.Mutex
	update := func() {
		if ctx.Err() != nil {
			return
		}
		updateMu.Lock()
		defer updateMu.Unlock()

		values := sub.Values()
		if len(values) == 0 {
			return
		}

		d.mu.Lock()

		sessions := map[string]SessionPusher{}
		for _, v := range values {
			if old, ok := d.sessionServers[v]; ok {
				sessions[v] = old
				continue
			}

			var (
				cli SessionPusher
				err error
			)
			if d.useStreamSession {
				cli, err = NewStreamingSession(v)
			} else {
				c.Endpoints = []string{v}
				cli, err = NewSession(c, SessionOptions{
					RoutineSize: d.conf.Routine.Size,
					RoutineChan: d.conf.Routine.Chan,
				})
			}
			if err != nil {
				logx.Errorf("watchComet NewClient(%+v) error(%v)", values, err)
				// A node can be present in etcd before its gRPC listener is
				// ready. Keep healthy nodes and let the periodic refresh below
				// retry this node instead of permanently losing its shard.
				continue
			}
			sessions[v] = cli
		}

		var removed []SessionPusher
		for key, old := range d.sessionServers {
			if _, ok := sessions[key]; !ok {
				removed = append(removed, old)
				logx.Infof("watchComet DelComet:%s", key)
			}
		}

		d.sessionServers = sessions
		d.mu.Unlock()

		// close removed sessions outside of lock
		for _, old := range removed {
			old.Close()
		}
	}

	sub.AddListener(update)
	update()

	// etcd membership does not change when a process is still starting. Retry
	// failed stream creation without requiring a second membership event; this
	// closes the startup race that otherwise leaves Sync with a stale shard map.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			update()
		}
	}
}

func (d *Dao) PushUpdatesToSession(ctx context.Context, serverId string, msg *session.TLSessionPushUpdatesData) (err error) {
	started := time.Now()
	defer func() { ObserveDeliveryStage("gateway_push", started, err) }()
	return d.pushToSession(ctx, serverId, func(pusher SessionPusher) error {
		return pusher.PushUpdates(ctx, msg)
	}, msg.PermAuthKeyId)
}

func (d *Dao) PushSessionUpdatesToSession(ctx context.Context, serverId string, msg *session.TLSessionPushSessionUpdatesData) (err error) {
	started := time.Now()
	defer func() { ObserveDeliveryStage("gateway_push", started, err) }()
	return d.pushToSession(ctx, serverId, func(pusher SessionPusher) error {
		return pusher.PushSessionUpdates(ctx, msg)
	}, msg.PermAuthKeyId)
}

func (d *Dao) PushRpcResultToSession(ctx context.Context, serverId string, msg *session.TLSessionPushRpcResultData) (err error) {
	started := time.Now()
	defer func() { ObserveDeliveryStage("gateway_push", started, err) }()
	return d.pushToSession(ctx, serverId, func(pusher SessionPusher) error {
		return pusher.PushRpcResult(ctx, msg)
	}, msg.PermAuthKeyId)
}

func (d *Dao) pushToSession(ctx context.Context, serverId string, push func(SessionPusher) error, permAuthKeyID int64) error {
	d.mu.RLock()
	pusher, ok := d.sessionServers[serverId]
	d.mu.RUnlock()
	if !ok {
		logx.WithContext(ctx).Errorf("pushToSession - stale session node %s (permAuthKeyId:%d)", serverId, permAuthKeyID)
		return fmt.Errorf("stale session node %s", serverId)
	}

	err := push(pusher)
	target, redirected := sessionRedirectTarget(err)
	if !redirected || target == serverId {
		return err
	}

	d.mu.RLock()
	redirectedPusher, ok := d.sessionServers[target]
	d.mu.RUnlock()
	if !ok {
		return err
	}
	return push(redirectedPusher)
}
