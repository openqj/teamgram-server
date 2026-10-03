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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMainMiniBotAppsLangRoundTrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81008}}
	const want = "en"
	if _, err := c.BotsAddPreviewMedia(&mtproto.TLBotsAddPreviewMedia{LangCode: want}); err != nil {
		t.Fatal(err)
	}
	got, err := c.BotsGetPreviewInfo(&mtproto.TLBotsGetPreviewInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetLangCodes()) != 1 || got.GetLangCodes()[0] != want {
		t.Fatalf("lang %v", got.GetLangCodes())
	}
}
