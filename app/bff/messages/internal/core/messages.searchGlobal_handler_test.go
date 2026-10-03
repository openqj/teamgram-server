package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesSearchGlobalRejectsMissingAuthenticationBeforeRequestAccess(t *testing.T) {
	core := &MessagesCore{}

	got, err := core.MessagesSearchGlobal(&mtproto.TLMessagesSearchGlobal{Q: "query"})
	if got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("MessagesSearchGlobal() = (%v, %v), want (nil, AUTH_KEY_UNREGISTERED)", got, err)
	}
}

func TestMessagesSearchGlobalRejectsEmptyQueryBeforeProviderAccess(t *testing.T) {
	core := &MessagesCore{Logger: logx.WithContext(context.Background()), MD: &metadata.RpcMetadata{UserId: 41}}

	got, err := core.MessagesSearchGlobal(&mtproto.TLMessagesSearchGlobal{})
	if got != nil || err != mtproto.ErrSearchQueryEmpty {
		t.Fatalf("MessagesSearchGlobal() = (%v, %v), want (nil, SEARCH_QUERY_EMPTY)", got, err)
	}
}
