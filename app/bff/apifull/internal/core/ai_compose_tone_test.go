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
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAiComposeToneRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: time.Now().UnixNano()}}
	if _, err := c.AicomposeCreateTone(&mtproto.TLAicomposeCreateTone{Title: "warm"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.AicomposeGetTones(&mtproto.TLAicomposeGetTones{})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.GetTones()) != 1 || got.GetTones()[0].GetTitle() != "warm" {
		t.Fatalf("tone: %+v", got)
	}
}
