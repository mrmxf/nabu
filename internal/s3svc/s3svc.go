// Package s3svc is nabu's client for the lesson resource store on peasoup cloud
// (S3-compatible). Following the security model, nabu holds ONE shared credential
// (Infisical -> ENV, never logged, never sent to the browser) and mediates all
// access: it mints presigned PUT URLs scoped to server-derived keys, imports from
// a URL server-side, and lists/sizes objects. Reads use public object URLs, so no
// presigned GET is needed.
//
// Config (all ENV; creds from Infisical):
//
//	S3_ENDPOINT     e.g. https://s3.peasoup.example   (custom endpoint)
//	S3_REGION       e.g. us-east-1                    (any; required by SigV4)
//	S3_ACCESS_KEY   the shared credential key id
//	S3_SECRET_KEY   the shared credential secret
//	S3_PUBLIC_BASE  base for public object URLs (defaults to S3_ENDPOINT)
//	S3_MAX_UPLOAD_MB cap for URL-import size (default 200)
package s3svc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const usageCacheTTL = 60 * time.Second

// importFetchTimeout bounds a server-side URL import. It sits well under the
// typical Cloudflare origin timeout (~100s) so a slow source fails here with a
// clear error rather than surfacing to the teacher as a Cloudflare 524 page.
const importFetchTimeout = 60 * time.Second

// ErrImportTimeout is returned by PutFromURL when the source fetch exceeds
// importFetchTimeout. Handlers map it to a 504 so the client shows a timeout
// message instead of a generic 500.
var ErrImportTimeout = fmt.Errorf("source fetch timed out")

// Object is one listed resource.
type Object struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	PublicURL    string    `json:"public_url"`
}

// Usage is the total size + object count under a prefix.
type Usage struct {
	Bytes int64 `json:"bytes"`
	Count int   `json:"count"`
}

// Service is the S3 client. Safe for concurrent use.
type Service struct {
	client     *s3.Client
	presign    *s3.PresignClient
	publicBase string
	maxUpload  int64
	configured bool

	mu    sync.Mutex
	cache map[string]usageEntry
}

type usageEntry struct {
	u  Usage
	at time.Time
}

// New builds the service from the environment. If S3 is not configured (endpoint
// or creds missing) it returns a Service whose Configured() is false; handlers
// should 503 rather than crash.
func New() *Service {
	endpoint := os.Getenv("S3_ENDPOINT")
	region := orDefault(os.Getenv("S3_REGION"), "us-east-1")
	key := os.Getenv("S3_ACCESS_KEY")
	secret := os.Getenv("S3_SECRET_KEY")
	publicBase := orDefault(os.Getenv("S3_PUBLIC_BASE"), endpoint)
	maxMB := int64(200)
	if v, err := strconv.Atoi(os.Getenv("S3_MAX_UPLOAD_MB")); err == nil && v > 0 {
		maxMB = int64(v)
	}

	svc := &Service{publicBase: publicBase, maxUpload: maxMB * 1024 * 1024, cache: map[string]usageEntry{}}
	if endpoint == "" || key == "" || secret == "" {
		return svc // not configured
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(key, secret, "")),
	)
	if err != nil {
		return svc // leave unconfigured; handlers report it
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true // S3-compatible stores address path-style
		// aws-sdk-go-v2 (>=1.65) defaults to adding a CRC32 request checksum
		// (aws-chunked trailer) on uploads, which S3-compatible stores like
		// peasoup/Ceph reject with 400 XAmzContentSHA256Mismatch. Only compute a
		// checksum when the operation actually requires one.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	svc.client = client
	svc.presign = s3.NewPresignClient(client)
	svc.configured = true
	slog.Info("s3 configured", "endpoint", endpoint, "region", region)
	return svc
}

// Configured reports whether S3 is set up (endpoint + creds present).
func (s *Service) Configured() bool { return s.configured }

// PublicURL builds the public object URL: {publicBase}/{bucket}/{key} (path-style).
func (s *Service) PublicURL(bucket, key string) string {
	return strings.TrimRight(s.publicBase, "/") + "/" + bucket + "/" + strings.TrimLeft(key, "/")
}

// PresignPut returns a presigned PUT URL for a direct browser upload to key.
func (s *Service) PresignPut(ctx context.Context, bucket, key, contentType string, ttl time.Duration) (string, error) {
	if !s.configured {
		return "", fmt.Errorf("s3 not configured")
	}
	in := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	req, err := s.presign.PresignPutObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return req.URL, nil
}

// PutFromURL fetches srcURL server-side and stores it at key, returning the public
// URL plus the stored size and content type (so the caller can index it without a
// follow-up HeadObject). Capped at S3_MAX_UPLOAD_MB; the body is buffered in memory
// (teacher-initiated, infrequent) — large media should use a direct presigned upload.
func (s *Service) PutFromURL(ctx context.Context, bucket, key, srcURL string) (string, int64, string, error) {
	if !s.configured {
		return "", 0, "", fmt.Errorf("s3 not configured")
	}
	// Bound the whole import (fetch + read) so a slow source can't outlast the
	// edge proxy; on expiry callers get ErrImportTimeout -> 504.
	ctx, cancel := context.WithTimeout(ctx, importFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", 0, "", fmt.Errorf("bad source url: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", 0, "", ErrImportTimeout
		}
		return "", 0, "", fmt.Errorf("fetch source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", 0, "", fmt.Errorf("source returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.maxUpload+1))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", 0, "", ErrImportTimeout
		}
		return "", 0, "", fmt.Errorf("read source: %w", err)
	}
	if int64(len(data)) > s.maxUpload {
		return "", 0, "", fmt.Errorf("source exceeds %d bytes", s.maxUpload)
	}
	contentType := resp.Header.Get("Content-Type")
	putIn := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}
	if contentType != "" {
		putIn.ContentType = aws.String(contentType)
	}
	if _, err := s.client.PutObject(ctx, putIn); err != nil {
		slog.Warn("s3 put failed", "bucket", bucket, "key", key, "err", err)
		return "", 0, "", fmt.Errorf("put object: %w", err)
	}
	s.invalidate(bucket)
	slog.Info("s3 put ok", "bucket", bucket, "key", key, "bytes", len(data))
	return s.PublicURL(bucket, key), int64(len(data)), contentType, nil
}

// CompletedPart identifies one uploaded part of a multipart upload: the part
// number (1-based, in order) and the ETag S3 returned for it.
type CompletedPart struct {
	PartNumber int32  `json:"part_number"`
	ETag       string `json:"etag"`
}

// CreateMultipart starts a multipart upload and returns its upload id. Used for
// large direct browser uploads: the browser uploads each part to a presigned
// UploadPart URL, then nabu completes. Falls back to nothing if S3 is down.
func (s *Service) CreateMultipart(ctx context.Context, bucket, key, contentType string) (string, error) {
	if !s.configured {
		return "", fmt.Errorf("s3 not configured")
	}
	in := &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := s.client.CreateMultipartUpload(ctx, in)
	if err != nil {
		return "", fmt.Errorf("create multipart: %w", err)
	}
	return aws.ToString(out.UploadId), nil
}

// PresignUploadPart returns a presigned PUT URL for one part of a multipart
// upload. partNumber is 1-based. The browser PUTs the part bytes to this URL and
// reads the ETag from the response (the bucket CORS exposes ETag).
func (s *Service) PresignUploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int32, ttl time.Duration) (string, error) {
	if !s.configured {
		return "", fmt.Errorf("s3 not configured")
	}
	req, err := s.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presign upload part: %w", err)
	}
	return req.URL, nil
}

// CompleteMultipart finalises a multipart upload from the parts the browser
// uploaded and returns the object's public URL. Parts are sorted by number and
// the usage cache is invalidated so the dashboard figure recomputes.
func (s *Service) CompleteMultipart(ctx context.Context, bucket, key, uploadID string, parts []CompletedPart) (string, error) {
	if !s.configured {
		return "", fmt.Errorf("s3 not configured")
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("no parts to complete")
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	cp := make([]s3types.CompletedPart, len(parts))
	for i, p := range parts {
		cp[i] = s3types.CompletedPart{ETag: aws.String(p.ETag), PartNumber: aws.Int32(p.PartNumber)}
	}
	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{Parts: cp},
	})
	if err != nil {
		slog.Warn("s3 complete multipart failed", "bucket", bucket, "key", key, "err", err)
		return "", fmt.Errorf("complete multipart: %w", err)
	}
	s.invalidate(bucket)
	slog.Info("s3 multipart complete", "bucket", bucket, "key", key, "parts", len(parts))
	return s.PublicURL(bucket, key), nil
}

// AbortMultipart cancels an in-progress multipart upload, discarding any parts
// already uploaded. Best-effort cleanup when the browser gives up or fails.
func (s *Service) AbortMultipart(ctx context.Context, bucket, key, uploadID string) error {
	if !s.configured {
		return fmt.Errorf("s3 not configured")
	}
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		slog.Warn("s3 abort multipart failed", "bucket", bucket, "key", key, "err", err)
		return fmt.Errorf("abort multipart: %w", err)
	}
	slog.Info("s3 multipart aborted", "bucket", bucket, "key", key)
	return nil
}

// List returns the objects under prefix, each with its public URL.
func (s *Service) List(ctx context.Context, bucket, prefix string) ([]Object, error) {
	if !s.configured {
		return nil, fmt.Errorf("s3 not configured")
	}
	var out []Object
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket), Prefix: aws.String(prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			slog.Warn("s3 access failed", "op", "list", "bucket", bucket, "err", err)
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, o := range page.Contents {
			key := aws.ToString(o.Key)
			out = append(out, Object{
				Key:          key,
				Size:         aws.ToInt64(o.Size),
				LastModified: aws.ToTime(o.LastModified),
				PublicURL:    s.PublicURL(bucket, key),
			})
		}
	}
	slog.Info("s3 access ok", "op", "list", "bucket", bucket, "objects", len(out))
	return out, nil
}

// Usage returns total bytes + count under prefix, cached briefly (ListObjectsV2 is
// paginated/expensive).
func (s *Service) Usage(ctx context.Context, bucket, prefix string) (Usage, error) {
	if !s.configured {
		return Usage{}, fmt.Errorf("s3 not configured")
	}
	ck := bucket + "\x00" + prefix
	s.mu.Lock()
	if e, ok := s.cache[ck]; ok && time.Since(e.at) < usageCacheTTL {
		s.mu.Unlock()
		return e.u, nil
	}
	s.mu.Unlock()

	var u Usage
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket), Prefix: aws.String(prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return Usage{}, fmt.Errorf("usage list: %w", err)
		}
		for _, o := range page.Contents {
			u.Bytes += aws.ToInt64(o.Size)
			u.Count++
		}
	}
	s.mu.Lock()
	s.cache[ck] = usageEntry{u: u, at: time.Now()}
	s.mu.Unlock()
	return u, nil
}

// PutBucketCORS applies a CORS policy to bucket. configJSON is the aws-CLI-shaped
// document ({"CORSRules":[...]}) — the same file the aws CLI would take — so the
// embedded policy stays portable. This is an administrative operation (it changes
// who may call the bucket from a browser); callers gate it behind confirmation.
func (s *Service) PutBucketCORS(ctx context.Context, bucket string, configJSON []byte) error {
	if !s.configured {
		return fmt.Errorf("s3 not configured")
	}
	var parsed struct {
		CORSRules []s3types.CORSRule `json:"CORSRules"`
	}
	if err := json.Unmarshal(configJSON, &parsed); err != nil {
		return fmt.Errorf("parse cors config: %w", err)
	}
	if len(parsed.CORSRules) == 0 {
		return fmt.Errorf("cors config has no CORSRules")
	}
	_, err := s.client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket:            aws.String(bucket),
		CORSConfiguration: &s3types.CORSConfiguration{CORSRules: parsed.CORSRules},
	})
	if err != nil {
		slog.Warn("s3 put bucket cors failed", "bucket", bucket, "err", err)
		return fmt.Errorf("put bucket cors: %w", err)
	}
	slog.Info("s3 bucket cors applied", "bucket", bucket, "rules", len(parsed.CORSRules))
	return nil
}

// HeadBucket cheaply proves a bucket is reachable with the configured credentials
// and endpoint (used by `nabu admin validate` for read-only reachability). It does
// not list or touch any objects.
func (s *Service) HeadBucket(ctx context.Context, bucket string) error {
	if !s.configured {
		return fmt.Errorf("s3 not configured")
	}
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return fmt.Errorf("head bucket: %w", err)
	}
	return nil
}

// PutProbe writes a tiny object to prove write access (used by
// `nabu admin validate --deep`). The caller removes it afterwards with Delete.
func (s *Service) PutProbe(ctx context.Context, bucket, key string) error {
	if !s.configured {
		return fmt.Errorf("s3 not configured")
	}
	body := []byte("nabu validate probe\n")
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String("text/plain"),
	})
	if err != nil {
		slog.Warn("s3 probe put failed", "bucket", bucket, "key", key, "err", err)
		return fmt.Errorf("probe put: %w", err)
	}
	return nil
}

// Delete removes a single object and invalidates the bucket's usage cache so the
// dashboard figure recomputes. The caller is responsible for confirming the key
// is one the requester is allowed to remove (see resourceDeleteHandler).
func (s *Service) Delete(ctx context.Context, bucket, key string) error {
	if !s.configured {
		return fmt.Errorf("s3 not configured")
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); err != nil {
		slog.Warn("s3 delete failed", "bucket", bucket, "key", key, "err", err)
		return fmt.Errorf("delete object: %w", err)
	}
	s.invalidate(bucket)
	slog.Info("s3 delete ok", "bucket", bucket, "key", key)
	return nil
}

// Copy server-side-copies srcKey to dstKey within the same bucket, preserving the
// object's content type and metadata (CopyObject's default directive). It is the
// first half of a rename (the caller then Delete()s the source). The copy source
// is path-escaped so keys containing spaces or reserved characters are handled.
func (s *Service) Copy(ctx context.Context, bucket, srcKey, dstKey string) error {
	if !s.configured {
		return fmt.Errorf("s3 not configured")
	}
	// CopySource must be "{bucket}/{key}", URL-encoded but with slashes preserved.
	source := (&url.URL{Path: bucket + "/" + srcKey}).EscapedPath()
	if _, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(source),
	}); err != nil {
		slog.Warn("s3 copy failed", "bucket", bucket, "src", srcKey, "dst", dstKey, "err", err)
		return fmt.Errorf("copy object: %w", err)
	}
	s.invalidate(bucket)
	slog.Info("s3 copy ok", "bucket", bucket, "src", srcKey, "dst", dstKey)
	return nil
}

// InvalidateUsage drops any cached Usage for bucket so the next Usage() recomputes
// from a fresh listing. Callers use it after a direct (presigned) upload, which
// the server never sees and so cannot invalidate on its own.
func (s *Service) InvalidateUsage(bucket string) { s.invalidate(bucket) }

func (s *Service) invalidate(bucket string) {
	s.mu.Lock()
	for k := range s.cache {
		if strings.HasPrefix(k, bucket+"\x00") {
			delete(s.cache, k)
		}
	}
	s.mu.Unlock()
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SafeFilename reduces an uploaded filename to a safe key segment: base name only
// (no path), unsafe runs collapsed to '-', no leading dots. Empty => "file".
func SafeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = unsafeChars.ReplaceAllString(name, "-")
	name = strings.TrimLeft(name, ".-")
	if name == "" {
		return "file"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
