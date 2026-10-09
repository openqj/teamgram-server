// Copyright 2026 Teamgram Authors
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

package core

import (
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestTranslateTextPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if !persist.PostgresEnabled() {
		t.Fatal("APIFull PostgreSQL store is not initialized")
	}

	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	uid := time.Now().UnixNano()
	outsiderID := uid + 1
	ownerKey := b13Key(uid)
	outsiderKey := b13Key(outsiderID)
	for _, key := range []string{ownerKey, outsiderKey} {
		if _, err = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k IN ($1, $2)`, ownerKey, outsiderKey)
		_ = db.Close()
	})

	owner := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	input := &mtproto.TLMessagesTranslateText{Text_FLAGSTRING: wrapperspb.String("hello postgres"), ToLang: "zh"}
	translated, err := owner.MessagesTranslateText(input)
	if err != nil {
		t.Fatal(err)
	}
	if translated == nil || translated.GetText() != "hello postgres" {
		t.Fatalf("translation = %#v, want text hello postgres", translated)
	}

	var stored string
	if err = db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, ownerKey).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "hello postgres" {
		t.Fatalf("stored translation = %q, want hello postgres", stored)
	}

	outsider := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsiderID}}
	translated, err = outsider.MessagesTranslateText(nil)
	if err != nil {
		t.Fatal(err)
	}
	if translated == nil || translated.GetText() != "" {
		t.Fatalf("outsider translation = %#v, must not expose owner text", translated)
	}

	if err = persist.ClosePostgres(); err != nil {
		t.Fatal(err)
	}
	if err = persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	translated, err = (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}).MessagesTranslateText(nil)
	if err != nil {
		t.Fatal(err)
	}
	if translated == nil || translated.GetText() != "hello postgres" {
		t.Fatalf("reopened translation = %#v, want hello postgres", translated)
	}
}
