// Sending one run to one sink — the dispatch behind --output.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
)

// sinkOptions is everything a sink needs beyond the run itself. It embeds
// renderCtx because a format renderer needs a strict subset: the push sinks are
// the ones that also need env var names and a template path.
type sinkOptions struct {
	renderCtx
	outFile    string
	webhookEnv string
	tgTokenEnv string
	tgChatEnv  string
	tmplFile   string
}

// emit sends the run to one sink: a push sink (slack/…) POSTs to its env URL, a
// format renderer writes to --out-file or stdout.
//
// Secrets are read here, from the environment, by the *name* the caller passed:
// a webhook URL or a bot token never travels as a flag value, where it would
// land in shell history and in `ps` output.
func emit(sink string, res engine.Result, o sinkOptions) error {
	switch sink {
	case "github":
		// Annotations go to stdout, where the Actions runner parses them.
		fmt.Print(output.GitHub(res))
		// The full report goes straight to the job summary file, appended so
		// it coexists with whatever other steps wrote there. Doing the write
		// here rather than telling users to pipe into $GITHUB_STEP_SUMMARY is
		// the point: a shell pipe swallows checkfleet's exit code unless
		// pipefail is set, which silently disables the CI gate.
		path := os.Getenv("GITHUB_STEP_SUMMARY")
		if path == "" {
			// Running outside Actions (local debugging): the annotations on
			// stdout are still useful, there is just nowhere to put a summary.
			return nil
		}
		return appendFile(path, output.GitHubSummary(res, o.module))
	case "slack":
		payload, err := output.Slack(res, o.module)
		if err != nil {
			return err
		}
		url := os.Getenv(o.webhookEnv)
		if url == "" {
			return fmt.Errorf("slack webhook not set: env %s is empty", o.webhookEnv)
		}
		if err := postJSON(context.Background(), url, payload); err != nil {
			return err
		}
		fmt.Println("checkfleet: report sent to Slack")
	case "discord":
		return postRendered(o.webhookEnv, "Discord", func() (string, error) { return output.Discord(res, o.module) })
	case "teams":
		return postRendered(o.webhookEnv, "Teams", func() (string, error) { return output.Teams(res, o.module) })
	case "telegram":
		text, err := output.Telegram(res, o.module)
		if err != nil {
			return err
		}
		token, chat := os.Getenv(o.tgTokenEnv), os.Getenv(o.tgChatEnv)
		if token == "" || chat == "" {
			return fmt.Errorf("telegram not set: env %s and/or %s are empty", o.tgTokenEnv, o.tgChatEnv)
		}
		if err := postTelegram(context.Background(), token, chat, text); err != nil {
			return err
		}
		fmt.Println("checkfleet: report sent to Telegram")
	case "webhook":
		var payload string
		var err error
		if o.tmplFile != "" {
			tmpl, rerr := os.ReadFile(o.tmplFile)
			if rerr != nil {
				return fmt.Errorf("reading template %s: %w", o.tmplFile, rerr)
			}
			payload, err = output.RenderTemplate(res, o.module, string(tmpl))
		} else {
			payload, err = output.JSON(res)
		}
		if err != nil {
			return err
		}
		url := os.Getenv(o.webhookEnv)
		if url == "" {
			return fmt.Errorf("webhook not set: env %s is empty", o.webhookEnv)
		}
		if err := postJSON(context.Background(), url, payload); err != nil {
			return err
		}
		fmt.Println("checkfleet: report sent to the webhook")
	default:
		rendered, err := render(sink, res, o.renderCtx)
		if err != nil {
			return err
		}
		if o.outFile != "" {
			return atomicWrite(o.outFile, rendered)
		}
		fmt.Print(rendered)
	}
	return nil
}

// emitAll sends the run to every sink.
//
// With one sink an error aborts the command (exit 1). With several, each sink is
// isolated: a down webhook or an unset env var is reported on stderr but does not
// stop the others or fail the run — the finding gate is a separate decision, and
// losing a Slack notification must not turn a healthy fleet into a red build.
func emitAll(sinks []string, res engine.Result, o sinkOptions) error {
	if len(sinks) == 1 {
		return emit(sinks[0], res, o)
	}
	for _, s := range sinks {
		if err := emit(s, res, o); err != nil {
			fmt.Fprintf(os.Stderr, "checkfleet: output %q: %v\n", s, err)
		}
	}
	return nil
}
