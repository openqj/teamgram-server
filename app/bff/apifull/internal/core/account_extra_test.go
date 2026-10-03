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

func TestAccountDeclinePasswordResetUser1(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.AccountDeclinePasswordReset(&mtproto.TLAccountDeclinePasswordReset{}); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordKDFSalt(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	pwd, err := c.AccountGetPassword(&mtproto.TLAccountGetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	algo := pwd.GetNewAlgo()
	if algo.GetPredicateName() != mtproto.Predicate_passwordKdfAlgoModPow {
		t.Fatalf("new algo %q", algo.GetPredicateName())
	}
	salt := algo.GetSalt1()
	if len(salt) != 32 || len(algo.GetSalt2()) != 32 {
		t.Fatalf("salt1 %x", salt)
	}
}
