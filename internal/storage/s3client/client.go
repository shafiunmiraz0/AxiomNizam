package s3client

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

// Client is a native S3-compatible HTTP client implementing AWS Signature V4.
// It replaces the MinIO SDK with direct HTTP calls, following the patterns
// from MinIO's cmd/bucket-handlers.go and cmd/object-handlers.go.
type Client struct {
	httpClient *http.Client
	endpoint   *url.URL
	cred       Credentials
}

// NewClient creates a new S3 HTTP client.
func NewClient(endpoint string, accessKey, secretKey, region string, useSSL bool) (*Client, error) {
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	u, err := url.Parse(fmt.Sprintf("%s://%s", scheme, endpoint))
	if err != nil {
		return nil, fmt.Errorf("storage: invalid endpoint %q: %w", endpoint, err)
	}

	logging.Z().Info(fmt.Sprintf("✅ Storage: native S3 client connected to %s (ssl=%v, region=%s)", endpoint, useSSL, region))

	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		endpoint:   u,
		cred: Credentials{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
			Region:          region,
		},
	}, nil
}

// Ping checks connectivity to the storage backend by listing buckets.
func (c *Client) Ping(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: ping request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusMethodNotAllowed:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		// Treat permission-denied responses as reachable if auth is structurally valid.
		err := parseS3Error(resp)
		if s3Err, ok := err.(*S3Error); ok {
			switch s3Err.Code {
			case "AccessDenied", "AllAccessDisabled", "AuthorizationHeaderMalformed":
				return nil
			case "InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken", "InvalidToken":
				return fmt.Errorf("storage: ping auth validation failed: %w", err)
			default:
				return nil
			}
		}
		return nil
	default:
		if resp.StatusCode >= http.StatusInternalServerError {
			return parseS3Error(resp)
		}
		return fmt.Errorf("storage: ping returned unexpected status %d", resp.StatusCode)
	}
}

// ---------- Bucket Operations ----------

// S3 XML types for bucket operations (following MinIO's cmd/api-response.go patterns).

type createBucketConfiguration struct {
	XMLName  xml.Name `xml:"CreateBucketConfiguration"`
	Location string   `xml:"LocationConstraint"`
}

type listAllMyBucketsResult struct {
	XMLName xml.Name     `xml:"ListAllMyBucketsResult"`
	Owner   s3Owner      `xml:"Owner"`
	Buckets s3BucketList `xml:"Buckets"`
}

type s3Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type s3BucketList struct {
	Bucket []s3Bucket `xml:"Bucket"`
}

type s3Bucket struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

// CreateBucket creates a new bucket. Idempotent — returns nil if already exists.
func (c *Client) CreateBucket(ctx context.Context, name string) error {
	body := createBucketConfiguration{Location: c.cred.Region}
	xmlBytes, err := xml.Marshal(body)
	if err != nil {
		return fmt.Errorf("storage: failed to marshal create bucket request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPut, "/"+name, bytes.NewReader(xmlBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	c.signRequest(req, xmlBytes)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: create bucket %q request failed: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		logging.Z().Info(fmt.Sprintf("✅ Storage: bucket %q created", name))
		return nil
	}
	if resp.StatusCode == http.StatusConflict {
		s3Err := parseS3Error(resp)
		if isBucketExistsError(s3Err) {
			return nil // idempotent
		}
		return s3Err
	}
	return parseS3Error(resp)
}

// DeleteBucket removes a bucket. The bucket must be empty.
func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/"+name, nil)
	if err != nil {
		return err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete bucket %q request failed: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		logging.Z().Info(fmt.Sprintf("✅ Storage: bucket %q deleted", name))
		return nil
	}
	return parseS3Error(resp)
}

// BucketExists returns true if the bucket exists.
func (c *Client) BucketExists(ctx context.Context, name string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodHead, "/"+name, nil)
	if err != nil {
		return false, err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("storage: bucket exists check %q: %w", name, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("storage: bucket exists check %q returned status %d", name, resp.StatusCode)
	}
}

// ListBuckets returns the names of all buckets.
func (c *Client) ListBuckets(ctx context.Context) ([]string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return nil, err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: list buckets request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseS3Error(resp)
	}

	var result listAllMyBucketsResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("storage: failed to parse list buckets response: %w", err)
	}

	names := make([]string, 0, len(result.Buckets.Bucket))
	for _, b := range result.Buckets.Bucket {
		names = append(names, b.Name)
	}
	return names, nil
}

// ---------- Versioning ----------

type versioningConfiguration struct {
	XMLName xml.Name `xml:"VersioningConfiguration"`
	Status  string   `xml:"Status"`
}

// SetBucketVersioning enables or suspends versioning on a bucket.
func (c *Client) SetBucketVersioning(ctx context.Context, bucket string, enabled bool) error {
	cfg := versioningConfiguration{}
	if enabled {
		cfg.Status = "Enabled"
	} else {
		cfg.Status = "Suspended"
	}

	xmlBytes, err := xml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("storage: failed to marshal versioning config: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPut, "/"+bucket+"?versioning", bytes.NewReader(xmlBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	c.signRequest(req, xmlBytes)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: set versioning on %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		logging.Z().Info(fmt.Sprintf("✅ Storage: versioning on %q set to %s", bucket, cfg.Status))
		return nil
	}
	return parseS3Error(resp)
}

// GetBucketVersioning returns true if versioning is enabled on the bucket.
func (c *Client) GetBucketVersioning(ctx context.Context, bucket string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/"+bucket+"?versioning", nil)
	if err != nil {
		return false, err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("storage: get versioning for %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, parseS3Error(resp)
	}

	var cfg versioningConfiguration
	if err := xml.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return false, fmt.Errorf("storage: parse versioning response for %q: %w", bucket, err)
	}
	return cfg.Status == "Enabled", nil
}

// ---------- Lifecycle ----------

type lifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	Rules   []lifecycleRule `xml:"Rule"`
}

type lifecycleRule struct {
	ID                          string                       `xml:"ID"`
	Status                      string                       `xml:"Status"`
	Filter                      lifecycleFilter              `xml:"Filter"`
	Expiration                  *lifecycleExpiration         `xml:"Expiration,omitempty"`
	NoncurrentVersionExpiration *noncurrentVersionExpiration `xml:"NoncurrentVersionExpiration,omitempty"`
}

type lifecycleFilter struct {
	Prefix string `xml:"Prefix"`
}

type lifecycleExpiration struct {
	Days int `xml:"Days"`
}

type noncurrentVersionExpiration struct {
	NoncurrentDays int `xml:"NoncurrentDays"`
}

// SetBucketLifecycle applies lifecycle rules to a bucket.
func (c *Client) SetBucketLifecycle(ctx context.Context, bucket string, rules []models.LifecycleRule) error {
	if len(rules) == 0 {
		return nil
	}

	lcConfig := lifecycleConfiguration{}
	for _, r := range rules {
		rule := lifecycleRule{
			ID:     r.ID,
			Status: "Enabled",
			Filter: lifecycleFilter{Prefix: r.Prefix},
		}
		if r.ExpirationDays > 0 {
			rule.Expiration = &lifecycleExpiration{Days: r.ExpirationDays}
		}
		if r.NoncurrentExpirationDays > 0 {
			rule.NoncurrentVersionExpiration = &noncurrentVersionExpiration{NoncurrentDays: r.NoncurrentExpirationDays}
		}
		lcConfig.Rules = append(lcConfig.Rules, rule)
	}

	xmlBytes, err := xml.Marshal(lcConfig)
	if err != nil {
		return fmt.Errorf("storage: failed to marshal lifecycle config: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPut, "/"+bucket+"?lifecycle", bytes.NewReader(xmlBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	c.signRequest(req, xmlBytes)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: set lifecycle on %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		logging.Z().Info(fmt.Sprintf("✅ Storage: lifecycle policy applied to %q (%d rules)", bucket, len(rules)))
		return nil
	}
	return parseS3Error(resp)
}

// ---------- Object Operations ----------

// PutObject uploads an object to the specified bucket.
func (c *Client) PutObject(ctx context.Context, bucket, key string, data io.Reader, size int64, contentType string) error {
	path := "/" + bucket + "/" + key

	var body io.Reader = data
	var bodyBytes []byte

	// For small objects, buffer to compute content hash
	if size >= 0 && size < 5*1024*1024 {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, data); err != nil {
			return fmt.Errorf("storage: failed to buffer object data: %w", err)
		}
		bodyBytes = buf.Bytes()
		body = bytes.NewReader(bodyBytes)
	}

	req, err := c.newRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if size >= 0 {
		req.ContentLength = size
		req.Header.Set("Content-Length", strconv.FormatInt(size, 10))
	}

	c.signRequest(req, bodyBytes)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: put object %q in %q: %w", key, bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}
	return parseS3Error(resp)
}

// GetObject retrieves an object from the specified bucket.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	path := "/" + bucket + "/" + key

	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: get object %q from %q: %w", key, bucket, err)
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return resp.Body, nil
	}

	defer resp.Body.Close()
	return nil, parseS3Error(resp)
}

// DeleteObject removes an object from the specified bucket.
func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	path := "/" + bucket + "/" + key

	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete object %q from %q: %w", key, bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	return parseS3Error(resp)
}

// S3 XML types for list objects (ListObjectsV2 API).

type listBucketResult struct {
	XMLName               xml.Name   `xml:"ListBucketResult"`
	Name                  string     `xml:"Name"`
	Prefix                string     `xml:"Prefix"`
	KeyCount              int        `xml:"KeyCount"`
	MaxKeys               int        `xml:"MaxKeys"`
	IsTruncated           bool       `xml:"IsTruncated"`
	NextContinuationToken string     `xml:"NextContinuationToken"`
	Contents              []s3Object `xml:"Contents"`
}

type s3Object struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

// ListObjects lists objects in a bucket with an optional prefix filter.
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]models.ObjectInfo, error) {
	var allObjects []models.ObjectInfo
	continuationToken := ""

	for {
		query := url.Values{}
		query.Set("list-type", "2")
		query.Set("max-keys", "1000")
		if prefix != "" {
			query.Set("prefix", prefix)
		}
		if continuationToken != "" {
			query.Set("continuation-token", continuationToken)
		}

		req, err := c.newRequest(ctx, http.MethodGet, "/"+bucket+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		c.signRequest(req, nil)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("storage: list objects in %q: %w", bucket, err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, parseS3Error(resp)
		}

		var result listBucketResult
		if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("storage: parse list objects response for %q: %w", bucket, err)
		}
		resp.Body.Close()

		for _, obj := range result.Contents {
			lastMod, _ := time.Parse(time.RFC3339, obj.LastModified)
			allObjects = append(allObjects, models.ObjectInfo{
				Key:          obj.Key,
				Size:         obj.Size,
				ETag:         strings.Trim(obj.ETag, "\""),
				LastModified: lastMod,
			})
		}

		if !result.IsTruncated || result.NextContinuationToken == "" {
			break
		}
		continuationToken = result.NextContinuationToken
	}

	return allObjects, nil
}

// StatObject returns metadata for a specific object using HEAD.
func (c *Client) StatObject(ctx context.Context, bucket, key string) (*models.ObjectInfo, error) {
	path := "/" + bucket + "/" + key

	req, err := c.newRequest(ctx, http.MethodHead, path, nil)
	if err != nil {
		return nil, err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: stat object %q in %q: %w", key, bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("storage: object %q not found in %q (status %d)", key, bucket, resp.StatusCode)
	}

	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	lastMod, _ := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified"))

	return &models.ObjectInfo{
		Key:          key,
		Size:         size,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         strings.Trim(resp.Header.Get("ETag"), "\""),
		LastModified: lastMod,
		VersionID:    resp.Header.Get("x-amz-version-id"),
	}, nil
}

// ---------- Pre-signed URLs ----------

// PresignGetObject generates a pre-signed GET URL for downloading an object.
func (c *Client) PresignGetObject(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	return PresignURL(http.MethodGet, c.endpoint, bucket, key, c.cred, expires), nil
}

// PresignPutObject generates a pre-signed PUT URL for uploading an object.
func (c *Client) PresignPutObject(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	return PresignURL(http.MethodPut, c.endpoint, bucket, key, c.cred, expires), nil
}

// Endpoint returns the storage backend endpoint.
func (c *Client) Endpoint() string {
	return c.endpoint.Host
}

// ---------- Internal helpers ----------

// newRequest creates a new HTTP request targeting the S3 endpoint.
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	u := *c.endpoint
	// Parse path and query separately
	parts := strings.SplitN(path, "?", 2)
	u.Path = parts[0]
	if len(parts) > 1 {
		u.RawQuery = parts[1]
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to create request: %w", err)
	}
	req.Header.Set("Host", c.endpoint.Host)
	return req, nil
}

// signRequest signs the request using AWS Signature V4.
func (c *Client) signRequest(req *http.Request, bodyBytes []byte) {
	payloadHash := unsignedPayload
	if bodyBytes != nil {
		payloadHash = sha256Hex(bodyBytes)
	}
	SignRequest(req, c.cred, payloadHash)
}

// ---------- Tagging ----------

type tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	TagSet  tagSet   `xml:"TagSet"`
}

type tagSet struct {
	Tags []xmlTag `xml:"Tag"`
}

type xmlTag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

// GetBucketTagging returns the tags for a bucket.
func (c *Client) GetBucketTagging(ctx context.Context, bucket string) ([]models.BucketTag, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/"+bucket+"?tagging", nil)
	if err != nil {
		return nil, err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: get tags for %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseS3Error(resp)
	}

	var t tagging
	if err := xml.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("storage: parse tags for %q: %w", bucket, err)
	}

	tags := make([]models.BucketTag, 0, len(t.TagSet.Tags))
	for _, xt := range t.TagSet.Tags {
		tags = append(tags, models.BucketTag{Key: xt.Key, Value: xt.Value})
	}
	return tags, nil
}

// PutBucketTagging sets the tags on a bucket.
func (c *Client) PutBucketTagging(ctx context.Context, bucket string, tags []models.BucketTag) error {
	t := tagging{TagSet: tagSet{Tags: make([]xmlTag, 0, len(tags))}}
	for _, tag := range tags {
		t.TagSet.Tags = append(t.TagSet.Tags, xmlTag{Key: tag.Key, Value: tag.Value})
	}

	xmlBytes, err := xml.Marshal(t)
	if err != nil {
		return fmt.Errorf("storage: marshal tags: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPut, "/"+bucket+"?tagging", bytes.NewReader(xmlBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	c.signRequest(req, xmlBytes)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: set tags on %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return parseS3Error(resp)
}

// DeleteBucketTagging removes all tags from a bucket.
func (c *Client) DeleteBucketTagging(ctx context.Context, bucket string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/"+bucket+"?tagging", nil)
	if err != nil {
		return err
	}
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete tags from %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	return parseS3Error(resp)
}

// ---------- Copy Object ----------

// CopyObject copies an object within or across buckets using the S3 COPY API.
func (c *Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	path := "/" + dstBucket + "/" + dstKey

	req, err := c.newRequest(ctx, http.MethodPut, path, nil)
	if err != nil {
		return err
	}
	copySource := "/" + srcBucket + "/" + srcKey
	req.Header.Set("x-amz-copy-source", copySource)
	c.signRequest(req, nil)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: copy %q/%q to %q/%q: %w", srcBucket, srcKey, dstBucket, dstKey, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		logging.Z().Info(fmt.Sprintf("✅ Storage: copied %s/%s → %s/%s", srcBucket, srcKey, dstBucket, dstKey))
		return nil
	}
	return parseS3Error(resp)
}

// ---------- Multi-Delete ----------

type deleteXML struct {
	XMLName xml.Name       `xml:"Delete"`
	Quiet   bool           `xml:"Quiet"`
	Objects []deleteObjXML `xml:"Object"`
}

type deleteObjXML struct {
	Key       string `xml:"Key"`
	VersionId string `xml:"VersionId,omitempty"`
}

type deleteResult struct {
	XMLName xml.Name      `xml:"DeleteResult"`
	Deleted []deletedObj  `xml:"Deleted"`
	Errors  []deleteError `xml:"Error"`
}

type deletedObj struct {
	Key string `xml:"Key"`
}

type deleteError struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// MultiDeleteObjects deletes multiple objects in a single request.
func (c *Client) MultiDeleteObjects(ctx context.Context, bucket string, keys []string) (int, []string, error) {
	d := deleteXML{Quiet: false}
	for _, k := range keys {
		d.Objects = append(d.Objects, deleteObjXML{Key: k})
	}

	xmlBytes, err := xml.Marshal(d)
	if err != nil {
		return 0, nil, fmt.Errorf("storage: marshal multi-delete: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/"+bucket+"?delete", bytes.NewReader(xmlBytes))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/xml")
	c.signRequest(req, xmlBytes)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("storage: multi-delete in %q: %w", bucket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, nil, parseS3Error(resp)
	}

	var result deleteResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, nil, fmt.Errorf("storage: parse multi-delete response: %w", err)
	}

	var errs []string
	for _, e := range result.Errors {
		errs = append(errs, e.Key+": "+e.Message)
	}
	return len(result.Deleted), errs, nil
}

// ---------- Bucket Size Estimation ----------

// GetBucketSize returns total size and count of objects in a bucket.
func (c *Client) GetBucketSize(ctx context.Context, bucket string) (int64, int64, error) {
	objects, err := c.ListObjects(ctx, bucket, "")
	if err != nil {
		return 0, 0, err
	}
	var totalSize int64
	for _, o := range objects {
		totalSize += o.Size
	}
	return totalSize, int64(len(objects)), nil
}
