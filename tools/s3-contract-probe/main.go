// Command s3-contract-probe checks the S3 data-plane operations used by DFS.
// It writes only a unique temporary prefix and removes it before exiting.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	defaultRegion      = "us-east-1"
	defaultTimeout     = 90 * time.Second
	defaultPartSizeMiB = 5
)

type probeConfig struct {
	endpoint             string
	accessKey            string
	secretKey            string
	secure               bool
	region               string
	bucket               string
	prefix               string
	partSize             int64
	timeout              time.Duration
	createBucket         bool
	requireMultipartList bool
}

type probe struct {
	client *minio.Client
	core   *minio.Core
	cfg    probeConfig
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	createBucket := flag.Bool("create-bucket", envBool("S3_PROBE_CREATE_BUCKET", false), "create the bucket when it does not exist")
	requireMultipartList := flag.Bool("require-multipart-list", envBool("S3_PROBE_REQUIRE_MULTIPART_LIST", false), "require ListMultipartUploads permission for the operations check")
	flag.Parse()

	cfg, err := loadConfig(*createBucket, *requireMultipartList)
	if err != nil {
		log.Fatal(err)
	}

	client, err := minio.New(cfg.endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure:       cfg.secure,
		Region:       cfg.region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		log.Fatal(err)
	}
	core := &minio.Core{Client: client}

	log.Printf("checking S3 data plane endpoint=%s bucket=%s prefix=%s region=%s", cfg.endpoint, cfg.bucket, cfg.prefix, cfg.region)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	if err := (&probe{client: client, core: core, cfg: cfg}).run(ctx); err != nil {
		log.Fatal(err)
	}
	log.Printf("S3 contract probe passed")
}

func loadConfig(createBucket, requireMultipartList bool) (probeConfig, error) {
	rawEndpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	if rawEndpoint == "" {
		return probeConfig{}, fmt.Errorf("S3_ENDPOINT is required")
	}
	endpoint, secure, err := normalizeEndpoint(rawEndpoint, envBool("S3_USE_SSL", false))
	if err != nil {
		return probeConfig{}, fmt.Errorf("S3_ENDPOINT: %w", err)
	}
	accessKey := strings.TrimSpace(os.Getenv("S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("S3_SECRET_KEY"))
	if accessKey == "" || secretKey == "" {
		return probeConfig{}, fmt.Errorf("S3_ACCESS_KEY and S3_SECRET_KEY are required")
	}

	bucket := strings.TrimSpace(os.Getenv("S3_PROBE_BUCKET"))
	if bucket == "" {
		bucket = strings.TrimSpace(os.Getenv("S3_BUCKET"))
	}
	if bucket == "" {
		return probeConfig{}, fmt.Errorf("S3_PROBE_BUCKET or S3_BUCKET is required")
	}
	if strings.ContainsAny(bucket, "/ \t\r\n") {
		return probeConfig{}, fmt.Errorf("S3_PROBE_BUCKET must be a bucket name without path or whitespace")
	}

	region := strings.TrimSpace(os.Getenv("S3_REGION"))
	if region == "" {
		region = defaultRegion
	}
	if strings.ContainsAny(region, " \t\r\n") {
		return probeConfig{}, fmt.Errorf("S3_REGION must not contain whitespace")
	}

	prefix := strings.TrimSpace(os.Getenv("S3_PROBE_PREFIX"))
	if prefix == "" {
		prefix = fmt.Sprintf("contract-probe/%d-%d", time.Now().UnixNano(), os.Getpid())
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" || strings.ContainsAny(prefix, "\r\n") {
		return probeConfig{}, fmt.Errorf("S3_PROBE_PREFIX must not be empty or contain newlines")
	}

	partSizeMiB, err := positiveIntEnv("S3_PROBE_PART_SIZE_MIB", defaultPartSizeMiB)
	if err != nil {
		return probeConfig{}, err
	}
	if partSizeMiB < defaultPartSizeMiB {
		return probeConfig{}, fmt.Errorf("S3_PROBE_PART_SIZE_MIB must be at least %d", defaultPartSizeMiB)
	}
	timeoutSeconds, err := positiveIntEnv("S3_PROBE_TIMEOUT_SECONDS", int(defaultTimeout/time.Second))
	if err != nil {
		return probeConfig{}, err
	}

	return probeConfig{
		endpoint:             endpoint,
		accessKey:            accessKey,
		secretKey:            secretKey,
		secure:               secure,
		region:               region,
		bucket:               bucket,
		prefix:               prefix,
		partSize:             int64(partSizeMiB) * 1024 * 1024,
		timeout:              time.Duration(timeoutSeconds) * time.Second,
		createBucket:         createBucket,
		requireMultipartList: requireMultipartList,
	}, nil
}

func (p *probe) run(ctx context.Context) (runErr error) {
	if err := p.ensureBucket(ctx); err != nil {
		return fmt.Errorf("head bucket: %w", err)
	}
	if p.cfg.requireMultipartList {
		if err := p.listMultipartUploads(ctx); err != nil {
			return fmt.Errorf("list multipart uploads: %w", err)
		}
	}

	singleKey := p.cfg.prefix + "/single-put.txt"
	multipartKey := p.cfg.prefix + "/multipart.bin"
	keys := []string{singleKey, multipartKey}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := p.cleanup(cleanupCtx, keys); err != nil {
			if runErr == nil {
				runErr = fmt.Errorf("cleanup: %w", err)
			} else {
				runErr = fmt.Errorf("%v; cleanup: %w", runErr, err)
			}
		}
	}()

	singlePayload := []byte("teamgram-s3-contract-probe\n")
	if err := p.singlePutRoundTrip(ctx, singleKey, singlePayload); err != nil {
		return err
	}
	if err := p.listContains(ctx, singleKey); err != nil {
		return fmt.Errorf("list objects: %w", err)
	}
	if err := p.multipartRoundTrip(ctx, multipartKey); err != nil {
		return err
	}
	return nil
}

func (p *probe) ensureBucket(ctx context.Context) error {
	exists, err := p.client.BucketExists(ctx, p.cfg.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if !p.cfg.createBucket {
		return fmt.Errorf("bucket does not exist; pass -create-bucket or S3_PROBE_CREATE_BUCKET=true for a new test bucket")
	}
	if err := p.client.MakeBucket(ctx, p.cfg.bucket, minio.MakeBucketOptions{Region: p.cfg.region}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	log.Printf("created bucket=%s; probe will leave the bucket in place", p.cfg.bucket)
	return nil
}

func (p *probe) singlePutRoundTrip(ctx context.Context, key string, payload []byte) error {
	const contentType = "application/x-teamgram-s3-contract"
	if _, err := p.client.PutObject(ctx, p.cfg.bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	if err := p.verifyObject(ctx, key, payload, contentType); err != nil {
		return fmt.Errorf("single PUT round trip %q: %w", key, err)
	}
	return nil
}

func (p *probe) multipartRoundTrip(ctx context.Context, key string) error {
	first := probePayload(p.cfg.partSize, "first")
	second := probePayload(1024*1024, "second")
	want := append(append(make([]byte, 0, len(first)+len(second)), first...), second...)

	uploadID, err := p.core.NewMultipartUpload(ctx, p.cfg.bucket, key, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return fmt.Errorf("start multipart upload %q: %w", key, err)
	}
	completed := false
	defer func() {
		if !completed {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if abortErr := p.core.AbortMultipartUpload(cleanupCtx, p.cfg.bucket, key, uploadID); abortErr != nil {
				log.Printf("warning: abort multipart key=%q: %v", key, abortErr)
			}
		}
	}()

	parts := make([]minio.CompletePart, 0, 2)
	for number, payload := range [][]byte{first, second} {
		part, err := p.core.PutObjectPart(ctx, p.cfg.bucket, key, uploadID, number+1, bytes.NewReader(payload), int64(len(payload)), "", "", nil)
		if err != nil {
			return fmt.Errorf("upload multipart part %d for %q: %w", number+1, key, err)
		}
		parts = append(parts, minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if _, err := p.core.CompleteMultipartUpload(ctx, p.cfg.bucket, key, uploadID, parts, minio.PutObjectOptions{}); err != nil {
		return fmt.Errorf("complete multipart upload %q: %w", key, err)
	}
	completed = true
	if err := p.verifyObject(ctx, key, want, "application/octet-stream"); err != nil {
		return fmt.Errorf("multipart round trip %q: %w", key, err)
	}
	return nil
}

func (p *probe) verifyObject(ctx context.Context, key string, want []byte, contentType string) error {
	info, err := p.client.StatObject(ctx, p.cfg.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("head object: %w", err)
	}
	if info.Size != int64(len(want)) {
		return fmt.Errorf("size=%d, want=%d", info.Size, len(want))
	}
	if info.ContentType != contentType {
		return fmt.Errorf("content type=%q, want=%q", info.ContentType, contentType)
	}

	object, err := p.client.GetObject(ctx, p.cfg.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer object.Close()
	digest := sha256.New()
	readSize, err := io.Copy(digest, object)
	if err != nil {
		return fmt.Errorf("read object: %w", err)
	}
	if readSize != int64(len(want)) {
		return fmt.Errorf("read size=%d, want=%d", readSize, len(want))
	}
	wantDigest := sha256.Sum256(want)
	gotDigest := hex.EncodeToString(digest.Sum(nil))
	if gotDigest != hex.EncodeToString(wantDigest[:]) {
		return fmt.Errorf("sha256=%s, want=%s", gotDigest, hex.EncodeToString(wantDigest[:]))
	}
	return nil
}

func (p *probe) listContains(ctx context.Context, key string) error {
	found := false
	for object := range p.client.ListObjects(ctx, p.cfg.bucket, minio.ListObjectsOptions{Prefix: p.cfg.prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if object.Key == key {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("object %q was not returned for prefix %q", key, p.cfg.prefix)
	}
	return nil
}

func (p *probe) listMultipartUploads(ctx context.Context) error {
	result, err := p.core.ListMultipartUploads(ctx, p.cfg.bucket, p.cfg.prefix, "", "", "", 1000)
	if err != nil {
		return err
	}
	if result.IsTruncated {
		return fmt.Errorf("more than 1000 incomplete uploads exist under prefix %q", p.cfg.prefix)
	}
	if len(result.Uploads) > 0 {
		log.Printf("warning: %d incomplete upload(s) already exist under prefix=%s", len(result.Uploads), p.cfg.prefix)
	}
	return nil
}

func (p *probe) cleanup(ctx context.Context, keys []string) error {
	var firstErr error
	for _, key := range keys {
		if err := p.client.RemoveObject(ctx, p.cfg.bucket, key, minio.RemoveObjectOptions{}); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("delete object %q: %w", key, err)
		}
	}
	return firstErr
}

func probePayload(size int64, seed string) []byte {
	payload := make([]byte, size)
	pattern := []byte("teamgram-s3-contract-" + seed + "\n")
	for offset := 0; offset < len(payload); offset += len(pattern) {
		copy(payload[offset:], pattern)
	}
	return payload
}

func normalizeEndpoint(raw string, useSSL bool) (string, bool, error) {
	if strings.ContainsAny(raw, "?#") {
		return "", false, fmt.Errorf("must not contain query parameters or fragments")
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || parsed.Path != "" || parsed.User != nil {
			return "", false, fmt.Errorf("must be an HTTP(S) origin without a path or embedded credentials")
		}
		return parsed.Host, parsed.Scheme == "https", nil
	}
	if strings.Contains(raw, "/") {
		return "", false, fmt.Errorf("must be host:port or an HTTP(S) origin")
	}
	return raw, useSSL, nil
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("%s must be true or false", name)
	}
	return parsed
}

func positiveIntEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}
