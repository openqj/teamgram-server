package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestCommunitiesRequireProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81002}}
	if _, err := c.CommunitiesCreate(&mtproto.TLCommunitiesCreate{Title: "harbor"}); !errors.Is(err, mtproto.ErrTitleInvalid) {
		t.Fatalf("communities create error = %v, want TITLE_INVALID", err)
	}
	calls := []func() error{
		func() error {
			_, err := c.CommunitiesGetJoinedCommunities(&mtproto.TLCommunitiesGetJoinedCommunities{})
			return err
		},
		func() error { _, err := c.CommunitiesTogglePeerLink(nil); return err },
		func() error { _, err := c.CommunitiesToggleCommunityCollapsedInDialogs(nil); return err },
		func() error { _, err := c.CommunitiesGetPeerLinkRequests(nil); return err },
		func() error { _, err := c.CommunitiesTogglePeerLinkRequestApproval(nil); return err },
		func() error { _, err := c.CommunitiesToggleAllPeerLinkRequestApproval(nil); return err },
		func() error { _, err := c.CommunitiesToggleParticipantBanned(nil); return err },
	}
	for _, call := range calls {
		if err := call(); err != nil && !errors.Is(err, mtproto.ErrMethodNotImpl) && !errors.Is(err, mtproto.ErrInputRequestInvalid) {
			t.Fatalf("communities error = %v, want success or validation/provider error", err)
		}
	}
}

func TestCommunitiesParticipantJoinedChatsValidatesInputBeforeProviderGap(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 81002}}
	if _, err := c.CommunitiesGetParticipantJoinedChats(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request error = %v, want INPUT_REQUEST_INVALID", err)
	}
	if _, err := c.CommunitiesGetParticipantJoinedChats(&mtproto.TLCommunitiesGetParticipantJoinedChats{}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("missing community error = %v, want CHANNEL_INVALID", err)
	}
}
