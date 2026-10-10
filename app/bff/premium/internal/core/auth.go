package core

import "github.com/teamgram/proto/mtproto"

func (c *PremiumCore) requireUserId() (int64, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return c.MD.UserId, nil
}
