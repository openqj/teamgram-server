package dao

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

type hashDocumentDFS struct {
	dfs_client.DfsClient
	content   []byte
	chunkSize int
	requests  []*dfs.TLDfsDownloadFile
	err       error
	returnNil bool
}

func (c *hashDocumentDFS) DfsDownloadFile(_ context.Context, in *dfs.TLDfsDownloadFile) (*mtproto.Upload_File, error) {
	c.requests = append(c.requests, in)
	if c.err != nil {
		return nil, c.err
	}
	if c.returnNil {
		return nil, nil
	}
	start := int(in.GetOffset())
	if start >= len(c.content) {
		return &mtproto.Upload_File{Bytes: []byte{}}, nil
	}
	end := start + c.chunkSize
	if end > len(c.content) {
		end = len(c.content)
	}
	return &mtproto.Upload_File{Bytes: c.content[start:end]}, nil
}

func TestHashDocumentContentStreamsExactDocumentBytes(t *testing.T) {
	content := []byte("document bytes split into short chunks")
	client := &hashDocumentDFS{content: content, chunkSize: 5}
	document := &mtproto.Document{Id: 42, AccessHash: 84}

	got, err := hashDocumentContent(context.Background(), client, document, int64(len(content)))
	if err != nil {
		t.Fatalf("hashDocumentContent() error = %v", err)
	}
	want := sha256.Sum256(content)
	if !reflect.DeepEqual(got, want[:]) {
		t.Fatalf("hashDocumentContent() = %x, want %x", got, want)
	}
	var offsets []int64
	for _, request := range client.requests {
		location := request.GetLocation().To_InputDocumentFileLocation()
		if location.GetId() != document.Id || location.GetAccessHash() != document.AccessHash {
			t.Fatalf("DFS location = %v, want document id/access hash %d/%d", request.GetLocation(), document.Id, document.AccessHash)
		}
		offsets = append(offsets, request.GetOffset())
	}
	if !reflect.DeepEqual(offsets, []int64{0, 5, 10, 15, 20, 25, 30, 35}) {
		t.Fatalf("DFS offsets = %v", offsets)
	}
}

func TestHashDocumentContentRejectsIncompleteOrInvalidContent(t *testing.T) {
	serviceErr := errors.New("DFS unavailable")
	for _, tc := range []struct {
		name      string
		client    dfs_client.DfsClient
		document  *mtproto.Document
		size      int64
		wantError error
	}{
		{name: "nil client", document: &mtproto.Document{Id: 1}, size: 1, wantError: mtproto.ErrMediaInvalid},
		{name: "nil document", client: &hashDocumentDFS{}, size: 1, wantError: mtproto.ErrMediaInvalid},
		{name: "negative size", client: &hashDocumentDFS{}, document: &mtproto.Document{Id: 1}, size: -1, wantError: mtproto.ErrMediaInvalid},
		{name: "empty data", client: &hashDocumentDFS{content: []byte("x"), chunkSize: 0}, document: &mtproto.Document{Id: 1}, size: 1, wantError: mtproto.ErrMediaInvalid},
		{name: "nil response", client: &hashDocumentDFS{returnNil: true}, document: &mtproto.Document{Id: 1}, size: 1, wantError: mtproto.ErrMediaInvalid},
		{name: "DFS error", client: &hashDocumentDFS{err: serviceErr}, document: &mtproto.Document{Id: 1}, size: 1, wantError: serviceErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hashDocumentContent(context.Background(), tc.client, tc.document, tc.size)
			if got != nil || !errors.Is(err, tc.wantError) {
				t.Fatalf("hashDocumentContent() = (%x, %v), want error %v", got, err, tc.wantError)
			}
		})
	}
}

func TestHashDocumentContentUsesEmptyFileHash(t *testing.T) {
	client := &hashDocumentDFS{}
	got, err := hashDocumentContent(context.Background(), client, &mtproto.Document{Id: 42}, 0)
	if err != nil {
		t.Fatalf("hashDocumentContent() error = %v", err)
	}
	want := sha256.Sum256(nil)
	if !reflect.DeepEqual(got, want[:]) || len(client.requests) != 0 {
		t.Fatalf("hashDocumentContent() = %x after %d DFS calls, want SHA-256 of empty content", got, len(client.requests))
	}
}
