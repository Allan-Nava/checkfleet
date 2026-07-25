// Package s3 checks an S3/object-storage bucket: it's reachable, and an optional
// sentinel object exists and is fresh. Requests are signed with AWS Signature
// V4 written by hand (zero dependencies, no AWS SDK). Credentials come from env;
// with none set it falls back to anonymous requests (public buckets).
package s3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg    engine.S3Config
	now    func() time.Time
	client *http.Client
}

func New(cfg engine.S3Config) *Check {
	return &Check{cfg: cfg, now: time.Now, client: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Check) Name() string { return "s3" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t)...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.S3Target) []engine.Finding {
	label := t.Name
	if label == "" {
		label = t.Bucket
	}

	// Bucket reachability.
	bf := engine.Finding{Check: c.Name(), Target: label}
	code, _, err := c.do(ctx, http.MethodHead, t, "")
	switch {
	case err != nil:
		bf.Status, bf.Message = engine.ERROR, fmt.Sprintf("bucket unreachable: %v", err)
	case code == http.StatusOK:
		bf.Status, bf.Message = engine.OK, "bucket reachable"
	case code == http.StatusNotFound:
		bf.Status, bf.Message = engine.BAD, "bucket not found (HTTP 404)"
	case code == http.StatusForbidden:
		bf.Status, bf.Message = engine.BAD, "access denied (HTTP 403) — check credentials"
	default:
		bf.Status, bf.Message = engine.BAD, fmt.Sprintf("unexpected HTTP %d", code)
	}
	findings := []engine.Finding{bf}

	// Skip the object check if the bucket isn't reachable — it would be redundant.
	if bf.Status != engine.OK || t.Object == "" {
		return findings
	}

	// Sentinel object: present + fresh.
	of := engine.Finding{Check: c.Name(), Target: label + "/" + t.Object}
	code, lastMod, err := c.do(ctx, http.MethodHead, t, t.Object)
	switch {
	case err != nil:
		of.Status, of.Message = engine.ERROR, fmt.Sprintf("object HEAD failed: %v", err)
	case code == http.StatusNotFound:
		of.Status, of.Message = engine.BAD, "sentinel object missing (HTTP 404)"
	case code != http.StatusOK:
		of.Status, of.Message = engine.BAD, fmt.Sprintf("object HTTP %d", code)
	default:
		age := c.now().UTC().Sub(lastMod)
		msg := fmt.Sprintf("present, modified %s ago", age.Round(time.Second))
		if t.MaxAgeWarnSeconds > 0 && age > time.Duration(t.MaxAgeWarnSeconds)*time.Second {
			of.Status, of.Message = engine.WARN, fmt.Sprintf("stale: %s (over %ds)", msg, t.MaxAgeWarnSeconds)
		} else {
			of.Status, of.Message = engine.OK, msg
		}
	}
	return append(findings, of)
}

// do issues a signed (or anonymous) request for the bucket (object == "") or an
// object, returning the status code and the parsed Last-Modified time.
func (c *Check) do(ctx context.Context, method string, t engine.S3Target, object string) (int, time.Time, error) {
	url := strings.TrimRight(t.Endpoint, "/")
	host := ""
	if t.PathStyle {
		url += "/" + t.Bucket
	} else {
		// virtual-hosted: bucket.<host>
		if i := strings.Index(url, "://"); i >= 0 {
			scheme, rest := url[:i+3], url[i+3:]
			url = scheme + t.Bucket + "." + rest
		}
	}
	if object != "" {
		url += "/" + object
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, time.Time{}, err
	}
	if host != "" {
		req.Host = host
	}

	ak, sk := os.Getenv(t.AccessKeyEnv), os.Getenv(t.SecretKeyEnv)
	if ak != "" && sk != "" {
		region := t.Region
		if region == "" {
			region = "us-east-1"
		}
		c.sign(req, ak, sk, region)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()
	var lastMod time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		lastMod, _ = http.ParseTime(lm)
	}
	return resp.StatusCode, lastMod, nil
}

// sign adds an AWS Signature V4 Authorization header for the S3 service.
func (c *Check) sign(req *http.Request, accessKey, secretKey, region string) {
	now := c.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256hex(nil)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalizeQuery(req.URL.RawQuery)

	canonReq := req.Method + "\n" + canonicalURI + "\n" + canonicalQuery + "\n" +
		canonHeaders + "\n" + signedHeaders + "\n" + payloadHash

	scope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256hex([]byte(canonReq))

	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func canonicalizeQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	sort.Strings(parts)
	return strings.Join(parts, "&")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
