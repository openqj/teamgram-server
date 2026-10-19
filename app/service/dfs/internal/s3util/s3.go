// Copyright (c) 2021-present, Teamgram Studio.
//
// Licensed under the Apache License, Version 2.0.

package s3util

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	defaultMaxIdleConns          = 256
	defaultMaxIdleConnsPerHost   = 64
	defaultIdleConnTimeout       = 60 * time.Second
	defaultResponseHeaderTimeout = 60 * time.Second
	defaultUploadPartSize        = 16 << 20
	defaultUploadConcurrency     = 4
	defaultDialTimeout           = 30 * time.Second
	defaultDialKeepAlive         = 30 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultExpectContinueTimeout = 1 * time.Second
)

// BucketConfig is the bucket mapping used by the S3-compatible client.
type BucketConfig struct {
	Documents      string `json:",default=documents"`
	Photos         string `json:",default=photos"`
	Videos         string `json:",default=videos"`
	EncryptedFiles string `json:",default=encryptedfiles"`
}

// TransportConfig controls connection reuse and multipart upload pressure.
// Zero values use the production defaults above.
type TransportConfig struct {
	MaxIdleConns              int `json:",default=256"`
	MaxIdleConnsPerHost       int `json:",default=64"`
	MaxConnsPerHost           int `json:",default=0"`
	IdleConnTimeoutSeconds    int `json:",default=60"`
	ResponseHeaderTimeoutSecs int `json:",default=60"`
	UploadPartSizeMiB         int `json:",default=16"`
	UploadConcurrency         int `json:",default=4"`
}

type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	Region          string
	Bucket          BucketConfig
	Transport       TransportConfig
}

// UploadInfo is the small result surface required by the DFS handlers.
type UploadInfo struct {
	Size     int64
	ETag     string
	Key      string
	Location string
}

type S3Util struct {
	c        *S3Config
	client   *s3.Client
	uploader *manager.Uploader
}

func MustNewS3Client(c *S3Config) *S3Util {
	client, err := newS3Client(c)
	if err != nil {
		logx.Must(err)
	}

	partSize := c.Transport.UploadPartSizeMiB
	if partSize <= 0 {
		partSize = defaultUploadPartSize >> 20
	}
	concurrency := c.Transport.UploadConcurrency
	if concurrency <= 0 {
		concurrency = defaultUploadConcurrency
	}
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = int64(partSize) << 20
		u.Concurrency = concurrency
		// SeaweedFS and RustFS implement standard S3 checksums, but older
		// deployments may reject optional SDK-generated checksum headers.
		u.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})

	return &S3Util{c: c, client: client, uploader: uploader}
}

func newS3Client(c *S3Config) (*s3.Client, error) {
	if c == nil {
		return nil, fmt.Errorf("S3 config is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return nil, fmt.Errorf("S3 endpoint is required")
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return nil, fmt.Errorf("S3 access key and secret key are required")
	}

	httpClient, err := newHTTPClient(c)
	if err != nil {
		return nil, err
	}
	endpoint := "http://" + strings.TrimSpace(c.Endpoint)
	if c.UseSSL {
		endpoint = "https://" + strings.TrimSpace(c.Endpoint)
	}

	awsConfig := aws.Config{
		Region:                     c.Region,
		Credentials:                credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, ""),
		HTTPClient:                 httpClient,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	return s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	}), nil
}

func newHTTPClient(c *S3Config) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.DialContext = (&net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultDialKeepAlive,
	}).DialContext
	transport.MaxIdleConns = positiveOr(c.Transport.MaxIdleConns, defaultMaxIdleConns)
	transport.MaxIdleConnsPerHost = positiveOr(c.Transport.MaxIdleConnsPerHost, defaultMaxIdleConnsPerHost)
	transport.MaxConnsPerHost = nonNegativeOr(c.Transport.MaxConnsPerHost, 0)
	transport.IdleConnTimeout = time.Duration(positiveOr(c.Transport.IdleConnTimeoutSeconds, int(defaultIdleConnTimeout/time.Second))) * time.Second
	transport.ResponseHeaderTimeout = time.Duration(positiveOr(c.Transport.ResponseHeaderTimeoutSecs, int(defaultResponseHeaderTimeout/time.Second))) * time.Second
	transport.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	transport.ExpectContinueTimeout = defaultExpectContinueTimeout
	transport.DisableCompression = true
	if c.UseSSL {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		if certFile := os.Getenv("SSL_CERT_FILE"); certFile != "" {
			certPEM, err := os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("read SSL_CERT_FILE: %w", err)
			}
			transport.TLSClientConfig.RootCAs = x509SystemCertPool()
			transport.TLSClientConfig.RootCAs.AppendCertsFromPEM(certPEM)
		}
	}
	return &http.Client{Transport: transport}, nil
}

func x509SystemCertPool() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		return x509.NewCertPool()
	}
	return pool
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func nonNegativeOr(value, fallback int) int {
	if value >= 0 {
		return value
	}
	return fallback
}

func (m *S3Util) GetFileObject(ctx context.Context, bucket, path string) (io.ReadCloser, error) {
	object, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFileObject error: %v", err)
		return nil, err
	}
	return object.Body, nil
}

func (m *S3Util) GetPhotoFileData(ctx context.Context, path string) ([]byte, error) {
	object, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.c.Bucket.Photos),
		Key:    aws.String(path),
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetPhotoFileData error: %v", err)
		return nil, err
	}
	defer object.Body.Close()
	var dataBuffer bytes.Buffer
	if size := aws.ToInt64(object.ContentLength); size > 0 && size <= int64(^uint(0)>>1) {
		dataBuffer.Grow(int(size))
	}
	if _, err = dataBuffer.ReadFrom(object.Body); err != nil {
		logx.WithContext(ctx).Errorf("GetPhotoFileData read error: %v", err)
		return nil, err
	}
	data := dataBuffer.Bytes()
	if len(data) == 0 {
		return nil, io.EOF
	}
	return data, nil
}

func (m *S3Util) GetFile(ctx context.Context, bucket, path string, offset int64, limit int32) ([]byte, error) {
	if offset < 0 || limit <= 0 {
		return nil, fmt.Errorf("invalid S3 range offset=%d limit=%d", offset, limit)
	}
	end := offset + int64(limit) - 1
	if end < offset {
		return nil, fmt.Errorf("S3 range overflow")
	}
	object, err := m.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(path),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", offset, end)),
	})
	if err != nil {
		logx.WithContext(ctx).Errorf("GetFile error: %v", err)
		return nil, err
	}
	defer object.Body.Close()
	readSize := int64(limit)
	if contentLength := aws.ToInt64(object.ContentLength); contentLength > 0 && contentLength < readSize {
		readSize = contentLength
	}
	data := make([]byte, int(readSize))
	n, err := io.ReadFull(object.Body, data)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		logx.WithContext(ctx).Errorf("GetFile read error: %v", err)
		return nil, err
	}
	data = data[:n]
	if len(data) == 0 {
		return nil, io.EOF
	}
	return data, nil
}

func (m *S3Util) GetPhotoFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.GetFile(ctx, m.c.Bucket.Photos, path, offset, limit)
}

func (m *S3Util) GetVideoFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.GetFile(ctx, m.c.Bucket.Videos, path, offset, limit)
}

func (m *S3Util) GetDocumentFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.GetFile(ctx, m.c.Bucket.Documents, path, offset, limit)
}

func (m *S3Util) GetEncryptedFile(ctx context.Context, path string, offset int64, limit int32) ([]byte, error) {
	return m.GetFile(ctx, m.c.Bucket.EncryptedFiles, path, offset, limit)
}

func (m *S3Util) PutPhotoFile(ctx context.Context, path string, buf []byte) (UploadInfo, error) {
	return m.put(ctx, m.c.Bucket.Photos, path, bytes.NewReader(buf), int64(len(buf)), contentType(path))
}

func (m *S3Util) PutPhotoFileV2(ctx context.Context, path string, r io.Reader) (UploadInfo, error) {
	return m.put(ctx, m.c.Bucket.Photos, path, r, -1, contentType(path))
}

func (m *S3Util) PutVideoFile(ctx context.Context, path string, buf []byte) (UploadInfo, error) {
	return m.put(ctx, m.c.Bucket.Videos, path, bytes.NewReader(buf), int64(len(buf)), contentType(path))
}

func (m *S3Util) PutVideoFileV2(ctx context.Context, path string, r io.Reader) (UploadInfo, error) {
	return m.put(ctx, m.c.Bucket.Videos, path, r, -1, contentType(path))
}

func (m *S3Util) PutDocumentFile(ctx context.Context, path string, r io.Reader) (UploadInfo, error) {
	return m.put(ctx, m.c.Bucket.Documents, path, r, -1, contentType(path))
}

func (m *S3Util) FPutDocumentFile(ctx context.Context, path, filePath string) (UploadInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return UploadInfo{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return UploadInfo{}, err
	}
	return m.put(ctx, m.c.Bucket.Documents, path, file, info.Size(), contentType(path))
}

func (m *S3Util) PutEncryptedFile(ctx context.Context, path string, r io.Reader) (UploadInfo, error) {
	return m.put(ctx, m.c.Bucket.EncryptedFiles, path, r, -1, "binary/octet-stream")
}

func (m *S3Util) put(ctx context.Context, bucket, path string, r io.Reader, size int64, contentType string) (UploadInfo, error) {
	if size < 0 {
		if detected, ok := readerSize(r); ok {
			size = detected
		}
	}

	var counted *countingReader
	source := r
	if size < 0 {
		counted = &countingReader{Reader: r}
		source = counted
	}
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(path),
		Body:        source,
		ContentType: aws.String(contentType),
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	output, err := m.uploader.Upload(ctx, input)
	if err != nil {
		logx.WithContext(ctx).Errorf("PutObject bucket=%s key=%s error: %v", bucket, path, err)
		return UploadInfo{}, err
	}
	if counted != nil {
		size = counted.n
	}
	return UploadInfo{
		Size:     size,
		ETag:     aws.ToString(output.ETag),
		Key:      aws.ToString(output.Key),
		Location: output.Location,
	}, nil
}

func readerSize(r io.Reader) (int64, bool) {
	seeker, ok := r.(io.Seeker)
	if !ok {
		return 0, false
	}
	current, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, false
	}
	end, err := seeker.Seek(0, io.SeekEnd)
	if err != nil {
		_, _ = seeker.Seek(current, io.SeekStart)
		return 0, false
	}
	if _, err = seeker.Seek(current, io.SeekStart); err != nil || end < current {
		return 0, false
	}
	return end - current, true
}

func contentType(path string) string {
	if ext := filepath.Ext(path); model.IsFileExtImage(ext) {
		return model.GetImageMimeType(ext)
	}
	return "binary/octet-stream"
}

type countingReader struct {
	io.Reader
	n int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.n += int64(n)
	return n, err
}
