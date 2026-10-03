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

package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestStatsUnavailableMethodsDoNotStoreOrEchoTokens(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81004}}
	if graph, err := c.StatsLoadAsyncGraph(nil); graph != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil graph request: %+v, %v", graph, err)
	}
	key := "b4:81004:"
	before, err := persist.Default.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := c.StatsLoadAsyncGraph(&mtproto.TLStatsLoadAsyncGraph{Token: "g81004"})
	if graph != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("graph: %+v, %v", graph, err)
	}
	after, err := persist.Default.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("graph token changed stored state: before %q, after %q", before, after)
	}
	if _, err = c.StatsLoadAsyncGraph(&mtproto.TLStatsLoadAsyncGraph{}); !errors.Is(err, mtproto.ErrTokenEmpty) {
		t.Fatalf("empty graph token: %v", err)
	}

	url, err := c.StatsGetBroadcastRevenueWithdrawalUrl(&mtproto.TLStatsGetBroadcastRevenueWithdrawalUrl{
		Peer: &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerChannel, ChannelId: 81004},
		Password: &mtproto.InputCheckPasswordSRP{
			PredicateName: mtproto.Predicate_inputCheckPasswordSRP,
			SrpId:         1,
			A:             []byte{1},
			M1:            []byte{1},
		},
	})
	if url != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("withdrawal URL: %+v, %v", url, err)
	}
}
