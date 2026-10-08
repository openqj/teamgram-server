/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright 2022 Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package authsession_helper

import (
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/server"
)

// PostgreSQL DAOs are exposed here so service consumers can adopt the
// independent pgx persistence boundary without importing internal packages.
type (
	PostgresDB              = postgres_dao.DB
	PostgresStore           = postgres_dao.Store
	PostgresAuthsDAO        = postgres_dao.AuthsDAO
	PostgresAuthKeysDAO     = postgres_dao.AuthKeysDAO
	PostgresAuthUsersDAO    = postgres_dao.AuthUsersDAO
	PostgresAuthKeyInfosDAO = postgres_dao.AuthKeyInfosDAO

	AuthsDO        = dataobject.AuthsDO
	AuthKeysDO     = dataobject.AuthKeysDO
	AuthUsersDO    = dataobject.AuthUsersDO
	AuthKeyInfosDO = dataobject.AuthKeyInfosDO
)

var (
	NewPostgresStore           = postgres_dao.NewStore
	NewPostgresAuthsDAO        = postgres_dao.NewAuthsDAO
	NewPostgresAuthKeysDAO     = postgres_dao.NewAuthKeysDAO
	NewPostgresAuthUsersDAO    = postgres_dao.NewAuthUsersDAO
	NewPostgresAuthKeyInfosDAO = postgres_dao.NewAuthKeyInfosDAO
)

var (
	New = server.New
)
