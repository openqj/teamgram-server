package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestTakeoutUnauthed(t *testing.T) {
	c := &ApiFullCore{}
	checks := []func() error{
		func() error { _, err := c.AccountInitTakeoutSession(nil); return err },
		func() error { _, err := c.AccountFinishTakeoutSession(nil); return err },
		func() error { _, err := c.MessagesGetSplitRanges(nil); return err },
		func() error { _, err := c.ChannelsGetLeftChannels(nil); return err },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Fatalf("handler %d: got %v", i, err)
		}
	}
}

func TestTakeoutInitFinishRoundtrip(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	out, err := c.AccountInitTakeoutSession(&mtproto.TLAccountInitTakeoutSession{Contacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.GetId() == 0 {
		t.Fatalf("takeout: %+v", out)
	}
	ok, err := c.AccountFinishTakeoutSession(&mtproto.TLAccountFinishTakeoutSession{Success: true})
	if err != nil {
		t.Fatal(err)
	}
	if !mtproto.FromBool(ok) {
		t.Fatalf("finish: %+v", ok)
	}
	if _, err := c.AccountFinishTakeoutSession(&mtproto.TLAccountFinishTakeoutSession{}); !errors.Is(err, mtproto.ErrTakeoutRequired) {
		t.Fatalf("second finish: got %v", err)
	}
}

func TestMessagesGetSplitRangesFailsClosedWithoutRangeProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	got, err := c.MessagesGetSplitRanges(&mtproto.TLMessagesGetSplitRanges{})
	if got != nil {
		t.Fatalf("MessagesGetSplitRanges() = %#v, want nil", got)
	}
	if !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesGetSplitRanges() error = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestChannelsGetLeftChannelsFailsClosedWithoutHistoryProvider(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	got, err := c.ChannelsGetLeftChannels(&mtproto.TLChannelsGetLeftChannels{Offset: 1})
	if got != nil {
		t.Fatalf("ChannelsGetLeftChannels() = %#v, want nil", got)
	}
	if !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("ChannelsGetLeftChannels() error = %v, want METHOD_NOT_IMPL", err)
	}
}
