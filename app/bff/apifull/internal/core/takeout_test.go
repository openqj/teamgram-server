package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
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
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 902, Takeout: &metadata.Takeout{Id: 1}}}
	out, err := c.AccountInitTakeoutSession(nil)
	if err != nil {
		t.Fatal(err)
	}
	c.MD.Takeout.Id = out.GetId()
	got, err := c.MessagesGetSplitRanges(&mtproto.TLMessagesGetSplitRanges{})
	if err == nil {
		if got == nil || len(got.GetDatas()) != 0 {
			t.Fatalf("MessagesGetSplitRanges() = %#v, want an empty PostgreSQL result", got)
		}
		return
	}
	if got != nil {
		t.Fatalf("MessagesGetSplitRanges() = %#v, want nil", got)
	}
	if !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesGetSplitRanges() error = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestChannelsGetLeftChannelsUsesProviderOrFailsClosed(t *testing.T) {
	userID := time.Now().UnixNano()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID, Takeout: &metadata.Takeout{Id: 1}}}
	out, err := c.AccountInitTakeoutSession(nil)
	if err != nil {
		t.Fatal(err)
	}
	c.MD.Takeout.Id = out.GetId()
	got, err := c.ChannelsGetLeftChannels(&mtproto.TLChannelsGetLeftChannels{Offset: 1})
	if domain.Ready() {
		if err != nil || got == nil || len(got.GetChats()) != 0 {
			t.Fatalf("ChannelsGetLeftChannels() = (%#v, %v), want empty PostgreSQL result", got, err)
		}
		return
	}
	if got != nil {
		t.Fatalf("ChannelsGetLeftChannels() = %#v, want nil", got)
	}
	if !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("ChannelsGetLeftChannels() error = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestTakeoutExportRequiresMatchingActiveSession(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 901, Takeout: &metadata.Takeout{Id: 901}}}
	if _, err := c.MessagesGetSplitRanges(&mtproto.TLMessagesGetSplitRanges{}); !errors.Is(err, mtproto.ErrTakeoutRequired) {
		t.Fatalf("missing active session: got %v", err)
	}
	out, err := c.AccountInitTakeoutSession(nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.GetId() == c.MD.Takeout.Id {
		t.Fatal("test requires a mismatched takeout ID")
	}
	if _, err := c.MessagesGetSplitRanges(&mtproto.TLMessagesGetSplitRanges{}); !errors.Is(err, mtproto.ErrTakeoutRequired) {
		t.Fatalf("mismatched session: got %v", err)
	}
	c.MD.Takeout.Id = out.GetId()
	if _, err := c.AccountFinishTakeoutSession(&mtproto.TLAccountFinishTakeoutSession{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.MessagesGetSplitRanges(&mtproto.TLMessagesGetSplitRanges{}); !errors.Is(err, mtproto.ErrTakeoutRequired) {
		t.Fatalf("finished session: got %v", err)
	}
}
