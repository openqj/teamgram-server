package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type seamlessURLAuthStore struct {
	values map[string]string
}

func (s *seamlessURLAuthStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *seamlessURLAuthStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func TestMessagesAcceptUrlAuthLayer229ReturnsURL(t *testing.T) {
	store := &seamlessURLAuthStore{values: make(map[string]string)}
	oldStore := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = oldStore })

	const url = "https://example.invalid/layer229-url-auth"
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 991229}}
	result, err := core.MessagesAcceptUrlAuth(&mtproto.TLMessagesAcceptUrlAuth{
		Url: wrapperspb.String(url),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.GetUrl_FLAGSTRING().GetValue(); got != url {
		t.Fatalf("response URL = %q, want %q", got, url)
	}

	buf := mtproto.NewEncodeBuf(128)
	if err = result.Encode(buf, 229); err != nil {
		t.Fatal(err)
	}
	decoded := mtproto.NewDecodeBuf(buf.GetBuf())
	object := decoded.Object()
	if err = decoded.GetError(); err != nil {
		t.Fatal(err)
	}
	accepted, ok := object.(*mtproto.TLUrlAuthResultAccepted)
	if !ok {
		t.Fatalf("decoded object = %T, want *mtproto.TLUrlAuthResultAccepted", object)
	}
	if got := accepted.GetUrl_FLAGSTRING().GetValue(); got != url {
		t.Fatalf("Layer 229 decoded URL = %q, want %q", got, url)
	}
}
