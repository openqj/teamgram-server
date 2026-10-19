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

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/s3util"
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
		logx.Field("s3_endpoint", c.S3.Endpoint),
		logx.Field("s3_use_ssl", c.S3.UseSSL),
		logx.Field("s3_region", c.S3.Region),
		logx.Field("s3_buckets", c.S3.Bucket),
		logx.Field("s3_max_idle_conns", c.S3.Transport.MaxIdleConns),
		logx.Field("s3_max_idle_conns_per_host", c.S3.Transport.MaxIdleConnsPerHost),
		logx.Field("s3_max_conns_per_host", c.S3.Transport.MaxConnsPerHost),
		logx.Field("s3_upload_part_size_mib", c.S3.Transport.UploadPartSizeMiB),
		logx.Field("s3_upload_concurrency", c.S3.Transport.UploadConcurrency),
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
		c.S3.Endpoint = endpoint
	}
	if accessKey := os.Getenv("S3_ACCESS_KEY"); accessKey != "" {
		c.S3.AccessKeyID = accessKey
	}
	if secretKey := os.Getenv("S3_SECRET_KEY"); secretKey != "" {
		c.S3.SecretAccessKey = secretKey
	}
	if useSSL := os.Getenv("S3_USE_SSL"); useSSL != "" {
		parsed, err := strconv.ParseBool(useSSL)
		if err != nil {
			return fmt.Errorf("S3_USE_SSL must be true or false: %w", err)
		}
		c.S3.UseSSL = parsed
	}
	if region := strings.TrimSpace(os.Getenv("S3_REGION")); region != "" {
		if strings.ContainsAny(region, " \t\r\n") {
			return fmt.Errorf("S3_REGION must not contain whitespace")
		}
		c.S3.Region = region
	}
	if c.S3.Region == "" {
		c.S3.Region = "us-east-1"
	}
	if err := applyS3TransportEnv(&c.S3.Transport); err != nil {
		return err
	}
	if c.S3.Endpoint == "" {
		return fmt.Errorf("S3_ENDPOINT or S3.Endpoint is required")
	}
	if c.S3.AccessKeyID == "" || c.S3.SecretAccessKey == "" {
		return fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY or config credentials are required")
	}
	return nil
}

func applyS3TransportEnv(transport *s3util.TransportConfig) error {
	if err := parsePositiveEnv("S3_MAX_IDLE_CONNS", &transport.MaxIdleConns, false); err != nil {
		return err
	}
	if err := parsePositiveEnv("S3_MAX_IDLE_CONNS_PER_HOST", &transport.MaxIdleConnsPerHost, false); err != nil {
		return err
	}
	if err := parsePositiveEnv("S3_MAX_CONNS_PER_HOST", &transport.MaxConnsPerHost, true); err != nil {
		return err
	}
	if err := parsePositiveEnv("S3_IDLE_CONN_TIMEOUT_SECONDS", &transport.IdleConnTimeoutSeconds, false); err != nil {
		return err
	}
	if err := parsePositiveEnv("S3_RESPONSE_HEADER_TIMEOUT_SECONDS", &transport.ResponseHeaderTimeoutSecs, false); err != nil {
		return err
	}
	if value := strings.TrimSpace(os.Getenv("S3_UPLOAD_PART_SIZE_MIB")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 5 {
			return fmt.Errorf("S3_UPLOAD_PART_SIZE_MIB must be an integer >= 5")
		}
		transport.UploadPartSizeMiB = parsed
	}
	if err := parsePositiveEnv("S3_UPLOAD_CONCURRENCY", &transport.UploadConcurrency, false); err != nil {
		return err
	}
	return nil
}

func parsePositiveEnv(name string, target *int, allowZero bool) error {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 || (!allowZero && parsed == 0) {
		if allowZero {
			return fmt.Errorf("%s must be a non-negative integer", name)
		}
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*target = parsed
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
