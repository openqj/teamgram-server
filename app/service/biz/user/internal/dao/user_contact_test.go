package dao

import (
	"errors"
	"reflect"
	"testing"
)

func TestGetContactIdListUsesCachedIDs(t *testing.T) {
	called := false
	got, err := getContactIdList([]int64{9}, func() ([]int64, error) {
		called = true
		return []int64{10}, nil
	})
	if err != nil {
		t.Fatalf("getContactIdList() error = %v", err)
	}
	if called {
		t.Fatal("getContactIdList() queried the database with cached IDs")
	}
	if !reflect.DeepEqual(got, []int64{9}) {
		t.Fatalf("getContactIdList() = %v, want [9]", got)
	}
}

func TestGetContactIdListLoadsWhenCacheIsEmpty(t *testing.T) {
	want := []int64{9, 10}
	got, err := getContactIdList(nil, func() ([]int64, error) {
		return want, nil
	})
	if err != nil {
		t.Fatalf("getContactIdList() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getContactIdList() = %v, want %v", got, want)
	}
}

func TestGetContactIdListPropagatesLoadError(t *testing.T) {
	wantErr := errors.New("contact query failed")
	got, err := getContactIdList(nil, func() ([]int64, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("getContactIdList() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("getContactIdList() = %v, want nil on error", got)
	}
}
