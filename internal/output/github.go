package output

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// GitHub renders the findings as GitHub Actions **workflow commands**, so they
// appear as inline annotations on the run and on any PR that triggered it.
//
// Only findings that need attention are emitted. OK findings are deliberately
// silent: annotations are an attention mechanism, and GitHub shows at most 10
// per level per step — spending that budget on green targets would push the
// real problems out of the UI. The full picture belongs in the job summary
// (see GitHubSummary) or in any other sink you fan out to.
//
// Severity maps to the two levels a reader acts on differently:
//
//	BAD, ERROR → ::error
//	WARN       → ::warning
//
// ERROR is not given its own level because Actions has none; the annotation
// title keeps the distinction visible ("ERROR" means the check could not
// measure, not that the target is unhealthy).
func GitHub(res engine.Result) string {
	var b strings.Builder
	for _, f := range res.Findings {
		level := ""
		switch f.Status {
		case engine.BAD, engine.ERROR:
			level = "error"
		case engine.WARN:
			level = "warning"
		default:
			continue
		}
		// title is a property (stricter escaping); the message is everything
		// after the "::".
		title := fmt.Sprintf("checkfleet %s: %s [%s]", f.Check, f.Target, f.Status)
		fmt.Fprintf(&b, "::%s title=%s::%s\n", level, escapeProperty(title), escapeData(f.Message))
	}
	return b.String()
}

// escapeData escapes a workflow command's *message*. The percent sign goes
// first: escaping it after the others would corrupt the escapes they inserted.
func escapeData(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	).Replace(s)
}

// escapeProperty escapes a workflow command's *property value*, which needs
// the message escapes plus ":" and "," — both are command syntax. This matters
// in practice: nearly every checkfleet target contains a colon (a URL scheme,
// a host:port), and an unescaped one truncates the annotation.
func escapeProperty(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	).Replace(s)
}

// GitHubSummary renders the Markdown report that belongs in the job summary
// ($GITHUB_STEP_SUMMARY). It is the same ops report as the markdown sink, so
// annotations and summary never disagree about a run.
func GitHubSummary(res engine.Result, title string) string {
	return Markdown(res, title)
}
