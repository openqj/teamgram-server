package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type getAuthorizationsDAO struct {
	svc.AuthSessionDAO
	key        *mtproto.AuthKeyInfo
	owner      int64
	listCalls  int
	listErr    error
	resetCalls int
}

func (d *getAuthorizationsDAO) QueryAuthKeyV2(context.Context, int64) (*mtproto.AuthKeyInfo, error) {
	return d.key, nil
}

func (d *getAuthorizationsDAO) GetAuthKeyUserId(context.Context, int64) int64 {
	return d.owner
}

func (d *getAuthorizationsDAO) GetAuthorizations(context.Context, int64, int64) []*mtproto.Authorization {
	d.listCalls++
	return []*mtproto.Authorization{}
}

func (d *getAuthorizationsDAO) GetAuthorizationsWithError(context.Context, int64, int64) ([]*mtproto.Authorization, error) {
	d.listCalls++
	return nil, d.listErr
}

func (d *getAuthorizationsDAO) ResetAuthorization(context.Context, int64, int64, int64) ([]int64, error) {
	d.resetCalls++
	return []int64{2001}, nil
}

func TestAuthsessionGetAuthorizationsRequiresCurrentKeyOwner(t *testing.T) {
	keyID := int64(1001)
	key := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{
		AuthKeyId: keyID, PermAuthKeyId: keyID,
	}).To_AuthKeyInfo()
	d := &getAuthorizationsDAO{key: key, owner: 7}
	c := &AuthsessionCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: d}, Logger: logx.WithContext(context.Background())}

	result, err := c.AuthsessionGetAuthorizations(&authsession.TLAuthsessionGetAuthorizations{
		UserId: 8, ExcludeAuthKeyId: keyID,
	})
	if result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("mismatched owner result=%v err=%v, want AUTH_KEY_UNREGISTERED", result, err)
	}
	if d.listCalls != 0 {
		t.Fatalf("authorization list queried %d times after owner mismatch", d.listCalls)
	}
}

func TestAuthsessionGetAuthorizationsAllowsCurrentKeyOwner(t *testing.T) {
	keyID := int64(1002)
	key := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{
		AuthKeyId: keyID, PermAuthKeyId: keyID,
	}).To_AuthKeyInfo()
	d := &getAuthorizationsDAO{key: key, owner: 7}
	c := &AuthsessionCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: d}, Logger: logx.WithContext(context.Background())}

	result, err := c.AuthsessionGetAuthorizations(&authsession.TLAuthsessionGetAuthorizations{
		UserId: 7, ExcludeAuthKeyId: keyID,
	})
	if err != nil || result == nil {
		t.Fatalf("matching owner result=%v err=%v", result, err)
	}
	if d.listCalls != 1 {
		t.Fatalf("authorization list queried %d times, want 1", d.listCalls)
	}
}

func TestAuthsessionGetAuthorizationsPropagatesPostgresError(t *testing.T) {
	keyID := int64(1003)
	key := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{
		AuthKeyId: keyID, PermAuthKeyId: keyID,
	}).To_AuthKeyInfo()
	storageErr := errors.New("postgres authorization query failed")
	d := &getAuthorizationsDAO{key: key, owner: 7, listErr: storageErr}
	c := &AuthsessionCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: d}, Logger: logx.WithContext(context.Background())}

	result, err := c.AuthsessionGetAuthorizations(&authsession.TLAuthsessionGetAuthorizations{
		UserId: 7, ExcludeAuthKeyId: keyID,
	})
	if result != nil || !errors.Is(err, storageErr) {
		t.Fatalf("storage failure result=%v err=%v, want propagated error", result, err)
	}
}

func TestAuthsessionResetAuthorizationRequiresCurrentKeyOwner(t *testing.T) {
	keyID := int64(1004)
	key := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{
		AuthKeyId: keyID, PermAuthKeyId: keyID,
	}).To_AuthKeyInfo()
	d := &getAuthorizationsDAO{key: key, owner: 7}
	c := &AuthsessionCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: d}, Logger: logx.WithContext(context.Background())}

	result, err := c.AuthsessionResetAuthorization(&authsession.TLAuthsessionResetAuthorization{
		UserId: 8, AuthKeyId: keyID,
	})
	if result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("mismatched owner result=%v err=%v, want AUTH_KEY_UNREGISTERED", result, err)
	}
	if d.resetCalls != 0 {
		t.Fatalf("reset called %d times after owner mismatch", d.resetCalls)
	}
}

func TestAuthsessionResetAuthorizationAllowsCurrentKeyOwner(t *testing.T) {
	keyID := int64(1005)
	key := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{
		AuthKeyId: keyID, PermAuthKeyId: keyID,
	}).To_AuthKeyInfo()
	d := &getAuthorizationsDAO{key: key, owner: 7}
	c := &AuthsessionCore{ctx: context.Background(), svcCtx: &svc.ServiceContext{Dao: d}, Logger: logx.WithContext(context.Background())}

	result, err := c.AuthsessionResetAuthorization(&authsession.TLAuthsessionResetAuthorization{
		UserId: 7, AuthKeyId: keyID,
	})
	if err != nil || result == nil || len(result.GetDatas()) != 1 || result.GetDatas()[0] != 2001 {
		t.Fatalf("matching owner result=%v err=%v, want reset vector", result, err)
	}
	if d.resetCalls != 1 {
		t.Fatalf("reset called %d times, want 1", d.resetCalls)
	}
}
