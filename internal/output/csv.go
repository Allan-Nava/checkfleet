package output

import (
	"encoding/csv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// CSV renders a run as CSV with a header row (status,check,target,message),
// worst-first (Result is pre-sorted). Fields are quoted/escaped by encoding/csv,
// so commas and newlines in messages are safe. For spreadsheets and ingestion.
func CSV(res engine.Result) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"status", "check", "target", "message"})
	for _, f := range res.Findings {
		if err := w.Write([]string{string(f.Status), f.Check, f.Target, f.Message}); err != nil {
			return "", err
		}
	}
	w.Flush()
	return b.String(), w.Error()
}
