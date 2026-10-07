package dao

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	kvstore "github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
)

type testQRCodeKV struct {
	kvstore.ExtStore
	mu     sync.Mutex
	hashes map[string]map[string]string
}

func newTestQRCodeKV() *testQRCodeKV {
	return &testQRCodeKV{hashes: make(map[string]map[string]string)}
}

func (s *testQRCodeKV) HgetallCtx(_ context.Context, key string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.hashes[key]))
	for field, value := range s.hashes[key] {
		result[field] = value
	}
	return result, nil
}

func (s *testQRCodeKV) EvalCtx(_ context.Context, script, key string, args ...any) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields := s.hashes[key]
	switch script {
	case createQRCodeScript:
		if len(args) != 1+2*12 {
			return nil, fmt.Errorf("unexpected create eval arguments: %d", len(args))
		}
		if _, exists := s.hashes[key]; exists {
			return int64(0), nil
		}
		fields = make(map[string]string)
		s.hashes[key] = fields
		for i := 1; i < len(args); i += 2 {
			fields[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
		}
		return int64(1), nil
	case rotateQRCodeScript:
		if len(args) != 3+2*12 {
			return nil, fmt.Errorf("unexpected rotate eval arguments: %d", len(args))
		}
		if fields["code_hash"] != fmt.Sprint(args[0]) {
			return int64(-1), nil
		}
		if fields["state"] != fmt.Sprint(args[1]) {
			return int64(0), nil
		}
		for i := 3; i < len(args); i += 2 {
			fields[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
		}
		return int64(1), nil
	case acceptQRCodeScript, acceptQRCodeAtDCScript:
		wantArgs := 6
		if script == acceptQRCodeAtDCScript {
			wantArgs = 7
		}
		if len(args) != wantArgs {
			return nil, fmt.Errorf("unexpected accept eval arguments: %d", len(args))
		}
		if fields["code_hash"] != fmt.Sprint(args[0]) {
			return int64(-1), nil
		}
		expireAt, err := strconv.ParseInt(fields["expire_at"], 10, 64)
		if err != nil {
			return nil, err
		}
		now, err := strconv.ParseInt(fmt.Sprint(args[1]), 10, 64)
		if err != nil {
			return nil, err
		}
		if expireAt < now {
			return int64(-2), nil
		}
		if fields["state"] == fmt.Sprint(args[2]) {
			fields["user_id"] = fmt.Sprint(args[3])
			fields["state"] = fmt.Sprint(args[4])
			if script == acceptQRCodeAtDCScript {
				fields["dc_id"] = fmt.Sprint(args[6])
			}
			return int64(1), nil
		}
		if (fields["state"] == fmt.Sprint(args[4]) || fields["state"] == fmt.Sprint(args[5])) && fields["user_id"] == fmt.Sprint(args[3]) {
			if script == acceptQRCodeAtDCScript && fields["dc_id"] == "0" {
				fields["dc_id"] = fmt.Sprint(args[6])
			}
			return int64(2), nil
		}
		if fields["state"] == fmt.Sprint(args[4]) || fields["state"] == fmt.Sprint(args[5]) {
			return int64(0), nil
		}
		return int64(-1), nil
	case commitQRCodeScript:
		if len(args) != 4 {
			return nil, fmt.Errorf("unexpected commit eval arguments: %d", len(args))
		}
		if fields["code_hash"] != fmt.Sprint(args[0]) {
			return int64(-1), nil
		}
		if fields["user_id"] != fmt.Sprint(args[1]) {
			return int64(0), nil
		}
		if fields["state"] == fmt.Sprint(args[2]) {
			fields["state"] = fmt.Sprint(args[3])
			return int64(1), nil
		}
		if fields["state"] == fmt.Sprint(args[3]) {
			return int64(2), nil
		}
		return int64(-1), nil
	default:
		return nil, fmt.Errorf("unexpected eval script")
	}
}

func TestQRCodeCacheRoundTripAndAtomicAccept(t *testing.T) {
	store := newTestQRCodeKV()
	d := &Dao{kv: store}
	qrCode := &model.QRCodeTransaction{
		PermAuthKeyId: 100,
		DcId:          2,
		AuthKeyId:     101,
		ApiId:         1,
		ApiHash:       "0123456789abcdef0123456789abcdef",
		ExceptIDs:     []int64{7, 9007199254740993},
		CodeHash:      "qr-code-hash",
		ExpireAt:      1000,
		State:         model.QRCodeStateNew,
	}
	if result, err := d.CreateCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode, 60); err != nil || result != 1 {
		t.Fatalf("create transaction = %d, %v; want 1, nil", result, err)
	}

	loaded, err := d.GetCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ExceptIDs) != 2 || loaded.ExceptIDs[0] != 7 || loaded.ExceptIDs[1] != 9007199254740993 {
		t.Fatalf("except IDs round trip = %v, want [7 9007199254740993]", loaded.ExceptIDs)
	}
	if loaded.DcId != qrCode.DcId {
		t.Fatalf("dc id round trip = %d, want %d", loaded.DcId, qrCode.DcId)
	}

	result, err := d.AcceptCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode.CodeHash, 9, 999)
	if err != nil || result != 1 {
		t.Fatalf("first accept = %d, %v; want 1, nil", result, err)
	}
	result, err = d.AcceptCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode.CodeHash, 10, 999)
	if err != nil || result != 0 {
		t.Fatalf("second accept = %d, %v; want 0, nil", result, err)
	}

	loaded, err = d.GetCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UserId != 9 || loaded.State != model.QRCodeStateAccepted || len(loaded.ExceptIDs) != 2 {
		t.Fatalf("accepted transaction lost state or except IDs: %+v", loaded)
	}
}

func TestQRCodeClaimCanResumeAndCommitOnlyForSameUser(t *testing.T) {
	store := newTestQRCodeKV()
	d := &Dao{kv: store}
	qrCode := &model.QRCodeTransaction{
		PermAuthKeyId: 150,
		AuthKeyId:     151,
		CodeHash:      "recoverable",
		ExpireAt:      1000,
		State:         model.QRCodeStateNew,
	}
	if result, err := d.CreateCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode, 60); err != nil || result != 1 {
		t.Fatalf("create transaction = %d, %v; want 1, nil", result, err)
	}
	if result, err := d.AcceptCacheQRLoginCode(context.Background(), 150, "recoverable", 9, 999); err != nil || result != 1 {
		t.Fatalf("first claim = %d, %v; want 1, nil", result, err)
	}
	if result, err := d.AcceptCacheQRLoginCode(context.Background(), 150, "recoverable", 9, 999); err != nil || result != 2 {
		t.Fatalf("same-user resume = %d, %v; want 2, nil", result, err)
	}
	if result, err := d.CommitCacheQRLoginCode(context.Background(), 150, "recoverable", 10); err != nil || result != 0 {
		t.Fatalf("other-user commit = %d, %v; want 0, nil", result, err)
	}
	if result, err := d.CommitCacheQRLoginCode(context.Background(), 150, "recoverable", 9); err != nil || result != 1 {
		t.Fatalf("owner commit = %d, %v; want 1, nil", result, err)
	}
	if result, err := d.CommitCacheQRLoginCode(context.Background(), 150, "recoverable", 9); err != nil || result != 2 {
		t.Fatalf("idempotent commit = %d, %v; want 2, nil", result, err)
	}
}

func TestQRCodeAcceptAtDCMovesOwnershipAtomically(t *testing.T) {
	store := newTestQRCodeKV()
	d := &Dao{kv: store}
	qrCode := &model.QRCodeTransaction{
		PermAuthKeyId: 175,
		DcId:          2,
		AuthKeyId:     176,
		CodeHash:      "migrate",
		ExpireAt:      1000,
		State:         model.QRCodeStateNew,
	}
	if result, err := d.CreateCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode, 60); err != nil || result != 1 {
		t.Fatalf("create transaction = %d, %v; want 1, nil", result, err)
	}
	if result, err := d.AcceptCacheQRLoginCodeAtDC(context.Background(), qrCode.PermAuthKeyId, qrCode.CodeHash, 42, 999, 1); err != nil || result != 1 {
		t.Fatalf("cross-DC accept = %d, %v; want 1, nil", result, err)
	}
	loaded, err := d.GetCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DcId != 1 || loaded.UserId != 42 || loaded.State != model.QRCodeStateAccepted {
		t.Fatalf("accepted QR = %+v, want dc=1 user=42 state=Accepted", loaded)
	}
	if result, err := d.AcceptCacheQRLoginCodeAtDC(context.Background(), qrCode.PermAuthKeyId, qrCode.CodeHash, 42, 999, 2); err != nil || result != 2 {
		t.Fatalf("same-user resume = %d, %v; want 2, nil", result, err)
	}
	loaded, err = d.GetCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DcId != 1 {
		t.Fatalf("same-user resume rewrote owner DC to %d, want 1", loaded.DcId)
	}
}

func TestQRCodeAcceptAtDCBackfillsLegacyOwnershipOnRetry(t *testing.T) {
	store := newTestQRCodeKV()
	d := &Dao{kv: store}
	qrCode := &model.QRCodeTransaction{
		PermAuthKeyId: 220,
		DcId:          0,
		AuthKeyId:     221,
		CodeHash:      "legacy",
		ExpireAt:      1000,
		UserId:        42,
		State:         model.QRCodeStateAccepted,
	}
	if result, err := d.CreateCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode, 60); err != nil || result != 1 {
		t.Fatalf("create legacy QR = (%d, %v)", result, err)
	}
	if result, err := d.AcceptCacheQRLoginCodeAtDC(context.Background(), qrCode.PermAuthKeyId, qrCode.CodeHash, qrCode.UserId, 999, 1); err != nil || result != 2 {
		t.Fatalf("retry legacy QR = (%d, %v), want (2, nil)", result, err)
	}
	loaded, err := d.GetCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DcId != 1 || loaded.UserId != qrCode.UserId || loaded.State != model.QRCodeStateAccepted {
		t.Fatalf("legacy QR after retry = %+v, want dc=1 user=42 state=Accepted", loaded)
	}
}

func TestQRCodeCacheAcceptRejectsStaleAndExpiredTransactions(t *testing.T) {
	store := newTestQRCodeKV()
	d := &Dao{kv: store}
	qrCode := &model.QRCodeTransaction{
		PermAuthKeyId: 200,
		AuthKeyId:     201,
		CodeHash:      "current",
		ExpireAt:      10,
		State:         model.QRCodeStateNew,
	}
	if result, err := d.CreateCacheQRLoginCode(context.Background(), qrCode.PermAuthKeyId, qrCode, 60); err != nil || result != 1 {
		t.Fatalf("create transaction = %d, %v; want 1, nil", result, err)
	}
	if result, err := d.AcceptCacheQRLoginCode(context.Background(), 200, "old", 9, 9); err != nil || result != -1 {
		t.Fatalf("stale-code accept = %d, %v; want -1, nil", result, err)
	}
	if result, err := d.AcceptCacheQRLoginCode(context.Background(), 200, "current", 9, 11); err != nil || result != -2 {
		t.Fatalf("expired accept = %d, %v; want -2, nil", result, err)
	}
}

func TestQRCodeRotateAndAcceptAreAtomicAgainstEachOther(t *testing.T) {
	for i := 0; i < 100; i++ {
		store := newTestQRCodeKV()
		d := &Dao{kv: store}
		oldQRCode := &model.QRCodeTransaction{
			PermAuthKeyId: 300,
			AuthKeyId:     301,
			CodeHash:      "old-code",
			ExpireAt:      1000,
			State:         model.QRCodeStateNew,
		}
		if result, err := d.CreateCacheQRLoginCode(context.Background(), oldQRCode.PermAuthKeyId, oldQRCode, 60); err != nil || result != 1 {
			t.Fatalf("create transaction = %d, %v; want 1, nil", result, err)
		}
		newQRCode := *oldQRCode
		newQRCode.CodeHash = "new-code"
		newQRCode.ExpireAt = 2000

		start := make(chan struct{})
		var wg sync.WaitGroup
		var rotateResult, acceptResult int64
		var rotateErr, acceptErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			rotateResult, rotateErr = d.RotateCacheQRLoginCode(context.Background(), oldQRCode.PermAuthKeyId, oldQRCode.CodeHash, &newQRCode, 60)
		}()
		go func() {
			defer wg.Done()
			<-start
			acceptResult, acceptErr = d.AcceptCacheQRLoginCode(context.Background(), oldQRCode.PermAuthKeyId, oldQRCode.CodeHash, 9, 999)
		}()
		close(start)
		wg.Wait()
		if rotateErr != nil || acceptErr != nil {
			t.Fatalf("rotate/accept errors = %v / %v", rotateErr, acceptErr)
		}

		loaded, err := d.GetCacheQRLoginCode(context.Background(), oldQRCode.PermAuthKeyId)
		if err != nil {
			t.Fatal(err)
		}
		switch rotateResult {
		case 1:
			if acceptResult != -1 || loaded.CodeHash != newQRCode.CodeHash || loaded.State != model.QRCodeStateNew || loaded.UserId != 0 {
				t.Fatalf("rotation won but state was corrupted: rotate=%d accept=%d qr=%+v", rotateResult, acceptResult, loaded)
			}
		case 0:
			if acceptResult != 1 || loaded.CodeHash != oldQRCode.CodeHash || loaded.State != model.QRCodeStateAccepted || loaded.UserId != 9 {
				t.Fatalf("accept won but state was overwritten: rotate=%d accept=%d qr=%+v", rotateResult, acceptResult, loaded)
			}
		default:
			t.Fatalf("unexpected rotate result: %d", rotateResult)
		}
	}
}
