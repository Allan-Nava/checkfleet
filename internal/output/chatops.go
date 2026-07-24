package output

import (
	"encoding/json"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// maxChatProblems caps how many problem entries chat messages include, so an
// embed/card stays readable and within provider field limits.
const maxChatProblems = 15

// chatProblems returns the non-OK findings (worst first, already sorted),
// capped, plus how many were dropped.
func chatProblems(res engine.Result) (shown []engine.Finding, extra int) {
	var problems []engine.Finding
	for _, f := range res.Findings {
		if f.Status != engine.OK {
			problems = append(problems, f)
		}
	}
	shown = problems
	if len(shown) > maxChatProblems {
		shown = shown[:maxChatProblems]
	}
	return shown, len(problems) - len(shown)
}

// worstDecimalColor maps the worst status to a Discord embed color (decimal).
func worstDecimalColor(res engine.Result) int {
	switch engine.Worst(res.Findings) {
	case engine.WARN:
		return 0xF1C40F
	case engine.BAD:
		return 0xE74C3C
	case engine.ERROR:
		return 0x9B59B6
	default:
		return 0x2ECC71
	}
}

// Discord renders a run as a Discord webhook payload (JSON): one embed with the
// summary as description and a field per non-OK finding (worst first, capped).
func Discord(res engine.Result, title string) (string, error) {
	type field struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Inline bool   `json:"inline"`
	}
	type embed struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Color       int     `json:"color"`
		Fields      []field `json:"fields,omitempty"`
	}
	type payload struct {
		Content string  `json:"content"`
		Embeds  []embed `json:"embeds"`
	}

	e := embed{
		Title:       "checkfleet — " + title,
		Description: summaryLine(res),
		Color:       worstDecimalColor(res),
	}
	shown, extra := chatProblems(res)
	if len(shown) == 0 {
		e.Description += "\nAll green. ✅"
	}
	for _, f := range shown {
		e.Fields = append(e.Fields, field{
			Name:  fmt.Sprintf("%s %s · %s", statusIcon[f.Status], f.Status, f.Check),
			Value: fmt.Sprintf("`%s` — %s", f.Target, f.Message),
		})
	}
	if extra > 0 {
		e.Fields = append(e.Fields, field{Name: "…", Value: fmt.Sprintf("and %d more problems", extra)})
	}

	out, err := json.MarshalIndent(payload{Embeds: []embed{e}}, "", "  ")
	return string(out), err
}

// Teams renders a run as a Microsoft Teams MessageCard (JSON) for an Office 365
// connector / incoming webhook: a themed card with the summary and a facts list
// of the non-OK findings (worst first, capped).
func Teams(res engine.Result, title string) (string, error) {
	type fact struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	type section struct {
		Text  string `json:"text,omitempty"`
		Facts []fact `json:"facts,omitempty"`
	}
	type card struct {
		Type       string    `json:"@type"`
		Context    string    `json:"@context"`
		ThemeColor string    `json:"themeColor"`
		Summary    string    `json:"summary"`
		Title      string    `json:"title"`
		Text       string    `json:"text"`
		Sections   []section `json:"sections,omitempty"`
	}

	c := card{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: fmt.Sprintf("%06X", worstDecimalColor(res)),
		Summary:    "checkfleet — " + title,
		Title:      "checkfleet — " + title,
		Text:       summaryLine(res),
	}
	shown, extra := chatProblems(res)
	if len(shown) == 0 {
		c.Text += "  \nAll green. ✅"
	}
	var sec section
	for _, f := range shown {
		sec.Facts = append(sec.Facts, fact{
			Name:  fmt.Sprintf("%s %s · %s", statusIcon[f.Status], f.Status, f.Check),
			Value: fmt.Sprintf("`%s` — %s", f.Target, f.Message),
		})
	}
	if extra > 0 {
		sec.Facts = append(sec.Facts, fact{Name: "…", Value: fmt.Sprintf("and %d more problems", extra)})
	}
	if len(sec.Facts) > 0 {
		c.Sections = []section{sec}
	}

	out, err := json.MarshalIndent(c, "", "  ")
	return string(out), err
}
