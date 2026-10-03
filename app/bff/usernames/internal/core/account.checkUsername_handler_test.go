package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/usernames/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type checkUsernameUserClient struct {
	user_client.UserClient
	response        *userpb.UsernameExist
	channelResponse *userpb.UsernameExist
	err             error
	channelErr      error
	calls           int
	channelCalls    int
}

func (f *checkUsernameUserClient) UserCheckAccountUsername(context.Context, *userpb.TLUserCheckAccountUsername) (*userpb.UsernameExist, error) {
	f.calls++
	return f.response, f.err
}

func (f *checkUsernameUserClient) UserCheckChannelUsername(context.Context, *userpb.TLUserCheckChannelUsername) (*userpb.UsernameExist, error) {
	f.channelCalls++
	return f.channelResponse, f.channelErr
}

func newCheckUsernameTestCore(client *checkUsernameUserClient) *UsernamesCore {
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}})
	c.MD = &metadata.RpcMetadata{UserId: 42}
	return c
}

func TestAccountCheckUsernameRequiresAuthenticationAndRequest(t *testing.T) {
	client := &checkUsernameUserClient{}
	c := newCheckUsernameTestCore(client)
	c.MD = &metadata.RpcMetadata{}

	if _, err := c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: "available"}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated error = %v, want %v", err, mtproto.ErrAuthKeyUnregistered)
	}
	c.MD.UserId = 42
	if _, err := c.AccountCheckUsername(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil request error = %v, want %v", err, mtproto.ErrInputRequestInvalid)
	}
	if client.calls != 0 {
		t.Fatalf("rejected requests reached user service %d times", client.calls)
	}
}

func TestAccountCheckUsernameRejectsInvalidFormatBeforeRPC(t *testing.T) {
	client := &checkUsernameUserClient{}
	c := newCheckUsernameTestCore(client)

	for _, username := range []string{"abc", "1invalid", "has-hyphen"} {
		if _, err := c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: username}); err != mtproto.ErrUsernameInvalid {
			t.Errorf("username %q: error = %v, want %v", username, err, mtproto.ErrUsernameInvalid)
		}
	}
	if client.calls != 0 {
		t.Fatalf("invalid usernames reached user service %d times", client.calls)
	}
}

func TestAccountCheckUsernameReturnsProviderState(t *testing.T) {
	tests := []struct {
		name     string
		response *userpb.UsernameExist
		want     *mtproto.Bool
	}{
		{name: "available", response: userpb.MakeTLUsernameNotExisted(nil).To_UsernameExist(), want: mtproto.BoolTrue},
		{name: "owned", response: userpb.MakeTLUsernameExistedIsMe(nil).To_UsernameExist(), want: mtproto.BoolTrue},
		{name: "occupied", response: userpb.MakeTLUsernameExistedNotMe(nil).To_UsernameExist(), want: mtproto.BoolFalse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &checkUsernameUserClient{response: tt.response}
			c := newCheckUsernameTestCore(client)
			got, err := c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: "available"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("reply = %v, want %v", got, tt.want)
			}
			if client.calls != 1 {
				t.Fatalf("provider calls = %d, want 1", client.calls)
			}
		})
	}
}

func TestAccountCheckUsernameFailsClosedOnProviderResponse(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response *userpb.UsernameExist
		err      error
	}{
		{name: "empty response"},
		{name: "provider error", err: errors.New("user service unavailable")},
		{name: "unknown predicate", response: &userpb.UsernameExist{PredicateName: "unexpected"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &checkUsernameUserClient{response: tt.response, err: tt.err}
			c := newCheckUsernameTestCore(client)
			_, err := c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: "available"})
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("error = %v, want %v", err, tt.err)
				}
			} else if err == nil {
				t.Fatal("malformed provider response was reported as success")
			}
		})
	}
}

func TestChannelsCheckUsernameReturnsProviderState(t *testing.T) {
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 9001, AccessHash: 1}).To_InputChannel()
	for _, tt := range []struct {
		name     string
		response *userpb.UsernameExist
		want     *mtproto.Bool
	}{
		{name: "available", response: userpb.MakeTLUsernameNotExisted(nil).To_UsernameExist(), want: mtproto.BoolTrue},
		{name: "owned", response: userpb.MakeTLUsernameExistedIsMe(nil).To_UsernameExist(), want: mtproto.BoolTrue},
		{name: "occupied", response: userpb.MakeTLUsernameExistedNotMe(nil).To_UsernameExist(), want: mtproto.BoolFalse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &checkUsernameUserClient{channelResponse: tt.response}
			c := newCheckUsernameTestCore(client)
			got, err := c.ChannelsCheckUsername(&mtproto.TLChannelsCheckUsername{
				Channel:  channel,
				Username: "available",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("reply = %v, want %v", got, tt.want)
			}
			if client.channelCalls != 1 {
				t.Fatalf("provider calls = %d, want 1", client.channelCalls)
			}
		})
	}
}

func TestChannelsCheckUsernameValidatesAuthInputAndProviderResponse(t *testing.T) {
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 9002, AccessHash: 1}).To_InputChannel()
	client := &checkUsernameUserClient{}
	c := newCheckUsernameTestCore(client)
	c.MD = &metadata.RpcMetadata{}
	if _, err := c.ChannelsCheckUsername(&mtproto.TLChannelsCheckUsername{Channel: channel, Username: "available"}); err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("unauthenticated error = %v, want %v", err, mtproto.ErrAuthKeyUnregistered)
	}
	c.MD.UserId = 42
	if _, err := c.ChannelsCheckUsername(nil); err != mtproto.ErrInputRequestInvalid {
		t.Fatalf("nil request error = %v, want %v", err, mtproto.ErrInputRequestInvalid)
	}
	if _, err := c.ChannelsCheckUsername(&mtproto.TLChannelsCheckUsername{Channel: channel, Username: "abc"}); err != mtproto.ErrUsernameInvalid {
		t.Fatalf("invalid username error = %v, want %v", err, mtproto.ErrUsernameInvalid)
	}
	if client.channelCalls != 0 {
		t.Fatalf("rejected requests reached provider %d times", client.channelCalls)
	}

	client.channelResponse = nil
	if _, err := c.ChannelsCheckUsername(&mtproto.TLChannelsCheckUsername{Channel: channel, Username: "available"}); err != mtproto.ErrInternalServerError {
		t.Fatalf("empty provider response error = %v, want %v", err, mtproto.ErrInternalServerError)
	}
	client.channelResponse = &userpb.UsernameExist{PredicateName: "unexpected"}
	if _, err := c.ChannelsCheckUsername(&mtproto.TLChannelsCheckUsername{Channel: channel, Username: "available"}); err != mtproto.ErrInternalServerError {
		t.Fatalf("unknown provider response error = %v, want %v", err, mtproto.ErrInternalServerError)
	}
}
