package logic

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
)

func TestIsAuthSignUpStateAllowed(t *testing.T) {
	for _, state := range []int{model.CodeStateOk, model.CodeStateSignIn} {
		if !isAuthSignUpStateAllowed(state) {
			t.Fatalf("state %d should allow signup", state)
		}
	}
	for _, state := range []int{model.CodeStateSend, model.CodeStateSent, model.CodeStateDeleted, model.CodeStateSignUp} {
		if isAuthSignUpStateAllowed(state) {
			t.Fatalf("state %d should not allow signup", state)
		}
	}
}
