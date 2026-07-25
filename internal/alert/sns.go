package alert

import (
	"net/url"
	"strings"
)

// SNSForm builds the x-www-form-urlencoded body for an SNS Publish call.
func SNSForm(topicArn, subject, message string) string {
	v := url.Values{}
	v.Set("Action", "Publish")
	v.Set("Version", "2010-03-31")
	v.Set("TopicArn", topicArn)
	v.Set("Message", message)
	if subject != "" {
		// SNS subjects must be ASCII, <=100 chars, no newlines.
		v.Set("Subject", truncateSubject(subject))
	}
	return v.Encode()
}

// RegionFromTopicARN extracts the region from arn:aws:sns:<region>:<acct>:<topic>.
func RegionFromTopicARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

func truncateSubject(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 100 {
		return s[:100]
	}
	return s
}
