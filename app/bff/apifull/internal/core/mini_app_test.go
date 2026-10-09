// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestMiniAppRequestWebView(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if got, err := c.MessagesRequestWebView(&mtproto.TLMessagesRequestWebView{}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("request webview: got=%v err=%v, want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := c.MessagesGetBotApp(&mtproto.TLMessagesGetBotApp{}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("get bot app: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
}

func TestMiniAppURL(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 14}}
	if got, err := c.MessagesSendWebViewData(&mtproto.TLMessagesSendWebViewData{}); got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("send webview data: got=%v err=%v, want METHOD_NOT_IMPL", got, err)
	}
	if got, err := c.MessagesProlongWebView(&mtproto.TLMessagesProlongWebView{}); got != nil || !errors.Is(err, mtproto.ErrQueryIdEmpty) {
		t.Fatalf("prolong webview: got=%v err=%v, want QUERY_ID_EMPTY", got, err)
	}
}
