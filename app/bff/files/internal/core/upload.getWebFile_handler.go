// Copyright 2022 Teamgram Authors
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
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/grpc/status"
)

var (
	ErrWebfileNotAvailable = status.Error(mtproto.ErrBadRequest, "WEBFILE_NOT_AVAILABLE")
)

const (
	maxWebFileChunk = 10 << 20
)

// webFileHTTPClient is deliberately configured without a proxy. A proxy can
// turn an otherwise safe public URL into an internal fetch target, so the
// dialer below validates the address that is actually connected to as well as
// the URL hostname.
var webFileHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:       nil,
		DialContext: safeWebFileDialContext,
	},
}

// UploadGetWebFile
// upload.getWebFile#24e6818d location:InputWebFileLocation offset:int limit:int = upload.WebFile;
func (c *FilesCore) UploadGetWebFile(in *mtproto.TLUploadGetWebFile) (*mtproto.Upload_WebFile, error) {
	if in == nil {
		c.Logger.Errorf("upload.getWebFile - empty request")
		return nil, mtproto.ErrInputRequestInvalid
	}
	location := in.GetLocation()
	if location == nil {
		c.Logger.Errorf("upload.getWebFile - empty location")
		return nil, mtproto.ErrLocationInvalid
	}
	switch location.GetPredicateName() {
	case mtproto.Predicate_inputWebFileAudioAlbumThumbLocation:
		c.Logger.Errorf("upload.getWebFile - error: %v", ErrWebfileNotAvailable)
		return nil, ErrWebfileNotAvailable
	case mtproto.Predicate_inputWebFileLocation:
	default:
		c.Logger.Errorf("upload.getWebFile - location: %s", location.GetPredicateName())
		return nil, mtproto.ErrLocationInvalid
	}

	raw := location.GetUrl()
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || webFileHostBlocked(u.Hostname()) {
		c.Logger.Errorf("upload.getWebFile - rejected url")
		return nil, mtproto.ErrLocationInvalid
	}
	if in.GetOffset() < 0 || in.GetLimit() <= 0 || in.GetLimit() > maxWebFileChunk {
		c.Logger.Errorf("upload.getWebFile - invalid range offset=%d limit=%d", in.GetOffset(), in.GetLimit())
		return nil, mtproto.ErrOffsetInvalid
	}

	webFile, err := fetchWebFile(c.ctx, u, in.GetOffset(), in.GetLimit(), webFileHTTPClient)
	if err != nil {
		c.Logger.Errorf("upload.getWebFile - fetch %q: %v", u.Redacted(), err)
		return nil, err
	}
	return webFile, nil
}

func safeWebFileDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid web-file address: %w", err)
	}
	if webFileHostBlocked(host) {
		return nil, fmt.Errorf("web-file address is private")
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve web-file host: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("web-file host has no addresses")
	}
	dialer := &net.Dialer{}
	for _, ip := range ips {
		if webFileHostBlocked(ip.String()) {
			return nil, fmt.Errorf("web-file host resolves to private address")
		}
	}
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("connect web-file host")
}

func fetchWebFile(ctx context.Context, u *url.URL, offset, limit int32, client *http.Client) (*mtproto.Upload_WebFile, error) {
	if u == nil || client == nil || offset < 0 || limit <= 0 || limit > maxWebFileChunk {
		return nil, mtproto.ErrLocationInvalid
	}
	if u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || webFileHostBlocked(u.Hostname()) {
		return nil, mtproto.ErrLocationInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, mtproto.ErrLocationInvalid
	}
	lastByte := int64(offset) + int64(limit) - 1
	req.Header.Set("Range", "bytes="+strconv.FormatInt(int64(offset), 10)+"-"+strconv.FormatInt(lastByte, 10))
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrWebfileNotAvailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ErrWebfileNotAvailable
	}
	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		// A server that ignores Range would make us return bytes from the wrong
		// offset. Refuse the response instead of silently corrupting media.
		return nil, ErrWebfileNotAvailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return nil, ErrWebfileNotAvailable
	}
	if len(data) > int(limit) {
		data = data[:limit]
	}
	mimeType := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	if mimeType == "" {
		mimeType = webFileMime(u.Path)
	}
	size := int64(0)
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		if i := strings.LastIndexByte(contentRange, '/'); i >= 0 {
			size, _ = strconv.ParseInt(contentRange[i+1:], 10, 64)
		}
	} else if length, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); err == nil && length >= 0 {
		size = length + int64(offset)
	}
	if size > int64(^uint32(0)>>1) {
		return nil, ErrWebfileNotAvailable
	}
	mtime := int32(0)
	if parsed, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		mtime = int32(parsed.Unix())
	}
	return mtproto.MakeTLUploadWebFile(&mtproto.Upload_WebFile{
		Size2:    int32(size),
		MimeType: mimeType,
		FileType: mtproto.MakeTLStorageFilePartial(nil).To_Storage_FileType(),
		Mtime:    mtime,
		Bytes:    data,
	}).To_Upload_WebFile(), nil
}
