package output

import (
	"encoding/json"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// maxSlackProblems caps how many problem lines we include; Slack rejects very
// large block lists, and a report should stay glanceable.
const maxSlackProblems = 20

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackBlock struct {
	Type string     `json:"type"`
	Text *slackText `json:"text,omitempty"`
}

type slackPayload struct {
	Blocks []slackBlock `json:"blocks"`
}

// Slack renders a run as a Slack Block Kit message (JSON) for an incoming
// webhook: a header, the summary line, then the non-OK findings (worst first,
// the Result is pre-sorted), capped to keep the message readable.
func Slack(res engine.Result, title string) (string, error) {
	blocks := []slackBlock{
		{Type: "header", Text: &slackText{Type: "plain_text", Text: "checkfleet — " + title}},
		{Type: "section", Text: &slackText{Type: "mrkdwn", Text: summaryLine(res)}},
	}

	var problems []engine.Finding
	for _, f := range res.Findings {
		if f.Status != engine.OK {
			problems = append(problems, f)
		}
	}
	if len(problems) == 0 {
		blocks = append(blocks, slackBlock{Type: "section", Text: &slackText{Type: "mrkdwn", Text: "All green. :white_check_mark:"}})
	}
	shown := problems
	if len(shown) > maxSlackProblems {
		shown = shown[:maxSlackProblems]
	}
	for _, f := range shown {
		line := fmt.Sprintf("%s *%s* `%s` %s — %s", statusIcon[f.Status], f.Status, f.Check, f.Target, f.Message)
		blocks = append(blocks, slackBlock{Type: "section", Text: &slackText{Type: "mrkdwn", Text: line}})
	}
	if extra := len(problems) - len(shown); extra > 0 {
		blocks = append(blocks, slackBlock{Type: "section", Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("…and %d more problems", extra)}})
	}

	out, err := json.MarshalIndent(slackPayload{Blocks: blocks}, "", "  ")
	return string(out), err
}
