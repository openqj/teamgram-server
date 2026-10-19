// Command s3-contract-probe checks the S3 data-plane operations used by DFS.
// It writes only a unique temporary prefix and removes it before exiting.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/teamgram/teamgram-server/tools/s3support"
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
	client *s3.Client
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
	client, err := s3support.NewClient(s3support.Config{
		Endpoint:  cfg.endpoint,
		AccessKey: cfg.accessKey,
		SecretKey: cfg.secretKey,
		UseSSL:    cfg.secure,
		Region:    cfg.region,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("checking S3 data plane endpoint=%s bucket=%s prefix=%s region=%s", cfg.endpoint, cfg.bucket, cfg.prefix, cfg.region)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	if err := (&probe{client: client, cfg: cfg}).run(ctx); err != nil {
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
	_, err := p.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(p.cfg.bucket)})
	if err == nil {
		return nil
	}
	if !p.cfg.createBucket || !isNotFound(err) {
		return err
	}
	if _, err = p.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(p.cfg.bucket)}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	log.Printf("created bucket=%s; probe will leave the bucket in place", p.cfg.bucket)
	return nil
}

func (p *probe) singlePutRoundTrip(ctx context.Context, key string, payload []byte) error {
	const contentType = "application/x-teamgram-s3-contract"
	if _, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(p.cfg.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(payload),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(payload))),
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

	created, err := p.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(p.cfg.bucket),
		Key:         aws.String(key),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("start multipart upload %q: %w", key, err)
	}
	uploadID := aws.ToString(created.UploadId)
	completed := false
	defer func() {
		if !completed {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if abortErr := p.abortMultipart(cleanupCtx, key, uploadID); abortErr != nil {
				log.Printf("warning: abort multipart key=%q: %v", key, abortErr)
			}
		}
	}()

	parts := make([]types.CompletedPart, 0, 2)
	for number, payload := range [][]byte{first, second} {
		part, err := p.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:        aws.String(p.cfg.bucket),
			Key:           aws.String(key),
			UploadId:      aws.String(uploadID),
			PartNumber:    int32Ptr(int32(number + 1)),
			Body:          bytes.NewReader(payload),
			ContentLength: aws.Int64(int64(len(payload))),
		})
		if err != nil {
			return fmt.Errorf("upload multipart part %d for %q: %w", number+1, key, err)
		}
		parts = append(parts, types.CompletedPart{PartNumber: int32Ptr(int32(number + 1)), ETag: part.ETag})
	}
	if _, err := p.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(p.cfg.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	}); err != nil {
		return fmt.Errorf("complete multipart upload %q: %w", key, err)
	}
	completed = true
	if err := p.verifyObject(ctx, key, want, "application/octet-stream"); err != nil {
		return fmt.Errorf("multipart round trip %q: %w", key, err)
	}
	return nil
}

func (p *probe) verifyObject(ctx context.Context, key string, want []byte, contentType string) error {
	info, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(p.cfg.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("head object: %w", err)
	}
	if aws.ToInt64(info.ContentLength) != int64(len(want)) {
		return fmt.Errorf("size=%d, want=%d", aws.ToInt64(info.ContentLength), len(want))
	}
	if aws.ToString(info.ContentType) != contentType {
		return fmt.Errorf("content type=%q, want=%q", aws.ToString(info.ContentType), contentType)
	}

	object, err := p.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(p.cfg.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer object.Body.Close()
	digest := sha256.New()
	readSize, err := io.Copy(digest, object.Body)
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
	paginator := s3.NewListObjectsV2Paginator(p.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(p.cfg.bucket),
		Prefix: aws.String(p.cfg.prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			if aws.ToString(object.Key) == key {
				return nil
			}
		}
	}
	return fmt.Errorf("object %q was not returned for prefix %q", key, p.cfg.prefix)
}

func (p *probe) listMultipartUploads(ctx context.Context) error {
	result, err := p.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
		Bucket:     aws.String(p.cfg.bucket),
		Prefix:     aws.String(p.cfg.prefix),
		MaxUploads: int32Ptr(1000),
	})
	if err != nil {
		return err
	}
	if aws.ToBool(result.IsTruncated) {
		return fmt.Errorf("more than 1000 incomplete uploads exist under prefix %q", p.cfg.prefix)
	}
	if len(result.Uploads) > 0 {
		log.Printf("warning: %d incomplete upload(s) already exist under prefix=%s", len(result.Uploads), p.cfg.prefix)
	}
	return nil
}

func (p *probe) abortMultipart(ctx context.Context, key, uploadID string) error {
	_, err := p.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(p.cfg.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	return err
}

func (p *probe) cleanup(ctx context.Context, keys []string) error {
	var firstErr error
	for _, key := range keys {
		if _, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(p.cfg.bucket), Key: aws.String(key)}); err != nil && firstErr == nil {
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

func isNotFound(err error) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 404 {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NoSuchBucket" || apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey"
	}
	return false
}

func int32Ptr(value int32) *int32 { return &value }

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
