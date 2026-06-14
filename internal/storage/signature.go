package storage

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/storage/access"
	"axiomnizam.bitbd.net/axiomnizam/internal/storage/models"
)

const (
	sigV4Algorithm                   = "AWS4-HMAC-SHA256"
	sigV4TimeLayout                  = "20060102T150405Z"
	sigV4DateLayout                  = "20060102"
	sigV4Service                     = "s3"
	sigV4Term                        = "aws4_request"
	sigV4UnsignedPayload             = "UNSIGNED-PAYLOAD"
	sigV4MaxPresignSeconds           = 7 * 24 * 60 * 60
	defaultPresignRegion             = "us-east-1"
	defaultPresignedRateLimitPerMins = 240
)

var (
	errInvalidPresignedRequest = errors.New("invalid presigned request")
	errExpiredPresignedURL     = errors.New("presigned URL expired")
)

type presignedRequestInfo struct {
	AccessKeyID string
	UserID      string
	TenantID    string
	Bucket      string
	ObjectKey   string
	Method      string
}

type presignedCtxKey string

const presignedRequestInfoKey presignedCtxKey = "storage_presigned_request_info"

type presignedMiddlewareConfig struct {
	resolveAccessKey func(string) (*models.AccessKey, error)
	limiter          *fixedWindowLimiter
}

var presignedMiddlewareState struct {
	mu  sync.RWMutex
	cfg presignedMiddlewareConfig
}

type fixedWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	items  map[string]*windowCounter
}

type windowCounter struct {
	windowStart time.Time
	count       int
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	if limit <= 0 {
		limit = defaultPresignedRateLimitPerMins
	}
	if window <= 0 {
		window = time.Minute
	}
	return &fixedWindowLimiter{
		limit:  limit,
		window: window,
		items:  make(map[string]*windowCounter),
	}
}

func (l *fixedWindowLimiter) allow(key string, now time.Time) (bool, int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.items[key]
	if state == nil || now.Sub(state.windowStart) >= l.window {
		state = &windowCounter{windowStart: now}
		l.items[key] = state
	}
	state.count++

	remaining := l.limit - state.count
	if remaining < 0 {
		remaining = 0
	}
	resetAt := state.windowStart.Add(l.window)
	return state.count <= l.limit, remaining, resetAt
}

func ConfigurePresignedMiddleware(resolveAccessKey func(string) (*models.AccessKey, error), rateLimitPerMinute int) {
	if rateLimitPerMinute <= 0 {
		rateLimitPerMinute = defaultPresignedRateLimitPerMins
	}
	presignedMiddlewareState.mu.Lock()
	presignedMiddlewareState.cfg = presignedMiddlewareConfig{
		resolveAccessKey: resolveAccessKey,
		limiter:          newFixedWindowLimiter(rateLimitPerMinute, time.Minute),
	}
	presignedMiddlewareState.mu.Unlock()
}

func getPresignedRequestInfo(ctx context.Context) (*presignedRequestInfo, bool) {
	if ctx == nil {
		return nil, false
	}
	info, ok := ctx.Value(presignedRequestInfoKey).(*presignedRequestInfo)
	if !ok || info == nil {
		return nil, false
	}
	return info, true
}

func isPresignedRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	q := r.URL.Query()
	required := []string{
		"X-Amz-Algorithm",
		"X-Amz-Credential",
		"X-Amz-Signature",
		"X-Amz-Date",
		"X-Amz-Expires",
	}
	for _, key := range required {
		if strings.TrimSpace(q.Get(key)) == "" {
			return false
		}
	}
	return true
}

func isExpired(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return true
	}
	q := r.URL.Query()
	dateRaw := strings.TrimSpace(q.Get("X-Amz-Date"))
	expiresRaw := strings.TrimSpace(q.Get("X-Amz-Expires"))
	if dateRaw == "" || expiresRaw == "" {
		return true
	}
	t, err := time.Parse(sigV4TimeLayout, dateRaw)
	if err != nil {
		return true
	}
	expiresSeconds, err := strconv.Atoi(expiresRaw)
	if err != nil || expiresSeconds <= 0 || expiresSeconds > sigV4MaxPresignSeconds {
		return true
	}
	return time.Now().UTC().After(t.Add(time.Duration(expiresSeconds) * time.Second))
}

func PresignedOrIAMMiddleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPresignedRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		cfg := currentPresignedMiddlewareConfig()
		if cfg.resolveAccessKey == nil {
			writePresignedError(w, http.StatusForbidden, "presigned access is not configured")
			return
		}

		authInfo, err := parsePresignedAuth(r)
		if err != nil {
			logPresignedAttempt("rejected", r, "", "", "", err)
			writePresignedError(w, http.StatusForbidden, err.Error())
			return
		}

		ak, err := cfg.resolveAccessKey(authInfo.AccessKeyID)
		if err != nil {
			logPresignedAttempt("rejected", r, authInfo.AccessKeyID, authInfo.Bucket, authInfo.ObjectKey, err)
			writePresignedError(w, http.StatusForbidden, "invalid presigned access key")
			return
		}

		if err := access.ValidateAccessKeyForObjectRequest(ak, r.Method, authInfo.Bucket, authInfo.ObjectKey); err != nil {
			logPresignedAttempt("rejected", r, ak.AccessKeyID, authInfo.Bucket, authInfo.ObjectKey, err)
			writePresignedError(w, http.StatusForbidden, err.Error())
			return
		}

		if err := ValidatePresignedSignature(r, ak.SecretAccessKey); err != nil {
			logPresignedAttempt("rejected", r, ak.AccessKeyID, authInfo.Bucket, authInfo.ObjectKey, err)
			writePresignedError(w, http.StatusForbidden, err.Error())
			return
		}

		if isExpired(r) {
			logPresignedAttempt("rejected", r, ak.AccessKeyID, authInfo.Bucket, authInfo.ObjectKey, errExpiredPresignedURL)
			writePresignedError(w, http.StatusForbidden, errExpiredPresignedURL.Error())
			return
		}

		rateKey := strings.Join([]string{ak.AccessKeyID, strings.ToUpper(r.Method), authInfo.Bucket}, "|")
		now := time.Now().UTC()
		allowed, remaining, resetAt := cfg.limiter.allow(rateKey, now)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(cfg.limiter.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
		if !allowed {
			logPresignedAttempt("rate_limited", r, ak.AccessKeyID, authInfo.Bucket, authInfo.ObjectKey, errors.New("presigned object rate limit exceeded"))
			writePresignedError(w, http.StatusTooManyRequests, "presigned object rate limit exceeded")
			return
		}

		info := &presignedRequestInfo{
			AccessKeyID: ak.AccessKeyID,
			UserID:      ak.UserID,
			TenantID:    ak.TenantID,
			Bucket:      authInfo.Bucket,
			ObjectKey:   authInfo.ObjectKey,
			Method:      strings.ToUpper(r.Method),
		}
		ctx := context.WithValue(r.Context(), presignedRequestInfoKey, info)
		logPresignedAttempt("allowed", r, ak.AccessKeyID, authInfo.Bucket, authInfo.ObjectKey, nil)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ValidatePresignedSignature(r *http.Request, secretKey string) error {
	if strings.TrimSpace(secretKey) == "" {
		return fmt.Errorf("%w: missing secret key", errInvalidPresignedRequest)
	}
	authInfo, err := parsePresignedAuth(r)
	if err != nil {
		return err
	}

	canonicalHeaders, err := buildCanonicalHeaders(r, authInfo.SignedHeaders)
	if err != nil {
		return err
	}

	query := cloneQueryWithoutSignature(r.URL.Query())
	canonicalRequest := strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(r.Method)),
		awsURIEncode(authInfo.CanonicalPath, false),
		canonicalQuery(query),
		canonicalHeaders,
		strings.Join(authInfo.SignedHeaders, ";"),
		sigV4UnsignedPayload,
	}, "\n")

	scope := strings.Join([]string{authInfo.ScopeDate, authInfo.ScopeRegion, authInfo.ScopeService, authInfo.ScopeTerm}, "/")
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		authInfo.AmzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	scopeDateTime, err := time.Parse(sigV4DateLayout, authInfo.ScopeDate)
	if err != nil {
		return fmt.Errorf("%w: invalid credential scope date", errInvalidPresignedRequest)
	}
	signingKey := deriveSigningKey(secretKey, scopeDateTime, authInfo.ScopeRegion)
	expected := hex.EncodeToString(sumHMAC(signingKey, []byte(stringToSign)))
	if !hmac.Equal([]byte(strings.ToLower(authInfo.Signature)), []byte(strings.ToLower(expected))) {
		return fmt.Errorf("%w: signature mismatch", errInvalidPresignedRequest)
	}
	return nil
}

func GeneratePresignedURL(method, bucket, objectKey string, expiry time.Duration, accessKey, secretKey string) (string, error) {
	return GeneratePresignedURLWithHost(method, bucket, objectKey, expiry, accessKey, secretKey, "localhost")
}

func GeneratePresignedURLWithHost(method, bucket, objectKey string, expiry time.Duration, accessKey, secretKey, host string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != http.MethodGet && method != http.MethodPut {
		return "", fmt.Errorf("method must be GET or PUT")
	}
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return "", fmt.Errorf("access key and secret key are required")
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	expiresSeconds := int(expiry.Seconds())
	if expiresSeconds <= 0 || expiresSeconds > sigV4MaxPresignSeconds {
		return "", fmt.Errorf("expiry must be between 1 and %d seconds", sigV4MaxPresignSeconds)
	}

	path, err := buildObjectRequestPath(bucket, objectKey)
	if err != nil {
		return "", err
	}

	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}

	now := time.Now().UTC()
	scope := strings.Join([]string{now.Format(sigV4DateLayout), defaultPresignRegion, sigV4Service, sigV4Term}, "/")

	query := url.Values{}
	query.Set("X-Amz-Algorithm", sigV4Algorithm)
	query.Set("X-Amz-Credential", accessKey+"/"+scope)
	query.Set("X-Amz-Date", now.Format(sigV4TimeLayout))
	query.Set("X-Amz-Expires", strconv.Itoa(expiresSeconds))
	query.Set("X-Amz-SignedHeaders", "host")

	canonicalHeaders := "host:" + normalizeHeaderValue(host) + "\n"
	canonicalRequest := strings.Join([]string{
		method,
		awsURIEncode(path, false),
		canonicalQuery(query),
		canonicalHeaders,
		"host",
		sigV4UnsignedPayload,
	}, "\n")
	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		now.Format(sigV4TimeLayout),
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(secretKey, now, defaultPresignRegion)
	signature := hex.EncodeToString(sumHMAC(signingKey, []byte(stringToSign)))
	query.Set("X-Amz-Signature", signature)

	return path + "?" + canonicalQuery(query), nil
}

type parsedPresignedAuth struct {
	AccessKeyID   string
	Signature     string
	AmzDate       string
	Expires       int
	SignedHeaders []string
	ScopeDate     string
	ScopeRegion   string
	ScopeService  string
	ScopeTerm     string
	Bucket        string
	ObjectKey     string
	CanonicalPath string
}

func parsePresignedAuth(r *http.Request) (*parsedPresignedAuth, error) {
	if r == nil || r.URL == nil {
		return nil, fmt.Errorf("%w: nil request", errInvalidPresignedRequest)
	}
	if !isPresignedRequest(r) {
		return nil, fmt.Errorf("%w: required X-Amz query parameters are missing", errInvalidPresignedRequest)
	}

	q := r.URL.Query()
	if strings.TrimSpace(q.Get("X-Amz-Algorithm")) != sigV4Algorithm {
		return nil, fmt.Errorf("%w: unsupported X-Amz-Algorithm", errInvalidPresignedRequest)
	}

	credential := strings.TrimSpace(q.Get("X-Amz-Credential"))
	credentialParts := strings.Split(credential, "/")
	if len(credentialParts) != 5 {
		return nil, fmt.Errorf("%w: invalid X-Amz-Credential", errInvalidPresignedRequest)
	}

	expires, err := strconv.Atoi(strings.TrimSpace(q.Get("X-Amz-Expires")))
	if err != nil || expires <= 0 || expires > sigV4MaxPresignSeconds {
		return nil, fmt.Errorf("%w: invalid X-Amz-Expires", errInvalidPresignedRequest)
	}

	amzDate := strings.TrimSpace(q.Get("X-Amz-Date"))
	if _, err := time.Parse(sigV4TimeLayout, amzDate); err != nil {
		return nil, fmt.Errorf("%w: invalid X-Amz-Date", errInvalidPresignedRequest)
	}

	bucket, key, err := extractBucketAndKeyFromPath(r.URL.Path)
	if err != nil {
		return nil, err
	}
	canonicalPath, err := buildObjectRequestPath(bucket, key)
	if err != nil {
		return nil, err
	}

	signedHeaders, err := parseSignedHeaders(strings.TrimSpace(q.Get("X-Amz-SignedHeaders")))
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(strings.TrimSpace(r.Method), http.MethodGet) && !strings.EqualFold(strings.TrimSpace(r.Method), http.MethodPut) {
		return nil, fmt.Errorf("%w: presigned method %s is not allowed", errInvalidPresignedRequest, r.Method)
	}

	if credentialParts[3] != sigV4Service || credentialParts[4] != sigV4Term {
		return nil, fmt.Errorf("%w: invalid credential scope", errInvalidPresignedRequest)
	}

	return &parsedPresignedAuth{
		AccessKeyID:   strings.TrimSpace(credentialParts[0]),
		Signature:     strings.TrimSpace(q.Get("X-Amz-Signature")),
		AmzDate:       amzDate,
		Expires:       expires,
		SignedHeaders: signedHeaders,
		ScopeDate:     credentialParts[1],
		ScopeRegion:   credentialParts[2],
		ScopeService:  credentialParts[3],
		ScopeTerm:     credentialParts[4],
		Bucket:        bucket,
		ObjectKey:     key,
		CanonicalPath: canonicalPath,
	}, nil
}

func parseSignedHeaders(raw string) ([]string, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: missing X-Amz-SignedHeaders", errInvalidPresignedRequest)
	}
	parts := strings.Split(raw, ";")
	set := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		h := strings.ToLower(strings.TrimSpace(p))
		if h == "" {
			continue
		}
		if _, exists := set[h]; exists {
			continue
		}
		set[h] = struct{}{}
		result = append(result, h)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: empty signed headers", errInvalidPresignedRequest)
	}
	sorted := append([]string(nil), result...)
	sort.Strings(sorted)
	for i := range sorted {
		if sorted[i] != result[i] {
			return nil, fmt.Errorf("%w: X-Amz-SignedHeaders must be sorted", errInvalidPresignedRequest)
		}
	}
	return result, nil
}

func buildCanonicalHeaders(r *http.Request, signedHeaders []string) (string, error) {
	var b strings.Builder
	for _, h := range signedHeaders {
		name := strings.ToLower(strings.TrimSpace(h))
		if name == "" {
			return "", fmt.Errorf("%w: invalid signed header", errInvalidPresignedRequest)
		}

		var value string
		if name == "host" {
			value = strings.TrimSpace(r.Host)
			if value == "" {
				value = strings.TrimSpace(r.Header.Get("Host"))
			}
		} else {
			value = strings.TrimSpace(r.Header.Get(name))
		}
		if value == "" {
			return "", fmt.Errorf("%w: missing signed header %s", errInvalidPresignedRequest, name)
		}
		b.WriteString(name)
		b.WriteString(":")
		b.WriteString(normalizeHeaderValue(value))
		b.WriteString("\n")
	}
	return b.String(), nil
}

func normalizeHeaderValue(v string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(v)), " ")
}

func cloneQueryWithoutSignature(query url.Values) url.Values {
	cloned := make(url.Values, len(query))
	for k, vals := range query {
		if strings.EqualFold(k, "X-Amz-Signature") {
			continue
		}
		vv := append([]string(nil), vals...)
		cloned[k] = vv
	}
	return cloned
}

func canonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := append([]string(nil), query[k]...)
		sort.Strings(vals)
		if len(vals) == 0 {
			parts = append(parts, awsPercentEncode(k)+"=")
			continue
		}
		for _, v := range vals {
			parts = append(parts, awsPercentEncode(k)+"="+awsPercentEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

func awsPercentEncode(s string) string {
	var b strings.Builder
	for _, ch := range []byte(s) {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			b.WriteByte(ch)
			continue
		}
		b.WriteString("%")
		b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{ch})))
	}
	return b.String()
}

func awsURIEncode(path string, encodeSlash bool) string {
	if path == "" {
		return "/"
	}
	var b strings.Builder
	for _, ch := range []byte(path) {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			b.WriteByte(ch)
			continue
		}
		if ch == '/' && !encodeSlash {
			b.WriteByte('/')
			continue
		}
		b.WriteString("%")
		b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{ch})))
	}
	return b.String()
}

func sumHMAC(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func deriveSigningKey(secretKey string, t time.Time, region string) []byte {
	dateKey := sumHMAC([]byte("AWS4"+secretKey), []byte(t.UTC().Format(sigV4DateLayout)))
	regionKey := sumHMAC(dateKey, []byte(region))
	serviceKey := sumHMAC(regionKey, []byte(sigV4Service))
	return sumHMAC(serviceKey, []byte(sigV4Term))
}

func buildObjectRequestPath(bucket, objectKey string) (string, error) {
	bucket = strings.TrimSpace(bucket)
	objectKey = strings.TrimPrefix(strings.TrimSpace(objectKey), "/")
	if bucket == "" || objectKey == "" {
		return "", fmt.Errorf("%w: bucket and object key are required", errInvalidPresignedRequest)
	}
	if strings.Contains(bucket, "*") || strings.Contains(objectKey, "*") {
		return "", fmt.Errorf("%w: wildcard bucket/key is not allowed", errInvalidPresignedRequest)
	}
	for _, seg := range strings.Split(objectKey, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: object key path traversal is not allowed", errInvalidPresignedRequest)
		}
	}

	escapedBucket := url.PathEscape(bucket)
	parts := strings.Split(objectKey, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	escapedKey := strings.Join(parts, "/")
	return "/api/v1/storage/buckets/" + escapedBucket + "/objects/" + escapedKey, nil
}

func extractBucketAndKeyFromPath(path string) (string, string, error) {
	idx := strings.Index(path, "/storage/buckets/")
	if idx == -1 {
		return "", "", fmt.Errorf("%w: request path is not a storage object route", errInvalidPresignedRequest)
	}
	rest := path[idx+len("/storage/buckets/"):]
	parts := strings.SplitN(rest, "/objects/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: request path is not a storage object route", errInvalidPresignedRequest)
	}

	bucketRaw := strings.TrimPrefix(parts[0], "/")
	keyRaw := strings.TrimPrefix(parts[1], "/")
	if bucketRaw == "" || keyRaw == "" {
		return "", "", fmt.Errorf("%w: bucket and object key are required", errInvalidPresignedRequest)
	}

	bucket, err := url.PathUnescape(bucketRaw)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid bucket path", errInvalidPresignedRequest)
	}
	key, err := url.PathUnescape(keyRaw)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid object key path", errInvalidPresignedRequest)
	}

	bucket = strings.TrimSpace(bucket)
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if bucket == "" || key == "" {
		return "", "", fmt.Errorf("%w: bucket and object key are required", errInvalidPresignedRequest)
	}
	return bucket, key, nil
}

func currentPresignedMiddlewareConfig() presignedMiddlewareConfig {
	presignedMiddlewareState.mu.RLock()
	cfg := presignedMiddlewareState.cfg
	presignedMiddlewareState.mu.RUnlock()
	if cfg.limiter == nil {
		cfg.limiter = newFixedWindowLimiter(defaultPresignedRateLimitPerMins, time.Minute)
	}
	return cfg
}

func writePresignedError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(payload)
}

func logPresignedAttempt(status string, r *http.Request, accessKey, bucket, key string, err error) {
	ip := ""
	method := ""
	if r != nil {
		ip = clientIP(r)
		method = strings.ToUpper(r.Method)
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("storage.presigned status=%s ip=%s accessKey=%s method=%s bucket=%s key=%s ts=%s error=%q", status, ip, accessKey, method, bucket, key, ts, err.Error()))
		return
	}
	logging.Z().Info(fmt.Sprintf("storage.presigned status=%s ip=%s accessKey=%s method=%s bucket=%s key=%s ts=%s", status, ip, accessKey, method, bucket, key, ts))
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	xri := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if xri != "" {
		return xri
	}
	hostPort := strings.TrimSpace(r.RemoteAddr)
	if hostPort == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return host
	}
	return hostPort
}
