/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package svc

import (
	"context"
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
)

type ServiceContext struct {
	Config config.Config
	Dao    AuthSessionDAO
}

// AuthSessionDAO is the narrow public surface consumed by generated RPC
// handlers. Keeping it small lets the runtime use PostgreSQL without leaking
// a storage driver into the MTProto service layer.
type AuthSessionDAO interface {
	BindAuthKeyUser(context.Context, int64, int64) (int64, error)
	BindTempAuthKeyV2(context.Context, int64, int64, int32) error
	GetApiLayer(context.Context, int64) int32
	GetAuthKeyUserId(context.Context, int64) int64
	GetAuthorization(context.Context, int64) (*mtproto.Authorization, error)
	GetAuthorizations(context.Context, int64, int64) []*mtproto.Authorization
	GetCacheAuthData(context.Context, int64) (*dao.CacheAuthData, error)
	GetClient(context.Context, int64) string
	GetFutureSalts(context.Context, int64, int32) (*mtproto.TLFutureSalts, error)
	GetLangCode(context.Context, int64) string
	GetLangPack(context.Context, int64) string
	PutSaltCache(context.Context, int64, *mtproto.TLFutureSalt) error
	QueryAuthKeyV2(context.Context, int64) (*mtproto.AuthKeyInfo, error)
	ResetAuthorization(context.Context, int64, int64, int64) ([]int64, error)
	SetAndroidPushSessionId(context.Context, int64, int64, int64) error
	SetAuthKeyV2(context.Context, *mtproto.AuthKeyInfo, int32) error
	SetClientSessionInfo(context.Context, *authsession.ClientSession) error
	SetInitConnection(context.Context, *authsession.TLAuthsessionSetInitConnection) error
	SetLayer(context.Context, *authsession.TLAuthsessionSetLayer) error
	UnbindAuthUser(context.Context, int64, int64) error
}

// These forwarding methods preserve the generated handler surface while the
// concrete DAO is selected at runtime.
func (s *ServiceContext) GetCacheAuthData(ctx context.Context, keyID int64) (*dao.CacheAuthData, error) {
	return s.Dao.GetCacheAuthData(ctx, keyID)
}

func (s *ServiceContext) GetApiLayer(ctx context.Context, keyID int64) int32 {
	return s.Dao.GetApiLayer(ctx, keyID)
}
func (s *ServiceContext) GetLangPack(ctx context.Context, keyID int64) string {
	return s.Dao.GetLangPack(ctx, keyID)
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if c.Postgres.DSN == "" {
		return nil, errors.New("authsession: Postgres.DSN is required")
	}
	pg, err := dao.NewPostgres(c)
	if err != nil {
		return nil, err
	}
	return &ServiceContext{Config: c, Dao: pg}, nil
}
