package dao

import (
	"context"
	"testing"
)

func TestPostgresMethodsFailClosedWhenStoreIsMissing(t *testing.T) {
	d := &Dao{}
	ctx := context.Background()
	if _, err := d.SelectUserByID(ctx, 1); err == nil {
		t.Fatal("SelectUserByID returned a successful empty result without a store")
	}
	if _, err := d.SelectUsernames(ctx, 1, nil); err == nil {
		t.Fatal("SelectUsernames returned a successful empty result without a store")
	}
	if _, err := d.SelectBotByToken(ctx, "token"); err == nil {
		t.Fatal("SelectBotByToken returned a successful zero result without a store")
	}
	if _, _, err := d.InsertOrUpdateContact(ctx, nil); err == nil {
		t.Fatal("InsertOrUpdateContact returned a successful zero result without a store")
	}
}
