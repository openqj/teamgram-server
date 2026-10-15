package domain

import (
	"errors"
	"testing"
)

func TestValidateChannelInviteExpiresAtBoundary(t *testing.T) {
	err := validateChannelInvite(ChannelInvite{
		AdminID:    7,
		ExpireDate: 100,
	}, Channel{Creator: 7}, 100)
	if !errors.Is(err, ErrChannelInviteExpired) {
		t.Fatalf("expiry boundary error = %v, want ErrChannelInviteExpired", err)
	}
}
