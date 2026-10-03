package gnet

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestNewHTTPSessionClientDataPreservesAuthKeyBinding(t *testing.T) {
	tests := []struct {
		name      string
		authKeyID int64
		keyType   int32
		permKeyID int64
	}{
		{name: "permanent key", authKeyID: 101, keyType: mtproto.AuthKeyTypePerm, permKeyID: 101},
		{name: "bound temporary key", authKeyID: 102, keyType: mtproto.AuthKeyTypeTemp, permKeyID: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newHTTPSessionClientData(
				&authKeyUtil{keyData: &mtproto.AuthKeyInfo{
					AuthKeyId:     tt.authKeyID,
					AuthKeyType:   tt.keyType,
					PermAuthKeyId: tt.permKeyID,
				}},
				"gateway",
				303,
				"127.0.0.1",
				404,
				[]byte{5},
			)

			if got.GetAuthKeyId() != tt.authKeyID || got.GetKeyType() != tt.keyType || got.GetPermAuthKeyId() != tt.permKeyID {
				t.Fatalf("auth metadata = (%d, %d, %d), want (%d, %d, %d)", got.GetAuthKeyId(), got.GetKeyType(), got.GetPermAuthKeyId(), tt.authKeyID, tt.keyType, tt.permKeyID)
			}
			if got.GetConnType() != 2 {
				t.Fatalf("ConnType = %d, want 2", got.GetConnType())
			}
		})
	}
}
