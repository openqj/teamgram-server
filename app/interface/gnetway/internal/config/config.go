/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package config

import (
	"net"
	"strings"

	"github.com/teamgram/marmota/pkg/container2"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	RSAKey           []RSAKey
	Gnetway          *GnetwayConfig
	Session          zrpc.RpcClientConf
	UseStreamSession bool `json:",default=false"`
}

type RSAKey struct {
	KeyFile        string
	KeyFingerprint string
}

type GnetwayServer struct {
	Proto     string `json:",default=tcp,options=tcp|websocket|http"`
	PPV1      bool   `json:",optional"`
	Addresses []string
}

type GnetwayConfig struct {
	Server        []GnetwayServer
	Multicore     bool
	SendBuf       int
	ReceiveBuf    int
	AuthKeyCacheM int `json:",default=10"`
	// Admission and worker limits are process-local. Account quotas must still
	// be enforced by the authoritative session service when more than one
	// gateway replica is serving the same account.
	MaxConnections           int     `json:",default=10000"`
	MaxConnectionsPerIP      int     `json:",default=0"`
	ConnectionRatePerIP      float64 `json:",default=0"`
	ConnectionBurstPerIP     int     `json:",default=0"`
	MaxConnectionsPerAuthKey int     `json:",default=0"`
	MaxHandshakesInFlight    int     `json:",default=512"`
	HandshakeRatePerIP       float64 `json:",default=0"`
	HandshakeBurstPerIP      int     `json:",default=0"`
	WorkerPoolSize           int     `json:",default=512"`
	WebsocketMaxMessageBytes int     `json:",default=16777216"`
	TCPKeepAliveSeconds      int     `json:",default=60"`
	TCPNoDelay               bool    `json:",default=true"`
}

// Normalize applies the resource-control defaults even when the config is
// constructed directly in tests or by a caller that bypasses go-zero's YAML
// default handling.
func (c *GnetwayConfig) Normalize() {
	if c == nil {
		return
	}
	if c.MaxConnections <= 0 {
		c.MaxConnections = 10000
	}
	if c.MaxHandshakesInFlight <= 0 {
		c.MaxHandshakesInFlight = 512
	}
	if c.WorkerPoolSize <= 0 {
		c.WorkerPoolSize = 512
	}
	if c.WebsocketMaxMessageBytes <= 0 {
		c.WebsocketMaxMessageBytes = 16 * 1024 * 1024
	}
	if c.TCPKeepAliveSeconds < 0 {
		c.TCPKeepAliveSeconds = 0
	}
	if c.ConnectionRatePerIP > 0 && c.ConnectionBurstPerIP <= 0 {
		c.ConnectionBurstPerIP = int(c.ConnectionRatePerIP)
		if c.ConnectionBurstPerIP < 1 {
			c.ConnectionBurstPerIP = 1
		}
	}
	if c.HandshakeRatePerIP > 0 && c.HandshakeBurstPerIP <= 0 {
		c.HandshakeBurstPerIP = int(c.HandshakeRatePerIP)
		if c.HandshakeBurstPerIP < 1 {
			c.HandshakeBurstPerIP = 1
		}
	}
}

func (c GnetwayConfig) IsWebsocket(addr string) bool {
	for _, server := range c.Server {
		if server.Proto == "websocket" {
			for _, address := range server.Addresses {
				if matchListenAddress(address, addr) {
					return true
				}
			}
		}
	}
	return false
}

func (c GnetwayConfig) IsHttp(addr string) bool {
	for _, server := range c.Server {
		if server.Proto == "http" {
			for _, address := range server.Addresses {
				if matchListenAddress(address, addr) {
					return true
				}
			}
		}
	}
	return false
}

func (c GnetwayConfig) IsTcp(addr string) bool {
	for _, server := range c.Server {
		if server.Proto == "tcp" {
			for _, address := range server.Addresses {
				if matchListenAddress(address, addr) {
					return true
				}
			}
		}
	}
	return false
}

func (c GnetwayConfig) IsProxyProtocolV1(addr string) bool {
	for _, server := range c.Server {
		if server.PPV1 {
			for _, address := range server.Addresses {
				if matchListenAddress(address, addr) {
					return true
				}
			}
		}
	}
	return false
}

func matchListenAddress(configured, actual string) bool {
	if configured == actual {
		return true
	}

	configuredHost, configuredPort, configuredErr := net.SplitHostPort(configured)
	actualHost, actualPort, actualErr := net.SplitHostPort(actual)
	if configuredErr != nil || actualErr != nil || configuredPort != actualPort {
		return false
	}

	return configuredHost == "0.0.0.0" || configuredHost == "::" ||
		(strings.Trim(configuredHost, "[]") == strings.Trim(actualHost, "[]"))
}

func (c GnetwayConfig) ToAddresses() []string {
	var addresses []string
	for _, server := range c.Server {
		for _, address := range server.Addresses {
			listenAddress := "tcp://" + address
			if ok := container2.ContainsString(addresses, listenAddress); !ok {
				addresses = append(addresses, listenAddress)
			}
		}
	}
	return addresses
}
