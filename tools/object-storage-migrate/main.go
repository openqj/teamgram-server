// Copy Teamgram's S3 objects between compatible providers without deleting the source.
// The source must be quiesced before a copy run so a final object set can be verified.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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

	// A restore target must be checked for every bucket before the first write;
	// otherwise a later non-empty bucket could leave an earlier bucket partially
	// restored before the run aborts.
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

func newClient(cfg endpointConfig) (*minio.Client, error) {
	return minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
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

func checkSourceBucket(ctx context.Context, client *minio.Client, bucket string) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("source bucket does not exist")
	}
	versioning, err := client.GetBucketVersioning(ctx, bucket)
	if err != nil {
		if isNotImplemented(err) {
			log.Printf("bucket=%s source versioning API is unavailable; continuing with current-object migration", bucket)
			return nil
		}
		return fmt.Errorf("check source versioning: %w", err)
	}
	if versioning.Enabled() {
		return fmt.Errorf("source bucket versioning is enabled; version history and delete markers require a separate migration")
	}
	return nil
}

func ensureTargetBucket(ctx context.Context, client *minio.Client, bucket, mode, region string, requireEmpty bool) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if exists {
		if requireEmpty {
			for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
				if object.Err != nil {
					return fmt.Errorf("list target bucket: %w", object.Err)
				}
				return fmt.Errorf("target bucket is not empty (first object %q)", object.Key)
			}
		}
		return nil
	}
	if mode != "copy" {
		return fmt.Errorf("target bucket does not exist")
	}
	return client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region})
}

func migrateBucket(ctx context.Context, source, target *minio.Client, bucket, mode string, manifest *manifestWriter) error {
	var failed int
	var firstErr error
	for info := range source.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if info.Err != nil {
			if firstErr == nil {
				firstErr = info.Err
			}
			failed++
			continue
		}
		if err := migrateObject(ctx, source, target, bucket, info.Key, mode, manifest); err != nil {
			log.Printf("bucket=%s key=%q failed: %v", bucket, info.Key, err)
			if firstErr == nil {
				firstErr = err
			}
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d object(s) failed; first error: %w", failed, firstErr)
	}
	return nil
}

func migrateObject(ctx context.Context, source, target *minio.Client, bucket, key, mode string, manifest *manifestWriter) error {
	sourceInfo, err := source.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	targetInfo, targetErr := target.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
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

	object, err := source.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get source: %w", err)
	}
	defer object.Close()
	digest := sha256.New()
	reader := &digestReader{reader: object, hash: digest}
	_, err = target.PutObject(ctx, bucket, key, reader, sourceInfo.Size, putOptions(sourceInfo))
	if err != nil {
		return fmt.Errorf("put target: %w", err)
	}
	if reader.bytes != sourceInfo.Size {
		return fmt.Errorf("source read size %d differs from expected %d", reader.bytes, sourceInfo.Size)
	}
	sourceDigest := hex.EncodeToString(digest.Sum(nil))
	targetInfo, err = target.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
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

func hashObject(ctx context.Context, client *minio.Client, bucket, key string, expectedSize int64) (string, error) {
	object, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return "", err
	}
	defer object.Close()
	digest := sha256.New()
	n, err := io.Copy(digest, object)
	if err != nil {
		return "", err
	}
	if n != expectedSize {
		return "", fmt.Errorf("read size %d differs from expected %d", n, expectedSize)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func putOptions(info minio.ObjectInfo) minio.PutObjectOptions {
	return minio.PutObjectOptions{
		ContentType:        defaultContentType(info.ContentType),
		ContentEncoding:    info.Metadata.Get("Content-Encoding"),
		ContentDisposition: info.Metadata.Get("Content-Disposition"),
		ContentLanguage:    info.Metadata.Get("Content-Language"),
		CacheControl:       info.Metadata.Get("Cache-Control"),
		UserMetadata:       copyStringMap(info.UserMetadata),
	}
}

func metadataEqual(source, target minio.ObjectInfo) bool {
	if defaultContentType(source.ContentType) != defaultContentType(target.ContentType) {
		return false
	}
	for _, header := range []string{"Content-Encoding", "Content-Disposition", "Content-Language", "Cache-Control"} {
		if source.Metadata.Get(header) != target.Metadata.Get(header) {
			return false
		}
	}
	sourceMetadata, sourceOK := canonicalUserMetadata(source.UserMetadata)
	targetMetadata, targetOK := canonicalUserMetadata(target.UserMetadata)
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

func canonicalUserMetadata(source minio.StringMap) (map[string]string, bool) {
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

func copyStringMap(source minio.StringMap) map[string]string {
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
	response := minio.ToErrorResponse(err)
	return response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NotFound"
}

func isNotImplemented(err error) bool {
	response := minio.ToErrorResponse(err)
	return response.StatusCode == 501 || response.Code == "NotImplemented" || response.Code == "NotImplementedException"
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
