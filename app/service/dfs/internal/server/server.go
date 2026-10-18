/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package server

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/server/http"
	"github.com/zeromicro/go-zero/rest"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/config"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/dfs.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
	httpSrv *rest.Server
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c)
	if err := applyObjectStorageEnv(&c); err != nil {
		return err
	}

	logx.Infow("dfs configuration loaded",
		logx.Field("s3_endpoint", c.Minio.Endpoint),
		logx.Field("s3_use_ssl", c.Minio.UseSSL),
		logx.Field("s3_region", c.Minio.Region),
		logx.Field("s3_buckets", c.Minio.Bucket),
	)
	ctx := svc.NewServiceContext(c)

	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)
	go func() {
		s.grpcSrv.Start()
	}()

	s.httpSrv = http.New(ctx, c.MiniHttp)

	return nil
}

func applyObjectStorageEnv(c *config.Config) error {
	if endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT")); endpoint != "" {
		if err := validateObjectStorageEndpoint(endpoint); err != nil {
			return err
		}
		c.Minio.Endpoint = endpoint
	}
	if accessKey := os.Getenv("S3_ACCESS_KEY"); accessKey != "" {
		c.Minio.AccessKeyID = accessKey
	}
	if secretKey := os.Getenv("S3_SECRET_KEY"); secretKey != "" {
		c.Minio.SecretAccessKey = secretKey
	}
	if useSSL := os.Getenv("S3_USE_SSL"); useSSL != "" {
		parsed, err := strconv.ParseBool(useSSL)
		if err != nil {
			return fmt.Errorf("S3_USE_SSL must be true or false: %w", err)
		}
		c.Minio.UseSSL = parsed
	}
	if region := strings.TrimSpace(os.Getenv("S3_REGION")); region != "" {
		if strings.ContainsAny(region, " \t\r\n") {
			return fmt.Errorf("S3_REGION must not contain whitespace")
		}
		c.Minio.Region = region
	}
	if c.Minio.Region == "" {
		c.Minio.Region = "us-east-1"
	}
	if c.Minio.Endpoint == "" {
		return fmt.Errorf("S3_ENDPOINT or Minio.Endpoint is required")
	}
	if c.Minio.AccessKeyID == "" || c.Minio.SecretAccessKey == "" {
		return fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY or config credentials are required")
	}
	return nil
}

func validateObjectStorageEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("S3_ENDPOINT must be a host:port value")
	}
	if strings.ContainsAny(endpoint, "/?#@") || strings.Contains(endpoint, "://") {
		return fmt.Errorf("S3_ENDPOINT must be a host:port value without a URL scheme, path, query, fragment, or credentials")
	}
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	s.grpcSrv.Stop()
}
