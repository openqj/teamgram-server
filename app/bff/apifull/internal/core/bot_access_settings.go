package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// botAccessTarget limits access-policy changes to the authenticated bot. The
// target InputUser is still checked against the authoritative user service so
// a stale or forged access hash cannot address another account.
func (c *ApiFullCore) botAccessTarget(in *mtproto.InputUser) (int64, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	if in.GetPredicateName() == mtproto.Predicate_inputUser {
		if in.GetUserId() != uid || in.GetAccessHash() == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
	} else if in.GetPredicateName() != mtproto.Predicate_inputUserSelf {
		return 0, mtproto.ErrUserIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: uid})
	if err != nil {
		return 0, err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetBot() == nil || profile.Deleted() {
		return 0, mtproto.ErrBotInvalid
	}
	if in.GetPredicateName() == mtproto.Predicate_inputUser && profile.GetUser().GetAccessHash() != in.GetAccessHash() {
		return 0, mtproto.ErrUserIdInvalid
	}
	return uid, nil
}

func (c *ApiFullCore) validateBotAccessUsers(uid int64, inputs []*mtproto.InputUser) ([]int64, error) {
	if len(inputs) == 0 {
		return []int64{}, nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	ids := make([]int64, 0, len(inputs))
	seen := make(map[int64]int64, len(inputs))
	for _, input := range inputs {
		if input == nil || input.GetPredicateName() != mtproto.Predicate_inputUser ||
			input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		if previous, ok := seen[input.GetUserId()]; ok {
			if previous != input.GetAccessHash() {
				return nil, mtproto.ErrUserIdInvalid
			}
			continue
		}
		seen[input.GetUserId()] = input.GetAccessHash()
		ids = append(ids, input.GetUserId())
	}
	for _, id := range ids {
		profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: id})
		if err != nil {
			return nil, err
		}
		if profile == nil || profile.GetUser() == nil || profile.Deleted() || profile.GetUser().GetId() != id {
			return nil, mtproto.ErrUserIdInvalid
		}
		if profile.GetUser().GetAccessHash() != seen[id] {
			return nil, mtproto.ErrUserIdInvalid
		}
	}
	return ids, nil
}

func (c *ApiFullCore) botAccessUsers(uid int64, ids []int64) ([]*mtproto.User, error) {
	users := make([]*mtproto.User, 0, len(ids))
	if len(ids) == 0 {
		return users, nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	for _, id := range ids {
		profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: id})
		if err != nil {
			return nil, err
		}
		if profile == nil || profile.GetUser() == nil || profile.Deleted() {
			return nil, mtproto.ErrUserIdInvalid
		}
		users = append(users, profile.ToUser(uid))
	}
	return users, nil
}

func (c *ApiFullCore) BotsGetAccessSettings(in *mtproto.TLBotsGetAccessSettings) (*mtproto.Bots_AccessSettings, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	uid, err := c.botAccessTarget(in.GetBot())
	if err != nil {
		return nil, err
	}
	state, err := persist.GetBotAccessSettings(c.ctx, uid)
	if err != nil {
		return nil, err
	}
	users, err := c.botAccessUsers(uid, state.AddUserIDs)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLBotsAccessSettings(&mtproto.Bots_AccessSettings{
		Restricted: state.Restricted,
		AddUsers:   users,
	}).To_Bots_AccessSettings(), nil
}

func (c *ApiFullCore) BotsEditAccessSettings(in *mtproto.TLBotsEditAccessSettings) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if !persist.PostgresEnabled() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	uid, err := c.botAccessTarget(in.GetBot())
	if err != nil {
		return nil, err
	}
	var ids []int64
	if in.GetAddUsers() != nil {
		ids, err = c.validateBotAccessUsers(uid, in.GetAddUsers())
		if err != nil {
			return nil, err
		}
	}
	if _, err = persist.EditBotAccessSettings(c.ctx, uid, in.GetRestricted(), ids); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
