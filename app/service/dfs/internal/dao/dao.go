/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/s3util"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	*s3util.S3Util
	idgen_client.IDGenClient2
	ssdb kv.ExtStore
}

func New(c config.Config) *Dao {
	return &Dao{
		S3Util:       s3util.MustNewS3Client(&c.S3),
		IDGenClient2: idgen_client.NewIDGenClient2(zrpc.MustNewClient(c.IdGen)),
		ssdb:         kv.NewStore(c.SSDB),
	}
}

func NewDFSHelper(s3 *s3util.S3Config, idgen zrpc.RpcClientConf, ssdb kv.KvConf) *Dao {
	return &Dao{
		S3Util:       s3util.MustNewS3Client(s3),
		IDGenClient2: idgen_client.NewIDGenClient2(zrpc.MustNewClient(idgen)),
		ssdb:         kv.NewStore(ssdb),
	}
}

func NewS3Helper(s3 *s3util.S3Config) *s3util.S3Util {
	return s3util.MustNewS3Client(s3)
}
