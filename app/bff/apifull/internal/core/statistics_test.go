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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestStatsUnknownGraphDoesNotStoreOrEchoTokens(t *testing.T) {
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
	if graph != nil || !errors.Is(err, mtproto.ErrGraphInvalidReload) {
		t.Fatalf("unknown graph token: %+v, %v", graph, err)
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

func TestStatsLoadAsyncGraphPostgresRoundTrip(t *testing.T) {
	const (
		userID = int64(981401)
		token  = "roundtrip-981401"
		key    = "stats:graph:981401:roundtrip-981401"
	)
	write := func(v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err = persist.Default.Set(key, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = persist.Default.Set(key, "") })

	write(map[string]any{
		"user_id":    userID,
		"data":       `{"count":7}`,
		"zoom_token": "zoom-1",
		"expires_at": time.Now().Add(time.Minute).Unix(),
	})
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	graph, err := core.StatsLoadAsyncGraph(&mtproto.TLStatsLoadAsyncGraph{Token: token})
	if err != nil || graph == nil || graph.GetPredicateName() != mtproto.Predicate_statsGraph {
		t.Fatalf("load graph: graph=%+v err=%v", graph, err)
	}
	if graph.GetJson() == nil || graph.GetJson().GetData() != `{"count":7}` {
		t.Fatalf("graph JSON=%+v", graph.GetJson())
	}
	if graph.GetZoomToken() == nil || graph.GetZoomToken().GetValue() != "zoom-1" {
		t.Fatalf("graph zoom token=%v", graph.GetZoomToken())
	}

	other := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID + 1}}
	if got, err := other.StatsLoadAsyncGraph(&mtproto.TLStatsLoadAsyncGraph{Token: token}); got != nil || !errors.Is(err, mtproto.ErrGraphInvalidReload) {
		t.Fatalf("cross-user graph access: graph=%+v err=%v", got, err)
	}

	write(map[string]any{"user_id": userID, "data": `{"count":8}`, "expires_at": time.Now().Add(-time.Second).Unix()})
	if got, err := core.StatsLoadAsyncGraph(&mtproto.TLStatsLoadAsyncGraph{Token: token}); got != nil || !errors.Is(err, mtproto.ErrGraphExpiredReload) {
		t.Fatalf("expired graph: graph=%+v err=%v", got, err)
	}

	write(map[string]any{"user_id": userID, "error": "provider unavailable"})
	got, err := core.StatsLoadAsyncGraph(&mtproto.TLStatsLoadAsyncGraph{Token: token})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_statsGraphError || got.GetError() != "provider unavailable" {
		t.Fatalf("graph error: graph=%+v err=%v", got, err)
	}
}
