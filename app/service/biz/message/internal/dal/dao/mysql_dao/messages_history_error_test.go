package mysql_dao

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

func TestSelectBackwardByOffsetIdLimitPropagatesQueryError(t *testing.T) {
	db, err := sqlx.Open(&sqlx.Config{DSN: "root@unix(" + filepath.Join(t.TempDir(), "missing.sock") + ")/message_history_error_test?timeout=100ms"})
	if err != nil {
		t.Fatalf("open lazy test connection: %v", err)
	}

	_, err = NewMessagesDAO(db, 1).SelectBackwardByOffsetIdLimitWithCB(context.Background(), 41, 41, 42, 100, 20, nil)
	if err == nil {
		t.Fatal("SelectBackwardByOffsetIdLimitWithCB() error = nil, want connection error")
	}
}
