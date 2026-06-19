package profile

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type S3Storage struct {
	endpoint        string
	presignEndpoint string
	publicBaseURL   string
	accessKey       string
	secretKey       string
	bucketName      string
	region          string
	useSSL          bool
	presignUseSSL   bool
	httpClient      *http.Client
}

type S3StorageConfig struct {
	Endpoint      string
	PublicBaseURL string
	AccessKey     string
	SecretKey     string
	BucketName    string
	Region        string
	UseSSL        bool
}

func NewS3Storage(cfg S3StorageConfig) *S3Storage {
	presignEndpoint, presignUseSSL := publicEndpoint(cfg.PublicBaseURL, cfg.Endpoint, cfg.UseSSL)

	return &S3Storage{
		endpoint:        strings.TrimRight(cfg.Endpoint, "/"),
		presignEndpoint: presignEndpoint,
		publicBaseURL:   strings.TrimRight(cfg.PublicBaseURL, "/"),
		accessKey:       cfg.AccessKey,
		secretKey:       cfg.SecretKey,
		bucketName:      cfg.BucketName,
		region:          cfg.Region,
		useSSL:          cfg.UseSSL,
		presignUseSSL:   presignUseSSL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (s *S3Storage) PublicURL(objectKey string) string {
	return s.publicBaseURL + "/" + escapeObjectKey(objectKey)
}

func (s *S3Storage) PresignPutObject(objectKey string, contentType string, expiresInSeconds int64) (string, error) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	host := s.presignEndpoint
	canonicalURI := "/" + s.bucketName + "/" + escapeObjectKey(objectKey)
	signedHeaders := "content-type;host"

	values := url.Values{}
	values.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	values.Set("X-Amz-Credential", s.accessKey+"/"+credentialScope)
	values.Set("X-Amz-Date", amzDate)
	values.Set("X-Amz-Expires", fmt.Sprintf("%d", expiresInSeconds))
	values.Set("X-Amz-SignedHeaders", signedHeaders)

	canonicalQuery := canonicalQueryString(values)
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\n", strings.ToLower(contentType), host)
	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(hmacSHA256(s.signingKey(dateStamp), []byte(stringToSign)))
	values.Set("X-Amz-Signature", signature)

	scheme := "http"
	if s.presignUseSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s?%s", scheme, host, canonicalURI, values.Encode()), nil
}

func (s *S3Storage) HeadObject(ctx context.Context, objectKey string) (*ObjectInfo, error) {
	scheme := "http"
	if s.useSSL {
		scheme = "https"
	}

	requestURL := fmt.Sprintf("%s://%s/%s/%s", scheme, s.endpoint, s.bucketName, escapeObjectKey(objectKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, requestURL, nil)
	if err != nil {
		return nil, err
	}

	s.signRequest(req, nil)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("head object retornou status %d", resp.StatusCode)
	}

	return &ObjectInfo{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
	}, nil
}

func (s *S3Storage) signRequest(req *http.Request, body []byte) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := hexSHA256(body)
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalURI := req.URL.EscapedPath()
	canonicalQuery := canonicalQueryString(req.URL.Query())
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf(
		"host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		host,
		payloadHash,
		amzDate,
	)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(s.signingKey(dateStamp), []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey,
		credentialScope,
		signedHeaders,
		signature,
	))
}

func (s *S3Storage) signingKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		for _, value := range vals {
			parts = append(parts, awsEscape(key)+"="+awsEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func escapeObjectKey(objectKey string) string {
	clean := path.Clean("/" + objectKey)
	clean = strings.TrimPrefix(clean, "/")
	segments := strings.Split(clean, "/")
	for i, segment := range segments {
		segments[i] = awsEscape(segment)
	}
	return strings.Join(segments, "/")
}

func awsEscape(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func hexSHA256(payload []byte) string {
	if payload == nil {
		payload = []byte{}
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func publicEndpoint(publicBaseURL string, fallbackEndpoint string, fallbackUseSSL bool) (string, bool) {
	parsed, err := url.Parse(strings.TrimRight(publicBaseURL, "/"))
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(fallbackEndpoint, "/"), fallbackUseSSL
	}
	return parsed.Host, parsed.Scheme == "https"
}
