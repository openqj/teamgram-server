// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/media/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
)

var (
	configFile = flag.String("f", "etc/media.yaml", "the config file")
)

var (
	cacheId = int64(1401151020526600192)
)

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	svcCtx := svc.NewServiceContext(c)
	GetCacheDocument(svcCtx)
}

func GetCacheDocument(svcCtx *svc.ServiceContext) {
	ctx := context.Background()

	// fmt.Println(document)

	document2, err := svcCtx.Dao.GetDocumentById(ctx, cacheId)
	if err != nil {
		panic(err)
	}

	fmt.Println(document2)
}
