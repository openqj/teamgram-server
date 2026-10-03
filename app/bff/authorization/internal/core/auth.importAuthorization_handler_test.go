package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAuthImportAuthorizationRejectsInvalidCredential(t *testing.T) {
	core := &AuthorizationCore{}
	got, err := core.AuthImportAuthorization(&mtproto.TLAuthImportAuthorization{Id: 123})
	if got != nil || err != mtproto.ErrAuthBytesInvalid {
		t.Fatalf("AuthImportAuthorization() = (%+v, %v), want (nil, AUTH_BYTES_INVALID)", got, err)
	}
}

const (
	importAuthorizationTargetAuthKeyID = int64(1001)
	importAuthorizationSourceAuthKeyID = int64(-2002)
	importAuthorizationUserID          = int64(3003)
	importAuthorizationSourceDCID      = int32(1)
	importAuthorizationTargetDCID      = int32(2)
)

func TestAuthImportAuthorizationDoesNotConsumeInvalidSourceCredential(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	importer.sourceUserID = importAuthorizationUserID + 1

	result, err := newImportAuthorizationCore().importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
	if result != nil || err != mtproto.ErrAuthBytesInvalid {
		t.Fatalf("importAuthorization() = (%v, %v), want (nil, AUTH_BYTES_INVALID)", result, err)
	}
	if importer.claimCalls != 0 {
		t.Fatalf("ClaimExportedAuthorization() calls = %d, want 0", importer.claimCalls)
	}
	if importer.authorization == nil {
		t.Fatal("invalid source ownership consumed the credential")
	}
}

func TestAuthImportAuthorizationDoesNotConsumeMissingUser(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	importer.user = nil

	result, err := newImportAuthorizationCore().importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
	if result != nil || err != mtproto.ErrAuthBytesInvalid {
		t.Fatalf("importAuthorization() = (%v, %v), want (nil, AUTH_BYTES_INVALID)", result, err)
	}
	if importer.claimCalls != 0 {
		t.Fatalf("ClaimExportedAuthorization() calls = %d, want 0", importer.claimCalls)
	}
	if importer.authorization == nil {
		t.Fatal("missing user consumed the credential")
	}
}

func TestAuthImportAuthorizationBindingFailureCanRetrySameTarget(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	bindFailure := errors.New("bind transport failure")
	importer.bindErr = bindFailure
	core := newImportAuthorizationCore()

	result, err := core.importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
	if result != nil || !errors.Is(err, bindFailure) {
		t.Fatalf("first import = (%v, %v), want recoverable bind failure", result, err)
	}
	if importer.authorization.State != dao.ExportedAuthorizationBinding || importer.authorization.TargetAuthKeyID != importAuthorizationTargetAuthKeyID {
		t.Fatalf("credential after bind failure = %+v, want Binding for target key", importer.authorization)
	}

	importer.bindErr = nil
	result, err = core.importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
	if result == nil || err != nil {
		t.Fatalf("retry import = (%v, %v), want authorization, nil", result, err)
	}
	if importer.authorization.State != dao.ExportedAuthorizationComplete {
		t.Fatalf("credential state = %d, want Complete", importer.authorization.State)
	}
	if importer.claimCalls != 2 || importer.completeCalls != 1 {
		t.Fatalf("claim/complete calls = %d/%d, want 2/1", importer.claimCalls, importer.completeCalls)
	}
}

func TestAuthImportAuthorizationDoesNotCompleteOnUnconfirmedBind(t *testing.T) {
	for _, bindReply := range []*mtproto.Int64{nil, {V: 0}, {V: 1}} {
		importer := newImportAuthorizationTestImporter()
		importer.bindReply = bindReply
		importer.bindPersists = false

		result, err := newImportAuthorizationCore().importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
		if result != nil || err != mtproto.ErrInternalServerError {
			t.Fatalf("unconfirmed bind (%v) = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", bindReply, result, err)
		}
		if importer.authorization.State != dao.ExportedAuthorizationBinding {
			t.Fatalf("unconfirmed bind consumed credential: state=%d, want Binding", importer.authorization.State)
		}
		if importer.completeCalls != 0 {
			t.Fatalf("unconfirmed bind completed credential: complete calls=%d", importer.completeCalls)
		}
	}
}

func TestAuthImportAuthorizationRetriesAfterCompletionResponseLoss(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	completionFailure := errors.New("completion response lost")
	importer.completeErr = completionFailure
	importer.completeErrAfterCommit = true
	core := newImportAuthorizationCore()

	result, err := core.importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
	if result != nil || !errors.Is(err, completionFailure) {
		t.Fatalf("first import = (%v, %v), want completion transport failure", result, err)
	}
	if importer.authorization.State != dao.ExportedAuthorizationComplete {
		t.Fatalf("credential state after lost completion response = %d, want Complete", importer.authorization.State)
	}

	result, err = core.importAuthorization(importAuthorizationRequest(), importAuthorizationTargetDCID, importer)
	if result == nil || err != nil {
		t.Fatalf("retry import = (%v, %v), want authorization, nil", result, err)
	}
	if importer.claimCalls != 1 || importer.completeCalls != 2 {
		t.Fatalf("claim/complete calls = %d/%d, want 1/2", importer.claimCalls, importer.completeCalls)
	}
}

func TestAuthImportAuthorizationRejectsWrongTargetDC(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	result, err := newImportAuthorizationCore().importAuthorization(importAuthorizationRequest(), 3, importer)
	if result != nil || err != mtproto.ErrAuthBytesInvalid {
		t.Fatalf("wrong-DC import = (%v, %v), want AUTH_BYTES_INVALID", result, err)
	}
	if importer.claimCalls != 0 {
		t.Fatalf("wrong-DC claim calls = %d, want 0", importer.claimCalls)
	}
}

func TestAuthImportAuthorizationAcceptsConfiguredSharedTarget(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	core := newImportAuthorizationCore()
	supports := func(dcID int32) bool { return dcID == 1 || dcID == 2 }

	result, err := core.importAuthorizationWithSupport(importAuthorizationRequest(), 1, supports, importer)
	if result == nil || err != nil {
		t.Fatalf("shared-DC import = (%v, %v), want authorization, nil", result, err)
	}
	if importer.claimCalls != 1 || importer.authorization.TargetAuthKeyID != importAuthorizationTargetAuthKeyID {
		t.Fatalf("shared-DC claim = calls %d, authorization %+v", importer.claimCalls, importer.authorization)
	}
}

func TestAuthImportAuthorizationRejectsUnconfiguredSharedTargetBeforeClaim(t *testing.T) {
	importer := newImportAuthorizationTestImporter()
	core := newImportAuthorizationCore()

	result, err := core.importAuthorizationWithSupport(importAuthorizationRequest(), 1, func(dcID int32) bool {
		return dcID == 1
	}, importer)
	if result != nil || err != mtproto.ErrAuthBytesInvalid {
		t.Fatalf("unconfigured shared-DC import = (%v, %v), want AUTH_BYTES_INVALID", result, err)
	}
	if importer.claimCalls != 0 {
		t.Fatalf("unconfigured shared-DC claim calls = %d, want 0", importer.claimCalls)
	}
}

func importAuthorizationRequest() *mtproto.TLAuthImportAuthorization {
	return &mtproto.TLAuthImportAuthorization{
		Id:    4004,
		Bytes: make([]byte, dao.ExportedAuthorizationTokenSize),
	}
}

func newImportAuthorizationCore() *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: importAuthorizationTargetAuthKeyID},
	}
}

type importAuthorizationTestImporter struct {
	authorization          *dao.ExportedAuthorization
	targetUserID           int64
	sourceUserID           int64
	user                   *mtproto.ImmutableUser
	claimCalls             int
	completeCalls          int
	bindCalls              int
	bindErr                error
	bindReply              *mtproto.Int64
	bindPersists           bool
	completeErr            error
	completeErrAfterCommit bool
}

func newImportAuthorizationTestImporter() *importAuthorizationTestImporter {
	return &importAuthorizationTestImporter{
		authorization: &dao.ExportedAuthorization{
			ID:              4004,
			UserID:          importAuthorizationUserID,
			SourceAuthKeyID: importAuthorizationSourceAuthKeyID,
			SourceDCID:      importAuthorizationSourceDCID,
			TargetDCID:      importAuthorizationTargetDCID,
			State:           dao.ExportedAuthorizationReady,
		},
		sourceUserID: importAuthorizationUserID,
		bindReply:    &mtproto.Int64{V: 1},
		bindPersists: true,
		user: &mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: importAuthorizationUserID},
		},
	}
}

func (f *importAuthorizationTestImporter) GetExportedAuthorization(_ context.Context, _ []byte) (*dao.ExportedAuthorization, error) {
	return copyExportedAuthorization(f.authorization), nil
}

func (f *importAuthorizationTestImporter) ClaimExportedAuthorization(_ context.Context, _ []byte, targetAuthKeyID int64, targetDCID int32) (*dao.ExportedAuthorization, error) {
	f.claimCalls++
	if f.authorization == nil || f.authorization.TargetDCID != targetDCID || f.authorization.State == dao.ExportedAuthorizationComplete {
		return nil, nil
	}
	if f.authorization.State == dao.ExportedAuthorizationReady {
		f.authorization.State = dao.ExportedAuthorizationBinding
		f.authorization.TargetAuthKeyID = targetAuthKeyID
	}
	if f.authorization.TargetAuthKeyID != targetAuthKeyID {
		return nil, nil
	}
	return copyExportedAuthorization(f.authorization), nil
}

func (f *importAuthorizationTestImporter) CompleteExportedAuthorization(_ context.Context, _ []byte, targetAuthKeyID int64) (bool, error) {
	f.completeCalls++
	if f.authorization == nil || f.authorization.State != dao.ExportedAuthorizationBinding || f.authorization.TargetAuthKeyID != targetAuthKeyID {
		if f.authorization != nil && f.authorization.State == dao.ExportedAuthorizationComplete && f.authorization.TargetAuthKeyID == targetAuthKeyID {
			return true, nil
		}
		return false, nil
	}
	if f.completeErr != nil {
		err := f.completeErr
		if f.completeErrAfterCommit {
			f.authorization.State = dao.ExportedAuthorizationComplete
			f.completeErrAfterCommit = false
		}
		f.completeErr = nil
		return false, err
	}
	f.authorization.State = dao.ExportedAuthorizationComplete
	return true, nil
}

func (f *importAuthorizationTestImporter) AuthsessionGetUserId(_ context.Context, in *authsession.TLAuthsessionGetUserId) (*mtproto.Int64, error) {
	switch in.GetAuthKeyId() {
	case importAuthorizationTargetAuthKeyID:
		return &mtproto.Int64{V: f.targetUserID}, nil
	case importAuthorizationSourceAuthKeyID:
		return &mtproto.Int64{V: f.sourceUserID}, nil
	default:
		return nil, nil
	}
}

func (f *importAuthorizationTestImporter) UserGetImmutableUser(_ context.Context, _ *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return f.user, nil
}

func (f *importAuthorizationTestImporter) AuthsessionBindAuthKeyUser(_ context.Context, _ *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	f.bindCalls++
	if f.bindErr != nil {
		return nil, f.bindErr
	}
	if f.bindPersists {
		f.targetUserID = importAuthorizationUserID
	}
	return f.bindReply, nil
}

func copyExportedAuthorization(authorization *dao.ExportedAuthorization) *dao.ExportedAuthorization {
	if authorization == nil {
		return nil
	}
	copy := *authorization
	return &copy
}
