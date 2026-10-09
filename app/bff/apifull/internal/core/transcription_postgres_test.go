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
	"strconv"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestTranscriptionPostgresRoundTrip(t *testing.T) {
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
	msgID := int32(uid % 1000000000)
	transcriptionID := uid + 2
	keys := []string{
		txStoredKey(uid), txMsgKey(uid, msgID),
		b18Key(outsiderID, "tx"), b18Key(outsiderID, "txm:"+strconv.FormatInt(int64(msgID), 10)),
		"sms:" + strconv.FormatInt(uid, 10) + ":rateTranscribedAudio",
	}
	for _, key := range keys {
		if _, err = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key)
		}
		_ = db.Close()
	})

	owner := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	if _, err = owner.MessagesRateTranscribedAudio(&mtproto.TLMessagesRateTranscribedAudio{
		MsgId: msgID, TranscriptionId: transcriptionID,
	}); err != nil {
		t.Fatal(err)
	}

	for _, check := range []struct {
		key  string
		want string
	}{
		{txStoredKey(uid), strconv.FormatInt(transcriptionID, 10)},
		{txMsgKey(uid, msgID), strconv.FormatInt(transcriptionID, 10)},
	} {
		var stored string
		if err = db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, check.key).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != check.want {
			t.Fatalf("%s = %q, want %q", check.key, stored, check.want)
		}
	}

	for _, request := range []*mtproto.TLMessagesTranscribeAudio{
		{MsgId: msgID}, nil,
	} {
		got, getErr := owner.MessagesTranscribeAudio(request)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if got == nil || got.GetTranscriptionId() != transcriptionID || got.GetPending() {
			t.Fatalf("transcription readback = %#v, want id %d and pending=false", got, transcriptionID)
		}
	}

	outsider := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsiderID}}
	got, err := outsider.MessagesTranscribeAudio(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetTranscriptionId() != 0 {
		t.Fatalf("outsider transcription = %#v, must not expose owner state", got)
	}

	if err = persist.ClosePostgres(); err != nil {
		t.Fatal(err)
	}
	if err = persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	got, err = (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}).MessagesTranscribeAudio(&mtproto.TLMessagesTranscribeAudio{MsgId: msgID})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetTranscriptionId() != transcriptionID {
		t.Fatalf("reopened transcription = %#v, want id %d", got, transcriptionID)
	}
}
