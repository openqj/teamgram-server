package dao

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/zeromicro/go-zero/core/stores/kv"
)

func TestExportedAuthorizationClaimIsDCAndTargetBound(t *testing.T) {
	store := newExportedAuthorizationTestStore()
	d := &Dao{kv: store}
	token := bytes.Repeat([]byte{0x7a}, ExportedAuthorizationTokenSize)
	want := &ExportedAuthorization{
		ID: 101, UserID: 202, SourceAuthKeyID: -303,
		SourceDCID: 1, TargetDCID: 2, State: ExportedAuthorizationReady,
	}

	if err := d.PutExportedAuthorization(context.Background(), token, want); err != nil {
		t.Fatalf("PutExportedAuthorization() error = %v", err)
	}
	if got, err := d.ClaimExportedAuthorization(context.Background(), token, 404, 3); err != nil || got != nil {
		t.Fatalf("wrong-DC claim = (%#v, %v), want (nil, nil)", got, err)
	}
	preview, err := d.GetExportedAuthorization(context.Background(), token)
	if err != nil || preview == nil || preview.State != ExportedAuthorizationReady {
		t.Fatalf("credential after wrong-DC claim = (%#v, %v), want Ready", preview, err)
	}

	claimed, err := d.ClaimExportedAuthorization(context.Background(), token, 404, 2)
	if err != nil || claimed == nil || claimed.State != ExportedAuthorizationBinding || claimed.TargetAuthKeyID != 404 {
		t.Fatalf("target claim = (%#v, %v), want Binding for key 404", claimed, err)
	}
	if got, err := d.ClaimExportedAuthorization(context.Background(), token, 404, 2); err != nil || got == nil {
		t.Fatalf("same-target retry = (%#v, %v), want recoverable claim", got, err)
	}
	if got, err := d.ClaimExportedAuthorization(context.Background(), token, 405, 2); err != nil || got != nil {
		t.Fatalf("other-target claim = (%#v, %v), want (nil, nil)", got, err)
	}
	if completed, err := d.CompleteExportedAuthorization(context.Background(), token, 405); err != nil || completed {
		t.Fatalf("other-target complete = (%t, %v), want false, nil", completed, err)
	}
	if completed, err := d.CompleteExportedAuthorization(context.Background(), token, 404); err != nil || !completed {
		t.Fatalf("owner complete = (%t, %v), want true, nil", completed, err)
	}
	if completed, err := d.CompleteExportedAuthorization(context.Background(), token, 404); err != nil || !completed {
		t.Fatalf("idempotent complete = (%t, %v), want true, nil", completed, err)
	}
	if got, err := d.ClaimExportedAuthorization(context.Background(), token, 404, 2); err != nil || got != nil {
		t.Fatalf("claim after complete = (%#v, %v), want (nil, nil)", got, err)
	}
}

func TestExportedAuthorizationAllowsOnlyOneTarget(t *testing.T) {
	store := newExportedAuthorizationTestStore()
	d := &Dao{kv: store}
	token := bytes.Repeat([]byte{0x3d}, ExportedAuthorizationTokenSize)
	credential := &ExportedAuthorization{
		ID: 101, UserID: 202, SourceAuthKeyID: -303,
		SourceDCID: 1, TargetDCID: 2, State: ExportedAuthorizationReady,
	}
	if err := d.PutExportedAuthorization(context.Background(), token, credential); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan *ExportedAuthorization, 2)
	var wg sync.WaitGroup
	for _, target := range []int64{404, 405} {
		wg.Add(1)
		go func(targetAuthKeyID int64) {
			defer wg.Done()
			<-start
			result, err := d.ClaimExportedAuthorization(context.Background(), token, targetAuthKeyID, 2)
			if err != nil {
				t.Errorf("ClaimExportedAuthorization() error = %v", err)
			}
			results <- result
		}(target)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result != nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("different target keys claimed credential %d times, want 1", successes)
	}
}

type exportedAuthorizationTestStore struct {
	kv.Store
	mu     sync.Mutex
	hashes map[string]map[string]string
}

func newExportedAuthorizationTestStore() *exportedAuthorizationTestStore {
	return &exportedAuthorizationTestStore{hashes: make(map[string]map[string]string)}
}

func (s *exportedAuthorizationTestStore) HgetallCtx(_ context.Context, key string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.hashes[key]))
	for field, value := range s.hashes[key] {
		result[field] = value
	}
	return result, nil
}

func (s *exportedAuthorizationTestStore) EvalCtx(_ context.Context, script, key string, args ...any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields := s.hashes[key]
	switch script {
	case putExportedAuthorizationScript:
		if fields != nil {
			return int64(0), nil
		}
		if len(args) != 7 {
			return nil, fmt.Errorf("unexpected put arguments: %d", len(args))
		}
		s.hashes[key] = map[string]string{
			"id": fmt.Sprint(args[0]), "user_id": fmt.Sprint(args[1]),
			"source_auth_key_id": fmt.Sprint(args[2]), "source_dc_id": fmt.Sprint(args[3]),
			"target_dc_id": fmt.Sprint(args[4]), "target_auth_key_id": "0", "state": fmt.Sprint(args[5]),
		}
		return int64(1), nil
	case claimExportedAuthorizationScript:
		if fields == nil {
			return int64(-1), nil
		}
		if fields["target_dc_id"] != fmt.Sprint(args[1]) {
			return int64(-2), nil
		}
		if fields["state"] == fmt.Sprint(args[2]) {
			fields["state"] = fmt.Sprint(args[3])
			fields["target_auth_key_id"] = fmt.Sprint(args[0])
			return int64(1), nil
		}
		if fields["state"] == fmt.Sprint(args[3]) && fields["target_auth_key_id"] == fmt.Sprint(args[0]) {
			return int64(2), nil
		}
		return int64(0), nil
	case completeExportedAuthorizationScript:
		if fields == nil {
			return int64(-1), nil
		}
		if fields["state"] == fmt.Sprint(args[1]) && fields["target_auth_key_id"] == fmt.Sprint(args[0]) {
			fields["state"] = fmt.Sprint(args[2])
			return int64(1), nil
		}
		if fields["state"] == fmt.Sprint(args[2]) && fields["target_auth_key_id"] == fmt.Sprint(args[0]) {
			return int64(2), nil
		}
		return int64(0), nil
	default:
		return nil, fmt.Errorf("unexpected script")
	}
}
