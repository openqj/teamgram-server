package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAuthExportAuthorizationRequiresAuthorizedSourceKey(t *testing.T) {
	result, err := (&AuthorizationCore{}).AuthExportAuthorization(&mtproto.TLAuthExportAuthorization{DcId: 2})
	if result != nil {
		t.Fatalf("AuthExportAuthorization() result = %v, want nil", result)
	}
	if err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("AuthExportAuthorization() error = %v, want AUTH_KEY_UNREGISTERED", err)
	}
}
