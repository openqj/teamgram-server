/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright 2022 Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dfs_helper

import (
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/s3util"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/server/http"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type (
	Config         = config.Config
	ServiceContext = svc.ServiceContext
)

var (
	New               = service.New
	NewServiceContext = svc.NewServiceContext
	NewS3Helper       = dao.NewS3Helper
)

//func New(c Config) *service.Service {
//	return service.New(svc.NewServiceContext(c))
//}

type (
	S3Config        = s3util.S3Config
	DFSHelper       = dao.Dao
	S3Helper        = s3util.S3Util
	DfsHttpFileInfo = model.DfsHttpFileInfo
)

// imaging - imaging
var (
	OpenWebp   = imaging.OpenWebp
	DecodeWebp = imaging.DecodeWebp
	EncodeWebp = imaging.EncodeWebp

	Open       = imaging.Open
	Save       = imaging.Save
	Decode     = imaging.Decode
	Resize     = imaging.Resize
	EncodeJpeg = imaging.EncodeJpeg
	EncodePng  = imaging.EncodePng

	EncodeStripped = imaging.EncodeStripped

	ReSizeImage        = imaging.ReSizeImage
	ReSizeImageByImage = imaging.ReSizeImageByImage

	MustNewS3Client = s3util.MustNewS3Client

	GetStorageFileTypeConstructor = model.GetStorageFileTypeConstructor
)

func init() {
	zrpc.DontLogContentForMethod("/dfs.RPCDfs/dfs_writeFilePartData")
}

func NewDFSHelper(s3 *S3Config, idgen zrpc.RpcClientConf, ssdb kv.KvConf) *DFSHelper {
	return dao.NewDFSHelper(s3, idgen, ssdb)
}

// GetDfsFile - GetDfsFile
var (
	GetDfsFile = http.GetDfsFile
)
