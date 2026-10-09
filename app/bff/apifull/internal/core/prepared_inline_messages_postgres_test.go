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
)

func TestPreparedInlineMessagesPostgresRoundTrip(t *testing.T) {
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
	resultID := "prepared-pg-" + time.Now().UTC().Format("20060102150405.000000000")
	ownerKey := b7Key(uid, resultID)
	outsiderKey := b7Key(outsiderID, resultID)
	for _, key := range []string{ownerKey, outsiderKey} {
		if _, err = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k IN ($1, $2)`, ownerKey, outsiderKey)
		_ = db.Close()
	})

	owner := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	saved, err := owner.MessagesSavePreparedInlineMessage(&mtproto.TLMessagesSavePreparedInlineMessage{
		Result: &mtproto.InputBotInlineResult{Id: resultID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.GetId() != resultID {
		t.Fatalf("saved prepared result = %#v, want id %q", saved, resultID)
	}

	var stored string
	if err = db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, ownerKey).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != resultID {
		t.Fatalf("stored prepared result = %q, want %q", stored, resultID)
	}

	got, err := owner.MessagesGetPreparedInlineMessage(&mtproto.TLMessagesGetPreparedInlineMessage{Id: resultID})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetResult() == nil || got.GetResult().GetId() != resultID {
		t.Fatalf("owner readback = %#v, want result id %q", got, resultID)
	}

	outsider := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsiderID}}
	got, err = outsider.MessagesGetPreparedInlineMessage(&mtproto.TLMessagesGetPreparedInlineMessage{Id: resultID})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetResult() == nil || got.GetResult().GetId() != "" {
		t.Fatalf("outsider readback = %#v, must not expose owner result", got)
	}

	if err = persist.ClosePostgres(); err != nil {
		t.Fatal(err)
	}
	if err = persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	got, err = (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}).MessagesGetPreparedInlineMessage(&mtproto.TLMessagesGetPreparedInlineMessage{Id: resultID})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetResult() == nil || got.GetResult().GetId() != resultID {
		t.Fatalf("reopened readback = %#v, want result id %q", got, resultID)
	}
}
