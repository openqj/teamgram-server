package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestValidateWriteFilePartData(t *testing.T) {
	tests := []struct {
		name string
		in   *dfs.TLDfsWriteFilePartData
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty bytes", in: &dfs.TLDfsWriteFilePartData{Creator: 1, FileId: 2, Bytes: nil}, want: mtproto.ErrFilePartEmpty},
		{name: "invalid part", in: &dfs.TLDfsWriteFilePartData{Creator: 1, FileId: 2, FilePart: 3000, Bytes: []byte{1}}, want: mtproto.ErrFilePartInvalid},
		{name: "missing total for big file", in: &dfs.TLDfsWriteFilePartData{Creator: 1, FileId: 2, Big: true, Bytes: []byte{1}}, want: mtproto.ErrFilePartsInvalid},
		{name: "part outside total", in: &dfs.TLDfsWriteFilePartData{Creator: 1, FileId: 2, FilePart: 2, Big: true, FileTotalParts: wrapperspb.Int32(2), Bytes: []byte{1}}, want: mtproto.ErrFilePartInvalid},
		{name: "oversized bytes", in: &dfs.TLDfsWriteFilePartData{Creator: 1, FileId: 2, Bytes: make([]byte, maxFilePartBytes+1)}, want: mtproto.ErrFilePartTooBig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateWriteFilePartData(tt.in); !errors.Is(err, tt.want) {
				t.Fatalf("validateWriteFilePartData() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateWriteFilePartDataAcceptsLastBigPart(t *testing.T) {
	in := &dfs.TLDfsWriteFilePartData{
		Creator:        1,
		FileId:         2,
		FilePart:       1,
		Bytes:          []byte{1},
		Big:            true,
		FileTotalParts: wrapperspb.Int32(2),
	}
	if err := validateWriteFilePartData(in); err != nil {
		t.Fatalf("validateWriteFilePartData() = %v, want nil", err)
	}
}
