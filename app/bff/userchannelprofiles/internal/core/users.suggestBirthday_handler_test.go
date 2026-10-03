package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestUsersSuggestBirthdayFailsClosedWithoutRecipientWorkflow(t *testing.T) {
	ctx := context.Background()
	c := &UserChannelProfilesCore{
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	got, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 99}).To_InputUser(),
		Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3}).To_Birthday(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("UsersSuggestBirthday() = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestUsersSuggestBirthdayKeepsInputValidation(t *testing.T) {
	ctx := context.Background()
	c := &UserChannelProfilesCore{
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	validRecipient := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 99}).To_InputUser()

	for _, tc := range []struct {
		name string
		in   *mtproto.TLUsersSuggestBirthday
		want error
	}{
		{
			name: "self recipient",
			in: &mtproto.TLUsersSuggestBirthday{
				Id:       mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
				Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3}).To_Birthday(),
			},
			want: mtproto.ErrUserIdInvalid,
		},
		{
			name: "missing birthday",
			in: &mtproto.TLUsersSuggestBirthday{
				Id: validRecipient,
			},
			want: mtproto.ErrInputRequestInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.UsersSuggestBirthday(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("UsersSuggestBirthday() = (%+v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}
