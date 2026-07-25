// Package awssig implements AWS Signature Version 4 request signing by hand
// (crypto/hmac + sha256), with no AWS SDK. It signs an *http.Request with a
// known body for any service/region — used by the SNS alert sink (and usable by
// other AWS-compatible callers).
package awssig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Sign adds the SigV4 Authorization and X-Amz-Date headers to req for the given
// body, service and region. Content-Type, when set, is included in the signature.
func Sign(req *http.Request, body []byte, accessKey, secretKey, region, service string, now time.Time) {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256hex(body)

	host := req.URL.Host
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", amzDate)

	// Headers to sign, always host + x-amz-date, plus content-type if present.
	headers := map[string]string{
		"host":         host,
		"x-amz-date":   amzDate,
	}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		headers["content-type"] = ct
	}
	names := make([]string, 0, len(headers))
	for n := range headers {
		names = append(names, n)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, n := range names {
		canonHeaders.WriteString(n + ":" + headers[n] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalizeQuery(req.URL.RawQuery)

	canonReq := req.Method + "\n" + canonicalURI + "\n" + canonicalQuery + "\n" +
		canonHeaders.String() + "\n" + signedHeaders + "\n" + payloadHash

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256hex([]byte(canonReq))

	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
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
