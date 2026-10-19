/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2024-present, Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package server

import (
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/server/http"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/httpserver.yaml", "the config file")

type Server struct {
	httpSrv *rest.Server
	ctx     *svc.ServiceContext
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if err := applyPaymentProviderEnvironment(&c); err != nil {
		return errors.Wrap(err, "load payment provider environment")
	}
	if err := applyIdentityEnvironment(&c); err != nil {
		return errors.Wrap(err, "load Identity environment")
	}

	logx.Infof("httpserver config loaded")
	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		return errors.Wrap(err, "create httpserver service context")
	}

	s.httpSrv = http.New(ctx, c.Http, splitCorsOrigins(c.CorsOrigins)...)
	s.ctx = ctx

	return nil
}

func splitCorsOrigins(value string) []string {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func applyIdentityEnvironment(c *config.Config) error {
	if value, ok := os.LookupEnv("TEAMGRAM_CORS_ORIGINS"); ok {
		c.CorsOrigins = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_IDENTITY_BASE_URL"); ok {
		c.IdentityBaseURL = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_IDENTITY_TEAMGRAM_SERVICE_TOKEN"); ok {
		c.IdentityTeamgramServiceToken = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_IDENTITY_INSTANCE_ID"); ok {
		c.IdentityTeamgramInstanceID = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_SUB2API_SERVICE_TOKEN"); ok {
		c.Sub2APIServiceToken = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_IDENTITY_REPLAY_TTL_SECONDS"); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed <= 0 || parsed > 300 {
			return errors.Errorf("TEAMGRAM_IDENTITY_REPLAY_TTL_SECONDS must be between 1 and 300")
		}
		c.IdentityReplayTTLSeconds = parsed
	}
	if value, ok := os.LookupEnv("TEAMGRAM_IDENTITY_REQUEST_TIMEOUT_SECONDS"); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed <= 0 {
			return errors.Errorf("TEAMGRAM_IDENTITY_REQUEST_TIMEOUT_SECONDS must be a positive integer")
		}
		c.IdentityRequestTimeoutSeconds = parsed
	}
	return nil
}

func applyPaymentProviderEnvironment(c *config.Config) error {
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_NAME"); ok {
		c.PaymentProviderName = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY"); ok {
		c.PaymentProviderWebhookSigningKey = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_MAX_BODY_BYTES"); ok {
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || parsed <= 0 {
			return errors.Errorf("TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_MAX_BODY_BYTES must be a positive integer")
		}
		c.PaymentProviderWebhookMaxBodyBytes = parsed
	}
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.httpSrv != nil {
		s.httpSrv.Stop()
	}
	if s.ctx != nil {
		s.ctx.Close()
		s.ctx = nil
	}
}
