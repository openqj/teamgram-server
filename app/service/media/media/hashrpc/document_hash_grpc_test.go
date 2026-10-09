package hashrpc

import (
	"context"
	"net"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type documentHashTestServer struct {
	UnimplementedDocumentHashLookupServer
	request *DocumentHashRequest
}

func (s *documentHashTestServer) GetDocumentByHash(_ context.Context, in *DocumentHashRequest) (*mtproto.Document, error) {
	s.request = in
	return &mtproto.Document{Id: 51, AccessHash: 102}, nil
}

func TestDocumentHashLookupGRPC(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	hashServer := &documentHashTestServer{}
	RegisterDocumentHashLookupServer(server, hashServer)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	connection, err := grpc.NewClient("passthrough:///media-hash", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	request := &DocumentHashRequest{Sha256: make([]byte, 32), Size: 900, MimeType: "application/octet-stream"}
	document, err := NewDocumentHashLookupClient(connection).GetDocumentByHash(context.Background(), request)
	if err != nil {
		t.Fatalf("GetDocumentByHash() error = %v", err)
	}
	if hashServer.request == nil || hashServer.request.GetSize() != request.GetSize() || hashServer.request.GetMimeType() != request.GetMimeType() || len(hashServer.request.GetSha256()) != 32 {
		t.Fatalf("server request = %v, want %v", hashServer.request, request)
	}
	if document.GetId() != 51 || document.GetAccessHash() != 102 {
		t.Fatalf("GetDocumentByHash() = %v", document)
	}
}
