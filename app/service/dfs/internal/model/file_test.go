package model

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCheckFilePartBounds(t *testing.T) {
	for _, part := range []int32{-1, 3000} {
		if err := CheckFilePart(part); !errors.Is(err, mtproto.ErrFilePartInvalid) {
			t.Fatalf("CheckFilePart(%d) = %v, want FILE_PART_INVALID", part, err)
		}
	}
	if err := CheckFilePart(2999); err != nil {
		t.Fatalf("CheckFilePart(2999) = %v, want nil", err)
	}
}

func TestCheckFilePartSizeRejectsNonPositive(t *testing.T) {
	for _, size := range []int32{0, -1024} {
		if err := CheckFilePartSize(size); !errors.Is(err, mtproto.ErrFilePartLengthInvalid) {
			t.Fatalf("CheckFilePartSize(%d) = %v, want FILE_PART_LENGTH_INVALID", size, err)
		}
	}
}
