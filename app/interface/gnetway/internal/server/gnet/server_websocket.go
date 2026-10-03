// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package gnet

import (
	"encoding/binary"
	"errors"

	"github.com/gobwas/ws/wsutil"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/codec"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/server/gnet/ws"

	"github.com/panjf2000/gnet/v2"
	"github.com/zeromicro/go-zero/core/logx"
)

func (s *Server) onWebsocketData(ctx *connContext, c gnet.Conn) (action gnet.Action) {
	ws := ctx.wsCodec
	if ws.ReadBufferBytes(c) == gnet.Close {
		return gnet.Close
	}
	ok, action := ws.Upgrade(c)
	if !ok {
		return
	}

	if ws.Buf.Len() <= 0 {
		return gnet.None
	}
	messages, err := ws.Decode(c)
	if err != nil {
		return gnet.Close
	}
	if messages == nil {
		return
	}
	bufferWebsocketMessages(&ws.Conn, messages)

	if ctx.codec == nil {
		ctx.codec, err = codec.CreateCodec(&ctx.wsCodec.Conn)
		if err != nil {
			if errors.Is(err, codec.ErrUnexpectedEOF) {
				metricCodecDecodeError.Inc("websocket", classifyCodecError(err))
				return gnet.None
			}

			metricCodecDecodeError.Inc("websocket", classifyCodecError(err))
			logx.Errorf("conn(%s) create codec error: %v", c, err)
			return gnet.Close
		}
		if dcID, ok := codec.DCID(ctx.codec); ok {
			ctx.setDCID(dcID)
		}
	}

	for {
		needAck, frame, err := ctx.codec.Decode(&ws.Conn)
		if err != nil {
			if errors.Is(err, codec.ErrUnexpectedEOF) {
				metricCodecDecodeError.Inc("websocket", classifyCodecError(err))
				return gnet.None
			}

			metricCodecDecodeError.Inc("websocket", classifyCodecError(err))
			logx.Errorf("conn(%s) frame is error: %v", c, err)
			return gnet.Close
		} else if frame == nil {
			logx.Debugf("conn(%s) frame is nil", c)
			return gnet.None
		}

		//msg2, ok := frame.(*mtproto.MTPRawMessage)
		//if !ok {
		//	logx.Errorf("conn(%s) recv error: msg2 not codec.MTPRawMessage type", c)
		//	action = gnet.Close
		//	return
		//}
		//
		//logx.Infof("conn(%s) recv frame: %s", c, msg2)

		action = s.onMTPRawMessage(ctx, c, int64(binary.LittleEndian.Uint64(frame)), needAck, frame)
		if action == gnet.Close {
			return
		}

	}
}

func bufferWebsocketMessages(conn *ws.WsConn, messages []wsutil.Message) {
	for _, message := range messages {
		_, _ = conn.InboundBuffer.Write(message.Payload)
	}
}
