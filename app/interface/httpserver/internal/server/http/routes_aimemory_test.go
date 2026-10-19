package http

import (
	"bytes"
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/core"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/dao"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
)

type stubAIMemoryTokenExchanger struct{}

func (stubAIMemoryTokenExchanger) Exchange(context.Context, int64) (string, int64, error) {
	return "token", 2_000_000_000, nil
}

// Embedding the interface supplies the full generated authsession method set;
// malformed proofs fail before any method can be called.
type stubAuthsessionClient struct {
	authsession_client.AuthsessionClient
}

func configuredAIMemoryTokenContext() *svc.ServiceContext {
	return &svc.ServiceContext{
		Dao:                    &dao.Dao{},
		AuthsessionClient:      stubAuthsessionClient{},
		AIMemoryTokenExchanger: stubAIMemoryTokenExchanger{},
	}
}

func TestAIMemoryTokenHandlerFailsClosedWhenBridgeIsNotConfigured(t *testing.T) {
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/internal/aimemory/token", nil)
	response := httptest.NewRecorder()

	aimemoryTokenHandler(&svc.ServiceContext{Dao: &dao.Dao{}})(response, request)

	if response.Code != stdhttp.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusNotFound)
	}
}

func TestAIMemoryTokenProofRejectsMalformedPayload(t *testing.T) {
	message := mtproto.NewMTPRawMessage(123, 0, core.TRANSPORT_HTTP)
	if err := message.Decode(make([]byte, 8)); err != nil {
		t.Fatalf("decode proof fixture: %v", err)
	}

	serviceContext := &svc.ServiceContext{Dao: &dao.Dao{}}
	_, _, err := core.New2(context.Background(), serviceContext, nil).IssueAIMemoryToken(message)
	if !errors.Is(err, core.ErrAIMemoryTokenProof) {
		t.Fatalf("error = %v, want ErrAIMemoryTokenProof", err)
	}
}

func TestAIMemoryTokenHandlerRejectsOversizedProof(t *testing.T) {
	request := httptest.NewRequest(
		stdhttp.MethodPost,
		"/v1/internal/aimemory/token",
		bytes.NewReader(make([]byte, (1<<20)+1)),
	)
	response := httptest.NewRecorder()

	aimemoryTokenHandler(configuredAIMemoryTokenContext())(response, request)

	if response.Code != stdhttp.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusRequestEntityTooLarge)
	}
}
