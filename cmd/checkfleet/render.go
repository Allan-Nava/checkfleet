// Rendering a run into an output format, and writing it somewhere safely.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
)

// render turns a run into the printable output for a format (not slack).
// renderCtx is the run context a renderer may need beyond the Result itself.
type renderCtx struct {
	module     string
	color      bool
	configPath string // SARIF anchors its results to this file
}

func render(format string, res engine.Result, ctx renderCtx) (string, error) {
	module, color := ctx.module, ctx.color
	switch format {
	case "sarif":
		s, err := output.SARIF(res, output.SARIFOptions{Version: version, ConfigPath: ctx.configPath})
		return s + "\n", err
	case "text":
		if color {
			return output.TextColor(res), nil
		}
		return output.Text(res), nil
	case "markdown":
		return output.Markdown(res, module), nil
	case "json":
		s, err := output.JSON(res)
		return s + "\n", err
	case "junit":
		s, err := output.JUnit(res, module)
		return s + "\n", err
	case "html":
		return output.HTML(res, module), nil
	case "prometheus":
		return output.Prometheus(res), nil
	case "otlp":
		s, err := output.OTLP(res)
		return s + "\n", err
	case "csv":
		return output.CSV(res)
	default:
		return "", fmt.Errorf("unknown format %q", format)
	}
}

// isTerminal reports whether f is a character device (a terminal), so we only
// emit ANSI colour when a human is watching — never into a pipe or file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// appendFile appends content to path, creating it if needed. Used for
// $GITHUB_STEP_SUMMARY, which is a shared, append-only scratch file: other
// steps of the same job write to it too, so atomicWrite's rename would throw
// their contributions away.
func appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// atomicWrite writes content to path via a temp file + rename, so a reader
// (e.g. the node_exporter textfile collector) never sees a partial file.
func atomicWrite(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".checkfleet-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
