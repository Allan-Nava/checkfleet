package output

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Telegram renders a run as a plain-text message for the Telegram Bot API
// (sendMessage). Plain text avoids MarkdownV2 escaping pitfalls; it stays within
// the 4096-char limit via the shared problem cap. The caller posts it with the
// bot token and chat id.
func Telegram(res engine.Result, title string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "checkfleet — %s\n%s\n", title, summaryLine(res))

	shown, extra := chatProblems(res)
	if len(shown) == 0 {
		b.WriteString("\nAll green ✅")
		return b.String(), nil
	}
	b.WriteString("\nNeeds attention:\n")
	for _, f := range shown {
		fmt.Fprintf(&b, "%s %s %s — %s\n", statusIcon[f.Status], f.Status, f.Target, f.Message)
	}
	if extra > 0 {
		fmt.Fprintf(&b, "…and %d more", extra)
	}
	return b.String(), nil
}
