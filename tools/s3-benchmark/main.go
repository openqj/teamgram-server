// Command s3-benchmark measures S3 PUT and GET latency for a temporary object
// set. It uses only the standard S3 API and removes all temporary objects.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/teamgram/teamgram-server/tools/s3support"
)

const (
	defaultSizeMiB           = 16
	defaultIterations        = 8
	defaultConcurrency       = 4
	defaultPartSizeMiB       = 16
	defaultUploadConcurrency = 4
	defaultTimeout           = 120 * time.Second
)

type sample struct {
	duration time.Duration
	bytes    int64
	ok       bool
}

type metricSummary struct {
	Count           int     `json:"count"`
	Success         int     `json:"success"`
	Errors          int     `json:"errors"`
	Bytes           int64   `json:"bytes"`
	WallSeconds     float64 `json:"wall_seconds"`
	P50Milliseconds float64 `json:"p50_ms"`
	P95Milliseconds float64 `json:"p95_ms"`
	P99Milliseconds float64 `json:"p99_ms"`
	ThroughputMiBPS float64 `json:"throughput_mib_per_second"`
}

type report struct {
	At                string        `json:"at"`
	Endpoint          string        `json:"endpoint"`
	Bucket            string        `json:"bucket"`
	SizeMiB           int           `json:"size_mib"`
	Iterations        int           `json:"iterations"`
	Concurrency       int           `json:"concurrency"`
	PartSizeMiB       int           `json:"part_size_mib"`
	UploadConcurrency int           `json:"upload_concurrency"`
	Upload            metricSummary `json:"upload"`
	Download          metricSummary `json:"download"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	sizeMiB := flag.Int("size-mib", envInt("S3_BENCHMARK_SIZE_MIB", defaultSizeMiB), "object size in MiB")
	iterations := flag.Int("iterations", envInt("S3_BENCHMARK_ITERATIONS", defaultIterations), "objects per phase")
	concurrency := flag.Int("concurrency", envInt("S3_BENCHMARK_CONCURRENCY", defaultConcurrency), "parallel objects per phase")
	partSizeMiB := flag.Int("part-size-mib", envInt("S3_BENCHMARK_PART_SIZE_MIB", defaultPartSizeMiB), "multipart part size in MiB")
	uploadConcurrency := flag.Int("upload-concurrency", envInt("S3_BENCHMARK_UPLOAD_CONCURRENCY", defaultUploadConcurrency), "multipart parts per object")
	timeoutSeconds := flag.Int("timeout-seconds", envInt("S3_BENCHMARK_TIMEOUT_SECONDS", int(defaultTimeout/time.Second)), "phase timeout")
	jsonOut := flag.String("json-out", strings.TrimSpace(os.Getenv("S3_BENCHMARK_JSON_OUT")), "optional JSON report path")
	keep := flag.Bool("keep", envBool("S3_BENCHMARK_KEEP", false), "keep temporary objects after the benchmark")
	flag.Parse()

	if *sizeMiB <= 0 || *iterations <= 0 || *concurrency <= 0 || *partSizeMiB < 5 || *uploadConcurrency <= 0 || *timeoutSeconds <= 0 {
		log.Fatal("size, iterations, concurrency, upload-concurrency, and timeout must be positive; part-size-mib must be at least 5")
	}

	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("S3_SECRET_KEY"))
	bucket := firstNonEmpty(
		os.Getenv("S3_BENCHMARK_BUCKET"),
		os.Getenv("S3_BUCKET"),
		os.Getenv("S3_PROBE_BUCKET"),
	)
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		log.Fatal("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, and S3_BENCHMARK_BUCKET or S3_BUCKET are required")
	}
	if strings.ContainsAny(bucket, "/ \t\r\n") {
		log.Fatal("S3_BENCHMARK_BUCKET must be a bucket name without path or whitespace")
	}

	secure, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("S3_USE_SSL")))
	if os.Getenv("S3_USE_SSL") == "" {
		secure = false
	} else if err != nil {
		log.Fatalf("S3_USE_SSL must be true or false: %v", err)
	}
	region := firstNonEmpty(os.Getenv("S3_REGION"), "us-east-1")
	client, err := s3support.NewClient(s3support.Config{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		UseSSL:    secure,
		Region:    region,
	})
	if err != nil {
		log.Fatal(err)
	}

	payload := makePayload(*sizeMiB)
	digest := sha256.Sum256(payload)
	prefix := firstNonEmpty(
		os.Getenv("S3_BENCHMARK_PREFIX"),
		fmt.Sprintf("benchmark/%d-%d", time.Now().UnixNano(), os.Getpid()),
	)
	keys := make([]string, *iterations)
	for i := range keys {
		keys[i] = fmt.Sprintf("%s/object-%06d.bin", strings.TrimSuffix(prefix, "/"), i)
	}

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = int64(*partSizeMiB) << 20
		u.Concurrency = *uploadConcurrency
		u.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})

	log.Printf("s3 benchmark endpoint=%s bucket=%s size_mib=%d iterations=%d concurrency=%d part_size_mib=%d upload_concurrency=%d", endpoint, bucket, *sizeMiB, *iterations, *concurrency, *partSizeMiB, *uploadConcurrency)
	phaseTimeout := time.Duration(*timeoutSeconds) * time.Second
	uploadCtx, cancelUpload := context.WithTimeout(context.Background(), phaseTimeout)
	uploadSamples, uploadWall := runUploads(uploadCtx, uploader, bucket, keys, payload, *concurrency)
	cancelUpload()

	downloadKeys := make([]string, 0, len(keys))
	for i, result := range uploadSamples {
		if result.ok {
			downloadKeys = append(downloadKeys, keys[i])
		}
	}
	downloadCtx, cancelDownload := context.WithTimeout(context.Background(), phaseTimeout)
	downloadSamples, downloadWall := runDownloads(downloadCtx, client, bucket, downloadKeys, digest, int64(len(payload)), *concurrency)
	cancelDownload()

	report := report{
		At:                time.Now().UTC().Format(time.RFC3339Nano),
		Endpoint:          endpoint,
		Bucket:            bucket,
		SizeMiB:           *sizeMiB,
		Iterations:        *iterations,
		Concurrency:       *concurrency,
		PartSizeMiB:       *partSizeMiB,
		UploadConcurrency: *uploadConcurrency,
		Upload:            summarize(uploadSamples, uploadWall),
		Download:          summarize(downloadSamples, downloadWall),
	}
	printReport(report)
	if *jsonOut != "" {
		if err := writeReport(*jsonOut, report); err != nil {
			log.Fatal(err)
		}
	}

	if !*keep {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		if err := deleteObjects(cleanupCtx, client, bucket, keys); err != nil {
			log.Fatalf("cleanup failed: %v", err)
		}
		log.Printf("removed %d temporary objects", len(keys))
	}

	if report.Upload.Errors > 0 || report.Download.Errors > 0 {
		os.Exit(1)
	}
}

func runUploads(ctx context.Context, uploader *manager.Uploader, bucket string, keys []string, payload []byte, concurrency int) ([]sample, time.Duration) {
	results := make([]sample, len(keys))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	phaseStarted := time.Now()
	for i, key := range keys {
		wg.Add(1)
		go func(index int, objectKey string) {
			defer wg.Done()
			started := time.Now()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[index] = sample{duration: time.Since(started)}
				return
			}
			defer func() { <-sem }()
			_, err := uploader.Upload(ctx, &s3.PutObjectInput{
				Bucket:        aws.String(bucket),
				Key:           aws.String(objectKey),
				Body:          bytes.NewReader(payload),
				ContentLength: aws.Int64(int64(len(payload))),
				ContentType:   aws.String("application/octet-stream"),
			})
			results[index] = sample{duration: time.Since(started), bytes: int64(len(payload)), ok: err == nil}
			if err != nil {
				log.Printf("upload failed key=%s: %v", objectKey, err)
			}
		}(i, key)
	}
	wg.Wait()
	return results, time.Since(phaseStarted)
}

func runDownloads(ctx context.Context, client *s3.Client, bucket string, keys []string, wantDigest [32]byte, wantSize int64, concurrency int) ([]sample, time.Duration) {
	results := make([]sample, len(keys))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	phaseStarted := time.Now()
	for i, key := range keys {
		wg.Add(1)
		go func(index int, objectKey string) {
			defer wg.Done()
			started := time.Now()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[index] = sample{duration: time.Since(started)}
				return
			}
			defer func() { <-sem }()

			object, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)})
			if err == nil {
				hasher := sha256.New()
				var readSize int64
				readSize, err = io.Copy(hasher, object.Body)
				closeErr := object.Body.Close()
				if err == nil {
					err = closeErr
				}
				if err == nil && readSize != wantSize {
					err = fmt.Errorf("size=%d, want=%d", readSize, wantSize)
				}
				if err == nil && !bytes.Equal(hasher.Sum(nil), wantDigest[:]) {
					err = fmt.Errorf("sha256 mismatch")
				}
			}
			results[index] = sample{duration: time.Since(started), bytes: wantSize, ok: err == nil}
			if err != nil {
				log.Printf("download failed key=%s: %v", objectKey, err)
			}
		}(i, key)
	}
	wg.Wait()
	return results, time.Since(phaseStarted)
}

func summarize(samples []sample, wall time.Duration) metricSummary {
	result := metricSummary{Count: len(samples)}
	durations := make([]float64, 0, len(samples))
	for _, sample := range samples {
		durations = append(durations, float64(sample.duration)/float64(time.Millisecond))
		if sample.ok {
			result.Success++
			result.Bytes += sample.bytes
		}
	}
	result.Errors = result.Count - result.Success
	if result.Count == 0 {
		return result
	}
	sort.Float64s(durations)
	result.P50Milliseconds = percentile(durations, 0.50)
	result.P95Milliseconds = percentile(durations, 0.95)
	result.P99Milliseconds = percentile(durations, 0.99)
	result.WallSeconds = wall.Seconds()
	if result.Success > 0 && result.WallSeconds > 0 {
		result.ThroughputMiBPS = float64(result.Bytes) / (1024 * 1024) / result.WallSeconds
	}
	return result
}

func percentile(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(values))*ratio)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func deleteObjects(ctx context.Context, client *s3.Client, bucket string, keys []string) error {
	for start := 0; start < len(keys); start += 1000 {
		end := start + 1000
		if end > len(keys) {
			end = len(keys)
		}
		identifiers := make([]types.ObjectIdentifier, 0, end-start)
		for _, key := range keys[start:end] {
			identifiers = append(identifiers, types.ObjectIdentifier{Key: aws.String(key)})
		}
		output, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: identifiers, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
		if len(output.Errors) > 0 {
			return fmt.Errorf("delete returned %d object errors", len(output.Errors))
		}
	}
	return nil
}

func makePayload(sizeMiB int) []byte {
	payload := make([]byte, sizeMiB<<20)
	for i := range payload {
		payload[i] = byte(i * 31)
	}
	return payload
}

func printReport(value report) {
	encoded, err := json.Marshal(value)
	if err != nil {
		log.Printf("report encoding failed: %v", err)
		return
	}
	log.Printf("upload count=%d success=%d errors=%d p50=%.2fms p95=%.2fms p99=%.2fms throughput=%.2fMiB/s", value.Upload.Count, value.Upload.Success, value.Upload.Errors, value.Upload.P50Milliseconds, value.Upload.P95Milliseconds, value.Upload.P99Milliseconds, value.Upload.ThroughputMiBPS)
	log.Printf("download count=%d success=%d errors=%d p50=%.2fms p95=%.2fms p99=%.2fms throughput=%.2fMiB/s", value.Download.Count, value.Download.Success, value.Download.Errors, value.Download.P50Milliseconds, value.Download.P95Milliseconds, value.Download.P99Milliseconds, value.Download.ThroughputMiBPS)
	log.Printf("report=%s", encoded)
}

func writeReport(path string, value report) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	log.Printf("wrote report=%s", path)
	return nil
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
