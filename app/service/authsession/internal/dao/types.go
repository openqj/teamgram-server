package dao

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// These errors are part of the authsession RPC mapping. They are storage
// independent and therefore shared by the PostgreSQL runtime and handlers.
var (
	ErrAuthKeyOwnedByAnotherUser = errors.New("auth key is already owned by another user")
	ErrTempAuthKeyAlreadyBound   = errors.New("temporary auth key is already bound to another permanent key")
)

type BindUser struct {
	UserId               int64 `json:"user_id"`
	Hash                 int64 `json:"hash"`
	DateCreated          int64 `json:"date_created"`
	DateActivated        int64 `json:"date_activated"`
	AndroidPushSessionId int64 `json:"android_push_sessionId"`
}

type CacheAuthData struct {
	Client   *authsession.ClientSession `json:"client"`
	BindUser *BindUser                  `json:"bind_user,omitempty"`
}

func (c *CacheAuthData) ToAuthState() int {
	if c == nil {
		return mtproto.AuthStateNew
	}
	if c.Client == nil {
		return mtproto.AuthStateWaitInit
	}
	if c.BindUser == nil {
		return mtproto.AuthStateUnauthorized
	}
	return mtproto.AuthStateNormal
}

func (c *CacheAuthData) GetClient() *authsession.ClientSession {
	if c == nil {
		return nil
	}
	return c.Client
}

func (c *CacheAuthData) AuthKeyId() int64       { return c.GetClient().GetAuthKeyId() }
func (c *CacheAuthData) Layer() int32           { return c.GetClient().GetLayer() }
func (c *CacheAuthData) ApiId() int32           { return c.GetClient().GetApiId() }
func (c *CacheAuthData) DeviceModel() string    { return c.GetClient().GetDeviceModel() }
func (c *CacheAuthData) SystemVersion() string  { return c.GetClient().GetSystemVersion() }
func (c *CacheAuthData) AppVersion() string     { return c.GetClient().GetAppVersion() }
func (c *CacheAuthData) SystemLangCode() string { return c.GetClient().GetSystemLangCode() }
func (c *CacheAuthData) LangPack() string       { return c.GetClient().GetLangPack() }
func (c *CacheAuthData) LangCode() string       { return c.GetClient().GetLangCode() }
func (c *CacheAuthData) ClientIp() string       { return c.GetClient().GetIp() }
func (c *CacheAuthData) Proxy() string          { return c.GetClient().GetProxy() }
func (c *CacheAuthData) Params() string         { return c.GetClient().GetParams() }

func (c *CacheAuthData) UserId() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.UserId
}

func (c *CacheAuthData) DateCreated() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.DateCreated
}

func (c *CacheAuthData) DateActivated() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.DateActivated
}

func (c *CacheAuthData) Hash() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.Hash
}

func (c *CacheAuthData) AndroidPushSessionId() int64 {
	if c == nil || c.BindUser == nil {
		return 0
	}
	return c.BindUser.AndroidPushSessionId
}
