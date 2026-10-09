package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestHelpGetSupportNameReturnsTypedEmptyName(t *testing.T) {
	got, err := (&ConfigurationCore{}).HelpGetSupportName(&mtproto.TLHelpGetSupportName{})
	if err != nil {
		t.Fatalf("HelpGetSupportName() error = %v", err)
	}
	if got.GetPredicateName() != mtproto.Predicate_help_supportName || got.GetName() != "" {
		t.Fatalf("HelpGetSupportName() = %+v", got)
	}
	buf := mtproto.NewEncodeBuf(64)
	if err := got.Encode(buf, 228); err != nil {
		t.Fatalf("encode: %v", err)
	}
}
