package alert

import (
	"net/url"
	"strings"
	"testing"
)

func TestSNSForm(t *testing.T) {
	body := SNSForm("arn:aws:sns:eu-west-1:123:alerts", "certs/api", "cert expired")
	v, err := url.ParseQuery(body)
	if err != nil {
		t.Fatal(err)
	}
	if v.Get("Action") != "Publish" || v.Get("TopicArn") != "arn:aws:sns:eu-west-1:123:alerts" ||
		v.Get("Message") != "cert expired" || v.Get("Subject") != "certs/api" {
		t.Errorf("unexpected form: %v", v)
	}
}

func TestSNSFormSubjectSanitised(t *testing.T) {
	long := strings.Repeat("x", 150) + "\nnewline"
	v, _ := url.ParseQuery(SNSForm("arn", "", "m")) // empty subject → omitted
	if _, ok := v["Subject"]; ok {
		t.Error("empty subject should be omitted")
	}
	v2, _ := url.ParseQuery(SNSForm("arn", long, "m"))
	s := v2.Get("Subject")
	if len(s) > 100 || strings.Contains(s, "\n") {
		t.Errorf("subject not sanitised: len=%d %q", len(s), s)
	}
}

func TestRegionFromTopicARN(t *testing.T) {
	if got := RegionFromTopicARN("arn:aws:sns:us-east-2:999:topic"); got != "us-east-2" {
		t.Errorf("region = %q, want us-east-2", got)
	}
	if got := RegionFromTopicARN("garbage"); got != "" {
		t.Errorf("bad ARN should give empty region, got %q", got)
	}
}
