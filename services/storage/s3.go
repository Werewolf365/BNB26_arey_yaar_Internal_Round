// S3-compatible content-addressed evidence blob backend.
//
// Object key is the content key verbatim (sha256/<64 lowercase hex>).
// Requests are signed in-file with SigV4 (AWS4-HMAC-SHA256, service s3)
// using UNSIGNED-PAYLOAD, which MinIO and LocalStack accept. Reads are
// hash-verified exactly like the Filesystem backend (AUDIT_TAMPERED on
// mismatch). Errors never include credentials.
package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// S3Config configures the S3-compatible backend. Endpoint selects the
// backend (e.g. http://localhost:4566 for the compose LocalStack); all
// other fields tune it. HTTPClient and Timeout are optional overrides.
type S3Config struct {
	Endpoint     string
	Bucket       string
	Region       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
	HTTPClient   *http.Client
	Timeout      time.Duration
}

// S3 is a Backend over S3-compatible object storage (MinIO, LocalStack,
// AWS S3). Object key = content key verbatim.
type S3 struct {
	endpoint string
	bucket   string
	region   string
	access   string
	secret   string
	pathCode bool
	client   *http.Client
}

var _ Backend = (*S3)(nil)

// NewS3 validates config (no network I/O; call EnsureBucket to fail fast
// on unreachable storage or a missing bucket).
func NewS3(cfg S3Config) (*S3, error) {
	ep := strings.TrimSpace(cfg.Endpoint)
	if ep == "" {
		return nil, fmt.Errorf("INVALID_INPUT: S3 endpoint required")
	}
	u, err := url.Parse(ep)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("INVALID_INPUT: bad S3 endpoint %q", ep)
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" || strings.Contains(bucket, "/") {
		return nil, fmt.Errorf("INVALID_INPUT: S3 bucket required")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("INVALID_INPUT: S3 credentials required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &S3{
		endpoint: strings.TrimSuffix(ep, "/"),
		bucket:   bucket,
		region:   region,
		access:   cfg.AccessKey,
		secret:   cfg.SecretKey,
		pathCode: cfg.UsePathStyle,
		client:   client,
	}, nil
}

// EnsureBucket fails fast when storage is unreachable and creates the
// bucket when it does not exist yet (idempotent; safe at startup).
func (s *S3) EnsureBucket(ctx context.Context) error {
	head, err := http.NewRequestWithContext(ctx, http.MethodHead, s.bucketURL(), nil)
	if err != nil {
		return fmt.Errorf("INVALID_INPUT: bad S3 endpoint: %v", err)
	}
	s.sign(head, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := s.client.Do(head)
	if err != nil {
		return fmt.Errorf("UNAVAILABLE: s3 bucket HEAD failed: %v", err)
	}
	drainClose(resp)
	switch resp.StatusCode {
	case 200:
		return nil
	case 403:
		return fmt.Errorf("UNAVAILABLE: s3 bucket HEAD status 403 (check access key/region)")
	case 404:
		// Missing bucket: create it, then it must HEAD 200.
		if err := s.createBucket(ctx); err != nil {
			return err
		}
		return nil
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("UNAVAILABLE: s3 bucket HEAD status %d", resp.StatusCode)
		}
		return fmt.Errorf("UNAVAILABLE: s3 bucket HEAD status %d", resp.StatusCode)
	}
}

func (s *S3) createBucket(ctx context.Context) error {
	var body []byte
	if s.region != "us-east-1" {
		body = []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<CreateBucketConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
			`<LocationConstraint>` + s.region + `</LocationConstraint>` +
			`</CreateBucketConfiguration>`)
	}
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, s.bucketURL(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("INVALID_INPUT: bad S3 endpoint: %v", err)
	}
	if len(body) > 0 {
		put.Header.Set("Content-Type", "application/xml")
	}
	s.sign(put, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := s.client.Do(put)
	if err != nil {
		return fmt.Errorf("UNAVAILABLE: s3 bucket create failed: %v", err)
	}
	drainClose(resp)
	if resp.StatusCode != 200 {
		return fmt.Errorf("UNAVAILABLE: s3 bucket create status %d", resp.StatusCode)
	}
	return nil
}

// Put stores data under its content key (idempotent: same bytes, same key).
func (s *S3) Put(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("INVALID_INPUT: empty blob")
	}
	if len(data) > MaxBlobBytes {
		return "", fmt.Errorf("ARTIFACT_OVERSIZED: %d bytes exceeds %d", len(data), MaxBlobBytes)
	}
	sum := sha256.Sum256(data)
	key := "sha256/" + hex.EncodeToString(sum[:])
	req, err := http.NewRequest(http.MethodPut, s.objectURL(key), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("INVALID_INPUT: bad S3 endpoint: %v", err)
	}
	s.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("UNAVAILABLE: s3 put failed: %v", err)
	}
	drainClose(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("UNAVAILABLE: s3 put status %d", resp.StatusCode)
	}
	return key, nil
}

// Get retrieves a blob and re-verifies its hash (tamper-evident reads).
func (s *S3) Get(key string) ([]byte, error) {
	if _, err := validateContentKey(key); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, s.objectURL(key), nil)
	if err != nil {
		return nil, fmt.Errorf("INVALID_INPUT: bad S3 endpoint: %v", err)
	}
	s.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("UNAVAILABLE: s3 get failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("EVIDENCE_NOT_FOUND: %s", key)
	}
	if resp.StatusCode != 200 {
		drainClose(resp)
		return nil, fmt.Errorf("UNAVAILABLE: s3 get status %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxBlobBytes {
		drainClose(resp)
		return nil, fmt.Errorf("ARTIFACT_OVERSIZED: stored blob exceeds cap")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBlobBytes+1))
	if err != nil {
		return nil, fmt.Errorf("UNAVAILABLE: s3 get read failed: %v", err)
	}
	if len(data) > MaxBlobBytes {
		return nil, fmt.Errorf("ARTIFACT_OVERSIZED: stored blob exceeds cap")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.TrimPrefix(key, "sha256/") {
		return nil, fmt.Errorf("AUDIT_TAMPERED: stored blob %s fails hash check", key)
	}
	return data, nil
}

// Stat reports existence without reading the blob.
func (s *S3) Stat(key string) (bool, error) {
	if _, err := validateContentKey(key); err != nil {
		return false, err
	}
	req, err := http.NewRequest(http.MethodHead, s.objectURL(key), nil)
	if err != nil {
		return false, fmt.Errorf("INVALID_INPUT: bad S3 endpoint: %v", err)
	}
	s.sign(req, "UNSIGNED-PAYLOAD", time.Now().UTC())
	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("UNAVAILABLE: s3 head failed: %v", err)
	}
	drainClose(resp)
	if resp.StatusCode == 200 {
		return true, nil
	}
	if resp.StatusCode == 404 {
		return false, nil
	}
	return false, fmt.Errorf("UNAVAILABLE: s3 head status %d", resp.StatusCode)
}

// bucketURL is the bucket-level URL (HEAD bucket / PUT bucket).
func (s *S3) bucketURL() string {
	if s.pathCode {
		return s.endpoint + "/" + s.bucket
	}
	u, _ := url.Parse(s.endpoint)
	u.Host = s.bucket + "." + u.Host
	u.Path = "/"
	return u.String()
}

// objectURL maps a content key to its object URL. Keys are restricted to
// [0-9a-f/] by validateContentKey, so segment escaping is identity, but
// the escaping is applied anyway to keep signing exact.
func (s *S3) objectURL(key string) string {
	escaped := escapeS3Path(key)
	if s.pathCode {
		return s.endpoint + "/" + s.bucket + "/" + escaped
	}
	u, _ := url.Parse(s.endpoint)
	u.Host = s.bucket + "." + u.Host
	u.RawPath = "/" + escaped
	u.Path = "/" + key
	return u.String()
}

func escapeS3Path(p string) string {
	segs := strings.Split(p, "/")
	for i, sg := range segs {
		segs[i] = url.PathEscape(sg)
	}
	return strings.Join(segs, "/")
}

// sign applies SigV4 (AWS4-HMAC-SHA256, service s3) with an unsigned
// payload, which S3-compat stores (MinIO, LocalStack) accept.
func (s *S3) sign(req *http.Request, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + strings.TrimSpace(host) + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	canonicalRequest := req.Method + "\n" + canonicalURI + "\n\n" +
		canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	scope := dateStamp + "/" + s.region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" +
		hex.EncodeToString(sum[:])
	kDate := hmacSHA256([]byte("AWS4"+s.secret), dateStamp)
	kRegion := hmacSHA256(kDate, s.region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.access+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func drainClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	resp.Body.Close()
}
