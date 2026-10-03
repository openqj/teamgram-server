package core

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

type webFileRoundTripper func(*http.Request) (*http.Response, error)

func (f webFileRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestFetchWebFileUsesRangeAndMetadata(t *testing.T) {
	var gotRange string
	client := &http.Client{Transport: webFileRoundTripper(func(r *http.Request) (*http.Response, error) {
		gotRange = r.Header.Get("Range")
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Range": []string{"bytes 4-7/12"},
				"Content-Type":  []string{"text/plain; charset=utf-8"},
				"Last-Modified": []string{"Wed, 21 Oct 2015 07:28:00 GMT"},
			},
			Body: io.NopCloser(strings.NewReader("data")),
		}, nil
	})}
	u, _ := url.Parse("https://example.com/file.txt")
	got, err := fetchWebFile(context.Background(), u, 4, 4, client)
	if err != nil {
		t.Fatalf("fetchWebFile() error = %v", err)
	}
	if gotRange != "bytes=4-7" {
		t.Fatalf("Range = %q, want bytes=4-7", gotRange)
	}
	if got.GetSize2() != 12 || got.GetMimeType() != "text/plain" || string(got.GetBytes()) != "data" {
		t.Fatalf("web file = %+v", got)
	}
	if got.GetMtime() != int32(time.Date(2015, 10, 21, 7, 28, 0, 0, time.UTC).Unix()) {
		t.Fatalf("mtime = %d", got.GetMtime())
	}
}

func TestFetchWebFileRejectsRangeIgnoringServer(t *testing.T) {
	client := &http.Client{Transport: webFileRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("body"))}, nil
	})}
	u, _ := url.Parse("https://example.com/file")
	if got, err := fetchWebFile(context.Background(), u, 1, 2, client); got != nil || err != ErrWebfileNotAvailable {
		t.Fatalf("fetchWebFile() = (%v, %v), want (nil, WEBFILE_NOT_AVAILABLE)", got, err)
	}
}

func TestFetchWebFileRejectsPrivateHost(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1/file")
	if got, err := fetchWebFile(context.Background(), u, 0, 1, http.DefaultClient); got != nil || err != mtproto.ErrLocationInvalid {
		t.Fatalf("fetchWebFile() = (%v, %v), want (nil, LOCATION_INVALID)", got, err)
	}
}
