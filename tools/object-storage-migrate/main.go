// Copy Teamgram's S3 objects between compatible providers without deleting the source.
// The source must be quiesced before a copy run so a final object set can be verified.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/teamgram/teamgram-server/tools/s3support"
)

const defaultBuckets = "documents,encryptedfiles,photos,videos"

type endpointConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Region    string
}

type migrationConfig struct {
	Source             endpointConfig
	Target             endpointConfig
	Buckets            []string
	Mode               string
	ManifestPath       string
	ConfirmQuiesced    bool
	RequireEmptyTarget bool
}

type manifestRecord struct {
	At         string `json:"at"`
	Bucket     string `json:"bucket"`
	Key        string `json:"key"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	SourceETag string `json:"source_etag,omitempty"`
	TargetETag string `json:"target_etag,omitempty"`
	Status     string `json:"status"`
}

type manifestWriter struct {
	file *os.File
	enc  *json.Encoder
}

type digestReader struct {
	reader io.Reader
	hash   hash.Hash
	bytes  int64
}

type objectInfo struct {
	Size               int64
	ETag               string
	ContentType        string
	ContentEncoding    string
	ContentDisposition string
	ContentLanguage    string
	CacheControl       string
	Metadata           map[string]string
}

func (r *digestReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		_, _ = r.hash.Write(p[:n])
		r.bytes += int64(n)
	}
	return n, err
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	mode := flag.String("mode", envOr("MIGRATION_MODE", "verify"), "verify or copy")
	manifestPath := flag.String("manifest", os.Getenv("MIGRATION_MANIFEST"), "append-only JSONL manifest path")
	flag.Parse()

	cfg, err := loadConfig(*mode, *manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.Mode == "copy" && !cfg.ConfirmQuiesced {
		log.Fatal("copy mode requires MIGRATION_CONFIRM_SOURCE_QUIESCED=YES after stopping writers")
	}

	ctx := context.Background()
	source, err := newClient(cfg.Source)
	if err != nil {
		log.Fatal(err)
	}
	target, err := newClient(cfg.Target)
	if err != nil {
		log.Fatal(err)
	}
	if sameEndpoint(cfg.Source, cfg.Target) {
		log.Fatal("source and target S3 endpoints must be different")
	}

	manifest, err := openManifest(cfg.ManifestPath)
	if err != nil {
		log.Fatal(err)
	}
	if manifest != nil {
		defer manifest.file.Close()
	}

	if cfg.RequireEmptyTarget {
		for _, bucket := range cfg.Buckets {
			if err := checkSourceBucket(ctx, source, bucket); err != nil {
				log.Fatalf("bucket=%s source preflight failed: %v", bucket, err)
			}
			if err := ensureTargetBucket(ctx, target, bucket, cfg.Mode, cfg.Target.Region, true); err != nil {
				log.Fatalf("bucket=%s target preflight failed: %v", bucket, err)
			}
		}
	}

	var failed int
	for _, bucket := range cfg.Buckets {
		if err := checkSourceBucket(ctx, source, bucket); err != nil {
			log.Printf("bucket=%s source preflight failed: %v", bucket, err)
			failed++
			continue
		}
		if err := ensureTargetBucket(ctx, target, bucket, cfg.Mode, cfg.Target.Region, cfg.RequireEmptyTarget); err != nil {
			log.Printf("bucket=%s target preflight failed: %v", bucket, err)
			failed++
			continue
		}
		if err := migrateBucket(ctx, source, target, bucket, cfg.Mode, manifest); err != nil {
			log.Printf("bucket=%s migration failed: %v", bucket, err)
			failed++
		}
	}

	if failed > 0 {
		log.Fatalf("migration completed with %d failed bucket(s); source data was not deleted", failed)
	}
	log.Printf("migration completed mode=%s buckets=%s", cfg.Mode, strings.Join(cfg.Buckets, ","))
}

func loadConfig(mode, manifestPath string) (migrationConfig, error) {
	if mode != "copy" && mode != "verify" {
		return migrationConfig{}, fmt.Errorf("MIGRATION_MODE must be verify or copy")
	}
	requireEmptyTarget, err := envBool("MIGRATION_REQUIRE_EMPTY_TARGET", false)
	if err != nil {
		return migrationConfig{}, err
	}
	if requireEmptyTarget && mode != "copy" {
		return migrationConfig{}, fmt.Errorf("MIGRATION_REQUIRE_EMPTY_TARGET=true requires MIGRATION_MODE=copy")
	}
	source, err := loadEndpoint("SOURCE_S3")
	if err != nil {
		return migrationConfig{}, err
	}
	target, err := loadEndpoint("TARGET_S3")
	if err != nil {
		return migrationConfig{}, err
	}
	bucketValue := os.Getenv("MIGRATION_BUCKETS")
	if bucketValue == "" {
		bucketValue = os.Getenv("S3_BUCKETS")
	}
	if bucketValue == "" {
		bucketValue = defaultBuckets
	}
	var buckets []string
	for _, value := range strings.Split(bucketValue, ",") {
		bucket := strings.TrimSpace(value)
		if bucket == "" {
			return migrationConfig{}, fmt.Errorf("MIGRATION_BUCKETS contains an empty bucket")
		}
		buckets = append(buckets, bucket)
	}
	return migrationConfig{
		Source:             source,
		Target:             target,
		Buckets:            buckets,
		Mode:               mode,
		ManifestPath:       manifestPath,
		ConfirmQuiesced:    os.Getenv("MIGRATION_CONFIRM_SOURCE_QUIESCED") == "YES",
		RequireEmptyTarget: requireEmptyTarget,
	}, nil
}

func loadEndpoint(prefix string) (endpointConfig, error) {
	rawEndpoint := strings.TrimSpace(os.Getenv(prefix + "_ENDPOINT"))
	if rawEndpoint == "" {
		return endpointConfig{}, fmt.Errorf("%s_ENDPOINT is required", prefix)
	}
	useSSL, err := envBool(prefix+"_USE_SSL", false)
	if err != nil {
		return endpointConfig{}, err
	}
	endpoint, explicitSSL, err := normalizeEndpoint(rawEndpoint, useSSL)
	if err != nil {
		return endpointConfig{}, fmt.Errorf("%s_ENDPOINT: %w", prefix, err)
	}
	region := os.Getenv(prefix + "_REGION")
	if region == "" {
		region = envOr("S3_REGION", "us-east-1")
	}
	return endpointConfig{
		Endpoint:  endpoint,
		AccessKey: requiredEnv(prefix + "_ACCESS_KEY"),
		SecretKey: requiredEnv(prefix + "_SECRET_KEY"),
		UseSSL:    explicitSSL,
		Region:    region,
	}, nil
}

func newClient(cfg endpointConfig) (*s3.Client, error) {
	return s3support.NewClient(s3support.Config{
		Endpoint:  cfg.Endpoint,
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		UseSSL:    cfg.UseSSL,
		Region:    cfg.Region,
	})
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

func checkSourceBucket(ctx context.Context, client *s3.Client, bucket string) error {
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return err
	}
	versioning, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
	if err != nil {
		if isNotImplemented(err) {
			log.Printf("bucket=%s source versioning API is unavailable; continuing with current-object migration", bucket)
			return nil
		}
		return fmt.Errorf("check source versioning: %w", err)
	}
	if versioning.Status == types.BucketVersioningStatusEnabled {
		return fmt.Errorf("source bucket versioning is enabled; version history and delete markers require a separate migration")
	}
	return nil
}

func ensureTargetBucket(ctx context.Context, client *s3.Client, bucket, mode, region string, requireEmpty bool) error {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		if requireEmpty {
			paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
			if paginator.HasMorePages() {
				page, listErr := paginator.NextPage(ctx)
				if listErr != nil {
					return fmt.Errorf("list target bucket: %w", listErr)
				}
				if len(page.Contents) > 0 {
					return fmt.Errorf("target bucket is not empty (first object %q)", aws.ToString(page.Contents[0].Key))
				}
			}
		}
		return nil
	}
	if !isNotFound(err) {
		return err
	}
	if mode != "copy" {
		return fmt.Errorf("target bucket does not exist")
	}
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	return err
}

func migrateBucket(ctx context.Context, source, target *s3.Client, bucket, mode string, manifest *manifestWriter) error {
	paginator := s3.NewListObjectsV2Paginator(source, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	var failed int
	var firstErr error
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if err := migrateObject(ctx, source, target, bucket, key, mode, manifest); err != nil {
				log.Printf("bucket=%s key=%q failed: %v", bucket, key, err)
				if firstErr == nil {
					firstErr = err
				}
				failed++
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d object(s) failed; first error: %w", failed, firstErr)
	}
	return nil
}

func migrateObject(ctx context.Context, source, target *s3.Client, bucket, key, mode string, manifest *manifestWriter) error {
	sourceInfo, err := headObject(ctx, source, bucket, key)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	targetInfo, targetErr := headObject(ctx, target, bucket, key)
	if targetErr == nil {
		sourceDigest, err := hashObject(ctx, source, bucket, key, sourceInfo.Size)
		if err != nil {
			return fmt.Errorf("hash source: %w", err)
		}
		targetDigest, err := hashObject(ctx, target, bucket, key, targetInfo.Size)
		if err != nil {
			return fmt.Errorf("hash target: %w", err)
		}
		if sourceInfo.Size != targetInfo.Size || sourceDigest != targetDigest || !metadataEqual(sourceInfo, targetInfo) {
			return fmt.Errorf("target object exists but content or metadata differs; refusing to overwrite")
		}
		return writeManifest(manifest, manifestRecord{At: time.Now().UTC().Format(time.RFC3339Nano), Bucket: bucket, Key: key, Size: sourceInfo.Size, SHA256: sourceDigest, SourceETag: sourceInfo.ETag, TargetETag: targetInfo.ETag, Status: "verified"})
	}
	if !isNotFound(targetErr) {
		return fmt.Errorf("stat target: %w", targetErr)
	}
	if mode == "verify" {
		return fmt.Errorf("target object is missing")
	}

	object, err := source.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("get source: %w", err)
	}
	defer object.Body.Close()
	digest := sha256.New()
	reader := &digestReader{reader: object.Body, hash: digest}
	uploader := manager.NewUploader(target, func(u *manager.Uploader) {
		u.PartSize = 16 << 20
		u.Concurrency = 4
		u.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	if _, err = uploader.Upload(ctx, putInput(bucket, key, reader, sourceInfo)); err != nil {
		return fmt.Errorf("put target: %w", err)
	}
	if reader.bytes != sourceInfo.Size {
		return fmt.Errorf("source read size %d differs from expected %d", reader.bytes, sourceInfo.Size)
	}
	sourceDigest := hex.EncodeToString(digest.Sum(nil))
	targetInfo, err = headObject(ctx, target, bucket, key)
	if err != nil {
		return fmt.Errorf("stat copied target: %w", err)
	}
	targetDigest, err := hashObject(ctx, target, bucket, key, targetInfo.Size)
	if err != nil {
		return fmt.Errorf("hash copied target: %w", err)
	}
	if sourceInfo.Size != targetInfo.Size || sourceDigest != targetDigest || !metadataEqual(sourceInfo, targetInfo) {
		return fmt.Errorf("copied object failed content or metadata verification")
	}
	return writeManifest(manifest, manifestRecord{At: time.Now().UTC().Format(time.RFC3339Nano), Bucket: bucket, Key: key, Size: sourceInfo.Size, SHA256: sourceDigest, SourceETag: sourceInfo.ETag, TargetETag: targetInfo.ETag, Status: "copied"})
}

func headObject(ctx context.Context, client *s3.Client, bucket, key string) (objectInfo, error) {
	output, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return objectInfo{}, err
	}
	return objectInfo{
		Size:               aws.ToInt64(output.ContentLength),
		ETag:               strings.Trim(aws.ToString(output.ETag), "\""),
		ContentType:        defaultContentType(aws.ToString(output.ContentType)),
		ContentEncoding:    aws.ToString(output.ContentEncoding),
		ContentDisposition: aws.ToString(output.ContentDisposition),
		ContentLanguage:    aws.ToString(output.ContentLanguage),
		CacheControl:       aws.ToString(output.CacheControl),
		Metadata:           copyStringMap(output.Metadata),
	}, nil
}

func hashObject(ctx context.Context, client *s3.Client, bucket, key string, expectedSize int64) (string, error) {
	object, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return "", err
	}
	defer object.Body.Close()
	digest := sha256.New()
	n, err := io.Copy(digest, object.Body)
	if err != nil {
		return "", err
	}
	if n != expectedSize {
		return "", fmt.Errorf("read size %d differs from expected %d", n, expectedSize)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func putInput(bucket, key string, body io.Reader, info objectInfo) *s3.PutObjectInput {
	return &s3.PutObjectInput{
		Bucket:             aws.String(bucket),
		Key:                aws.String(key),
		Body:               body,
		ContentLength:      aws.Int64(info.Size),
		ContentType:        aws.String(defaultContentType(info.ContentType)),
		ContentEncoding:    optionalString(info.ContentEncoding),
		ContentDisposition: optionalString(info.ContentDisposition),
		ContentLanguage:    optionalString(info.ContentLanguage),
		CacheControl:       optionalString(info.CacheControl),
		Metadata:           copyStringMap(info.Metadata),
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}

func metadataEqual(source, target objectInfo) bool {
	if defaultContentType(source.ContentType) != defaultContentType(target.ContentType) {
		return false
	}
	if source.ContentEncoding != target.ContentEncoding || source.ContentDisposition != target.ContentDisposition || source.ContentLanguage != target.ContentLanguage || source.CacheControl != target.CacheControl {
		return false
	}
	sourceMetadata, sourceOK := canonicalUserMetadata(source.Metadata)
	targetMetadata, targetOK := canonicalUserMetadata(target.Metadata)
	if !sourceOK || !targetOK || len(sourceMetadata) != len(targetMetadata) {
		return false
	}
	for key, value := range sourceMetadata {
		if targetMetadata[key] != value {
			return false
		}
	}
	return true
}

func canonicalUserMetadata(source map[string]string) (map[string]string, bool) {
	if len(source) == 0 {
		return nil, true
	}
	target := make(map[string]string, len(source))
	for key, value := range source {
		canonicalKey := strings.ToLower(strings.TrimSpace(key))
		if previous, exists := target[canonicalKey]; exists && previous != value {
			return nil, false
		}
		target[canonicalKey] = value
	}
	return target, true
}

func copyStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string]string, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func defaultContentType(value string) string {
	if value == "" {
		return "application/octet-stream"
	}
	return value
}

func openManifest(path string) (*manifestWriter, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &manifestWriter{file: file, enc: json.NewEncoder(file)}, nil
}

func writeManifest(manifest *manifestWriter, record manifestRecord) error {
	if manifest == nil {
		return nil
	}
	if err := manifest.enc.Encode(record); err != nil {
		return err
	}
	if err := manifest.file.Sync(); err != nil {
		return fmt.Errorf("sync migration manifest: %w", err)
	}
	return nil
}

func sameEndpoint(source, target endpointConfig) bool {
	return source.Endpoint == target.Endpoint && source.UseSSL == target.UseSSL
}

func isNotFound(err error) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 404 {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NoSuchBucket" || apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NoSuchObject" || apiErr.ErrorCode() == "NotFound"
	}
	return false
}

func isNotImplemented(err error) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 501 {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NotImplemented" || apiErr.ErrorCode() == "NotImplementedException"
	}
	return false
}

func envBool(name string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
