package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userdao "github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserContactHandlersRejectNilRequests(t *testing.T) {
	c := New(context.Background(), &svc.ServiceContext{Dao: &userdao.Dao{}})
	tests := []struct {
		name string
		call func() error
	}{
		{"add contact", func() error { _, err := c.UserAddContact(nil); return err }},
		{"block peer", func() error { _, err := c.UserBlockPeer(nil); return err }},
		{"blocked by user", func() error { _, err := c.UserBlockedByUser(nil); return err }},
		{"check block list", func() error { _, err := c.UserCheckBlockUserList(nil); return err }},
		{"check contact", func() error { _, err := c.UserCheckContact(nil); return err }},
		{"get contact", func() error { _, err := c.UserGetContact(nil); return err }},
		{"get contact ids", func() error { _, err := c.UserGetContactIdList(nil); return err }},
		{"get contact list", func() error { _, err := c.UserGetContactList(nil); return err }},
		{"delete contact", func() error { _, err := c.UserDeleteContact(nil); return err }},
		{"get signup notification", func() error { _, err := c.UserGetContactSignUpNotification(nil); return err }},
		{"get birthdays", func() error { _, err := c.UserGetBirthdays(nil); return err }},
		{"get last seen", func() error { _, err := c.UserGetLastSeen(nil); return err }},
		{"get last seens", func() error { _, err := c.UserGetLastSeens(nil); return err }},
		{"update last seen", func() error { _, err := c.UserUpdateLastSeen(nil); return err }},
		{"is blocked", func() error { _, err := c.UserIsBlockedByUser(nil); return err }},
		{"set signup notification", func() error { _, err := c.UserSetContactSignUpNotification(nil); return err }},
		{"unblock peer", func() error { _, err := c.UserUnBlockPeer(nil); return err }},
		{"edit close friends", func() error { _, err := c.UserEditCloseFriends(nil); return err }},
		{"get blocked list", func() error { _, err := c.UserGetBlockedList(nil); return err }},
		{"is bot", func() error { _, err := c.UserIsBot(nil); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
				t.Fatalf("error = %v, want INPUT_REQUEST_INVALID", err)
			}
		})
	}
}

func TestUserContactHandlersFailClosedWithoutPostgres(t *testing.T) {
	c := New(context.Background(), &svc.ServiceContext{Dao: &userdao.Dao{}})
	tests := []struct {
		name string
		call func() error
	}{
		{"add contact", func() error { _, err := c.UserAddContact(&user.TLUserAddContact{UserId: 1, Id: 2}); return err }},
		{"block peer", func() error {
			_, err := c.UserBlockPeer(&user.TLUserBlockPeer{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2})
			return err
		}},
		{"blocked by user", func() error {
			_, err := c.UserBlockedByUser(&user.TLUserBlockedByUser{UserId: 1, PeerUserId: 2})
			return err
		}},
		{"check block list", func() error {
			_, err := c.UserCheckBlockUserList(&user.TLUserCheckBlockUserList{UserId: 1})
			return err
		}},
		{"check contact", func() error { _, err := c.UserCheckContact(&user.TLUserCheckContact{UserId: 1, Id: 2}); return err }},
		{"get contact", func() error { _, err := c.UserGetContact(&user.TLUserGetContact{UserId: 1, Id: 2}); return err }},
		{"get contact ids", func() error { _, err := c.UserGetContactIdList(&user.TLUserGetContactIdList{UserId: 1}); return err }},
		{"get contact list", func() error { _, err := c.UserGetContactList(&user.TLUserGetContactList{UserId: 1}); return err }},
		{"delete contact", func() error { _, err := c.UserDeleteContact(&user.TLUserDeleteContact{UserId: 1, Id: 2}); return err }},
		{"get signup notification", func() error {
			_, err := c.UserGetContactSignUpNotification(&user.TLUserGetContactSignUpNotification{UserId: 1})
			return err
		}},
		{"get birthdays", func() error { _, err := c.UserGetBirthdays(&user.TLUserGetBirthdays{UserId: 1}); return err }},
		{"get last seen", func() error { _, err := c.UserGetLastSeen(&user.TLUserGetLastSeen{Id: 1}); return err }},
		{"get last seens", func() error { _, err := c.UserGetLastSeens(&user.TLUserGetLastSeens{Id: []int64{1}}); return err }},
		{"update last seen", func() error { _, err := c.UserUpdateLastSeen(&user.TLUserUpdateLastSeen{Id: 1}); return err }},
		{"is blocked", func() error {
			_, err := c.UserIsBlockedByUser(&user.TLUserIsBlockedByUser{UserId: 1, PeerUserId: 2})
			return err
		}},
		{"set signup notification", func() error {
			_, err := c.UserSetContactSignUpNotification(&user.TLUserSetContactSignUpNotification{UserId: 1})
			return err
		}},
		{"unblock peer", func() error {
			_, err := c.UserUnBlockPeer(&user.TLUserUnBlockPeer{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2})
			return err
		}},
		{"edit close friends", func() error { _, err := c.UserEditCloseFriends(&user.TLUserEditCloseFriends{UserId: 1}); return err }},
		{"get blocked list", func() error { _, err := c.UserGetBlockedList(&user.TLUserGetBlockedList{UserId: 1}); return err }},
		{"is bot", func() error { _, err := c.UserIsBot(&user.TLUserIsBot{Id: 1}); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("error = %v, want METHOD_NOT_IMPL", err)
			}
		})
	}
}
