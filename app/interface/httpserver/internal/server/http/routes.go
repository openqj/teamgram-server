// Copyright (c) 2024-present, Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package http

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/core"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func RegisterHandlers(engine *rest.Server, serverCtx *svc.ServiceContext) {
	engine.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/v1/internal/aimemory/token",
				Handler: aimemoryTokenHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/payment/provider/webhook",
				Handler: paymentProviderWebhookHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/file/:token/:kind/:file_name",
				Handler: botAPIFileHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/bot/:token/:method",
				Handler: botAPIHandler(serverCtx),
			},
			{
				// Telegram's canonical Bot API URL joins the "bot" prefix and
				// token (for example, /bot123:ABC/getMe). Keep the slash form
				// above for existing deployments and accept the canonical form
				// through a two-segment dynamic route.
				Method:  http.MethodGet,
				Path:    "/:token/:method",
				Handler: botAPIHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/bot/:token/:method",
				Handler: botAPIHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/:token/:method",
				Handler: botAPIHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/api",
				Handler: apiw1(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/apiw1",
				Handler: apiw1(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/apiw1_test",
				Handler: apiw1(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/apiw1_premium",
				Handler: apiw1(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/apiw2",
				Handler: apiw1Debug(serverCtx),
			},
		},
	)
}

func aimemoryTokenHandler(serverCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if serverCtx == nil || serverCtx.Dao == nil {
			http.Error(w, "AIMemory token exchange is unavailable", http.StatusServiceUnavailable)
			return
		}
		if serverCtx.AuthsessionClient == nil || serverCtx.AIMemoryTokenExchanger == nil {
			http.Error(w, "AIMemory token exchange is not configured", http.StatusNotFound)
			return
		}
		if r.Body == nil {
			http.Error(w, "invalid MTProto proof", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid MTProto proof", http.StatusRequestEntityTooLarge)
			return
		}
		if len(body) < 8 {
			http.Error(w, "invalid MTProto proof", http.StatusUnauthorized)
			return
		}
		authKeyID := int64(binary.LittleEndian.Uint64(body[:8]))
		message := mtproto.NewMTPRawMessage(authKeyID, 0, core.TRANSPORT_HTTP)
		if err := message.Decode(body); err != nil {
			http.Error(w, "invalid MTProto proof", http.StatusUnauthorized)
			return
		}

		c := core.New2(r.Context(), serverCtx, &metadata.RpcMetadata{
			ServerId:    serverCtx.Dao.GetGatewayId(),
			ClientAddr:  httpx.GetRemoteAddr(r),
			AuthId:      authKeyID,
			ReceiveTime: time.Now().Unix(),
		})
		accessToken, expiresAt, err := c.IssueAIMemoryToken(message)
		if err != nil {
			status := http.StatusServiceUnavailable
			message := "AIMemory token exchange is unavailable"
			switch {
			case errors.Is(err, core.ErrAIMemoryTokenProof), errors.Is(err, core.ErrAIMemoryTokenReplay):
				status = http.StatusUnauthorized
				message = "invalid MTProto proof"
			case errors.Is(err, core.ErrAIMemoryTokenIdentity):
				status = http.StatusNotFound
				message = "Teamgram account is not bound to a product account"
			}
			http.Error(w, message, status)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_at":   expiresAt,
		})
	}
}

func apiw1(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength <= 0 || r.Body == nil {
			http.NotFound(w, r)
		} else {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.NotFound(w, r)
			} else {
				authKeyId := int64(binary.LittleEndian.Uint64(body))
				message := mtproto.NewMTPRawMessage(authKeyId, 0, 2)
				message.Decode(body)

				c := core.New2(
					r.Context(),
					ctx,
					&metadata.RpcMetadata{
						ServerId:      ctx.Dao.GetGatewayId(),
						ClientAddr:    httpx.GetRemoteAddr(r),
						AuthId:        authKeyId,
						SessionId:     0,
						ReceiveTime:   time.Now().Unix(),
						UserId:        0,
						ClientMsgId:   0,
						IsBot:         false,
						Layer:         0,
						Client:        "",
						IsAdmin:       false,
						Takeout:       nil,
						Langpack:      "",
						PermAuthKeyId: 0,
						LangCode:      "",
					})
				rData, err2 := c.HttpserverApiw1(message)
				if err2 != nil {
					http.Error(w, err2.Error(), http.StatusInternalServerError)
				} else {
					w.Header().Set("Access-Control-Allow-Headers", "origin, content-type")
					w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Access-Control-Max-Age", "1728000")
					w.Header().Set("Cache-control", "no-store")
					w.Header().Set("Connection", "keep-alive")
					w.Header().Set("Content-type", "application/octet-stream")
					w.Header().Set("Pragma", "no-cache")
					w.Header().Set("Strict-Transport-Security", "max-age=15768000")
					w.WriteHeader(200)
					w.Write(rData.Payload)
				}
			}
		}
	}
}

func apiw1Debug(ctx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r)
		http.NotFound(w, r)
	}
}
