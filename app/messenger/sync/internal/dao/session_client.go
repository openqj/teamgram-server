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

func (d *Dao) watch(c zrpc.RpcClientConf) {
	sub, _ := discov.NewSubscriber(c.Etcd.Hosts, c.Etcd.Key)
	update := func() {
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
				d.mu.Unlock()
				logx.Errorf("watchComet NewClient(%+v) error(%v)", values, err)
				return
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
}

func (d *Dao) PushUpdatesToSession(ctx context.Context, serverId string, msg *session.TLSessionPushUpdatesData) (err error) {
	d.mu.RLock()
	pusher, ok := d.sessionServers[serverId]
	d.mu.RUnlock()

	if ok {
		return pusher.PushUpdates(ctx, msg)
	}
	logx.WithContext(ctx).Errorf("PushUpdatesToSession - stale gateway, serverId %s not in active sessions (permAuthKeyId:%d)",
		serverId, msg.PermAuthKeyId)
	return fmt.Errorf("stale gateway %s", serverId)
}

func (d *Dao) PushSessionUpdatesToSession(ctx context.Context, serverId string, msg *session.TLSessionPushSessionUpdatesData) (err error) {
	d.mu.RLock()
	pusher, ok := d.sessionServers[serverId]
	d.mu.RUnlock()

	if ok {
		return pusher.PushSessionUpdates(ctx, msg)
	}
	logx.WithContext(ctx).Errorf("PushSessionUpdatesToSession - stale gateway, serverId %s not in active sessions (permAuthKeyId:%d)",
		serverId, msg.PermAuthKeyId)
	return fmt.Errorf("stale gateway %s", serverId)
}

func (d *Dao) PushRpcResultToSession(ctx context.Context, serverId string, msg *session.TLSessionPushRpcResultData) (err error) {
	d.mu.RLock()
	pusher, ok := d.sessionServers[serverId]
	d.mu.RUnlock()

	if ok {
		return pusher.PushRpcResult(ctx, msg)
	}
	logx.WithContext(ctx).Errorf("PushRpcResultToSession - stale gateway, serverId %s not in active sessions (permAuthKeyId:%d)",
		serverId, msg.PermAuthKeyId)
	return fmt.Errorf("stale gateway %s", serverId)
}
