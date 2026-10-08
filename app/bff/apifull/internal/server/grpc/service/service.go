// Copyright 2026 Teamgram Authors
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

package service

import (
	"context"
	"sync"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

type Service struct {
	svcCtx       *svc.ServiceContext
	workerMu     sync.Mutex
	workerCancel context.CancelFunc
	workerDone   chan struct{}
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) GetServiceContext() *svc.ServiceContext {
	return s.svcCtx
}

func (s *Service) StartWorkers() {
	if s == nil || s.svcCtx == nil || s.svcCtx.Config.PostgresDSN == "" {
		return
	}
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if s.workerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.workerCancel, s.workerDone = cancel, done
	go func() {
		defer close(done)
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			core.RunPremiumGrantReconciler(ctx, s.svcCtx)
		}()
		go func() {
			defer workers.Done()
			core.RunChannelDeliveryReconciler(ctx, s.svcCtx)
		}()
		workers.Wait()
	}()
}

func (s *Service) StopWorkers() {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if s.workerCancel == nil {
		return
	}
	s.workerCancel()
	<-s.workerDone
	s.workerCancel, s.workerDone = nil, nil
}
