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
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type businessChatLinkTestStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *businessChatLinkTestStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], nil
}

func (s *businessChatLinkTestStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func TestBusinessGreetingSetGet(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	ok, err := c.AccountUpdateBusinessGreetingMessage(&mtproto.TLAccountUpdateBusinessGreetingMessage{
		Message: &mtproto.InputBusinessGreetingMessage{
			ShortcutId:     42,
			NoActivityDays: 7,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !mtproto.FromBool(ok) {
		t.Fatal("expected boolTrue")
	}
	got := businessGreetingMessage(1)
	if got == nil || got.GetShortcutId() != 42 || got.GetNoActivityDays() != 7 {
		t.Fatalf("greeting roundtrip: %+v", got)
	}

	if _, err := (&ApiFullCore{}).AccountUpdateBusinessGreetingMessage(nil); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("auth: %v", err)
	}
}

func TestBusinessChatLinkLifecycle(t *testing.T) {
	previous := persist.Default
	persist.Use(&businessChatLinkTestStore{values: make(map[string]string)})
	t.Cleanup(func() { persist.Use(previous) })

	const ownerID int64 = 81024001
	owner := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: ownerID}}
	visitor := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: ownerID + 1}}

	created, err := owner.AccountCreateBusinessChatLink(&mtproto.TLAccountCreateBusinessChatLink{
		Link: &mtproto.InputBusinessChatLink{Message: "hello", Title: wrapperspb.String("Support")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.GetLink()) != 48 || created.GetLink() == "81024001-1" {
		t.Fatalf("created link slug is not an opaque 192-bit token: %q", created.GetLink())
	}

	second, err := owner.AccountCreateBusinessChatLink(&mtproto.TLAccountCreateBusinessChatLink{
		Link: &mtproto.InputBusinessChatLink{Message: "another"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.GetLink() == created.GetLink() {
		t.Fatal("generated duplicate business chat link slug")
	}

	listed, err := owner.AccountGetBusinessChatLinks(&mtproto.TLAccountGetBusinessChatLinks{})
	if err != nil || len(listed.GetLinks()) != 2 {
		t.Fatalf("get links: links=%d err=%v", len(listed.GetLinks()), err)
	}

	resolved, err := visitor.AccountResolveBusinessChatLink(&mtproto.TLAccountResolveBusinessChatLink{Slug: created.GetLink()})
	if err != nil || resolved.GetPeer().GetUserId() != ownerID || resolved.GetMessage() != "hello" {
		t.Fatalf("resolve link: peer=%+v message=%q err=%v", resolved.GetPeer(), resolved.GetMessage(), err)
	}

	edited, err := owner.AccountEditBusinessChatLink(&mtproto.TLAccountEditBusinessChatLink{
		Slug: created.GetLink(), Link: &mtproto.InputBusinessChatLink{Message: "updated", Title: wrapperspb.String("Help")},
	})
	if err != nil || edited.GetMessage() != "updated" || edited.GetTitle().GetValue() != "Help" {
		t.Fatalf("edit link: link=%+v err=%v", edited, err)
	}
	resolved, err = visitor.AccountResolveBusinessChatLink(&mtproto.TLAccountResolveBusinessChatLink{Slug: created.GetLink()})
	if err != nil || resolved.GetMessage() != "updated" {
		t.Fatalf("resolve edited link: message=%q err=%v", resolved.GetMessage(), err)
	}

	if _, err := owner.AccountEditBusinessChatLink(&mtproto.TLAccountEditBusinessChatLink{
		Slug: "missing", Link: &mtproto.InputBusinessChatLink{Message: "must not create"},
	}); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("edit unknown slug: %v", err)
	}

	if result, err := owner.AccountDeleteBusinessChatLink(&mtproto.TLAccountDeleteBusinessChatLink{Slug: created.GetLink()}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("delete link: result=%v err=%v", result, err)
	}
	resolved, err = visitor.AccountResolveBusinessChatLink(&mtproto.TLAccountResolveBusinessChatLink{Slug: created.GetLink()})
	if resolved != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("deleted slug did not fail closed: response=%+v err=%v", resolved, err)
	}
	if _, err := owner.AccountDeleteBusinessChatLink(&mtproto.TLAccountDeleteBusinessChatLink{Slug: second.GetLink()}); err != nil {
		t.Fatal(err)
	}
	listed, err = owner.AccountGetBusinessChatLinks(&mtproto.TLAccountGetBusinessChatLinks{})
	if err != nil || len(listed.GetLinks()) != 0 {
		t.Fatalf("links remain after cleanup: links=%d err=%v", len(listed.GetLinks()), err)
	}
}
