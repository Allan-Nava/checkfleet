package output

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
)

// SARIF 2.1.0 — the interchange format for static analysis results. Emitting it
// puts checkfleet findings in GitHub's Code scanning / Security tab, and in any
// other SARIF-aware tool, with no extra glue.
//
// The impedance mismatch worth knowing about: SARIF is file-oriented (a result
// points at a line of source), while a checkfleet finding is about a *network
// target*. There is no file to blame, so every result is anchored to the config
// file that declares the fleet — see sarifLocation. The real subject lives in
// the message, in properties.target, and in the fingerprint.

const sarifSchema = "https://json.schemastore.org/sarif-2.1.0.json"

// SARIFOptions carries the run context that a Result does not hold.
type SARIFOptions struct {
	Version    string // checkfleet version → tool.driver.version
	ConfigPath string // anchor for result locations; repo-relative is best
}

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	ShortDescription sarifText `json:"shortDescription"`
	FullDescription  sarifText `json:"fullDescription"`
	HelpURI          string    `json:"helpUri,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          map[string]any    `json:"properties,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// sarifLevel maps a finding status to one of SARIF's four levels.
//
// BAD and ERROR both become "error" because SARIF has no third failure level;
// the distinction (unhealthy target vs. a check that could not measure) stays
// readable in the message and in properties.status.
//
// OK becomes "none" rather than being dropped: SARIF is a full record of a run,
// and "none" is exactly the level for "examined, nothing to report". GitHub
// does not raise alerts for it, so including them costs nothing in the UI.
func sarifLevel(s engine.Status) string {
	switch s {
	case engine.BAD, engine.ERROR:
		return "error"
	case engine.WARN:
		return "warning"
	default:
		return "none"
	}
}

// SARIF renders the run as a SARIF 2.1.0 log.
func SARIF(res engine.Result, opts SARIFOptions) (string, error) {
	// One rule per module that actually produced findings, in a stable order so
	// the document is byte-reproducible for the same run.
	var names []string
	seen := map[string]bool{}
	for _, f := range res.Findings {
		if !seen[f.Check] {
			seen[f.Check] = true
			names = append(names, f.Check)
		}
	}
	sort.Strings(names)

	rules := make([]sarifRule, 0, len(names))
	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
		doc, ok := moduledoc.Doc(name)
		if !ok {
			doc = "checkfleet module " + name
		}
		rules = append(rules, sarifRule{
			ID:               "checkfleet/" + name,
			Name:             name,
			ShortDescription: sarifText{Text: firstSentenceOf(doc)},
			FullDescription:  sarifText{Text: doc},
			HelpURI:          "https://allan-nava.github.io/checkfleet/modules.html#" + name,
		})
	}

	results := make([]sarifResult, 0, len(res.Findings))
	for _, f := range res.Findings {
		props := map[string]any{
			"target": f.Target,
			"check":  f.Check,
			// The engine's own status, which is finer-grained than the SARIF
			// level: BAD and ERROR both map to "error" but mean different things.
			"status": string(f.Status),
		}
		if f.Value != nil {
			props["value"] = *f.Value
		}
		if f.Unit != "" {
			props["unit"] = f.Unit
		}
		for k, v := range res.Labels {
			props["label."+k] = v
		}
		results = append(results, sarifResult{
			RuleID:    "checkfleet/" + f.Check,
			RuleIndex: index[f.Check],
			Level:     sarifLevel(f.Status),
			// The target has to be in the message: it is the only place a SARIF
			// reader shows, and the location points at the config file, not at
			// the thing that is broken.
			Message:             sarifText{Text: f.Target + ": " + f.Message},
			Locations:           []sarifLocation{sarifLocationFor(opts.ConfigPath)},
			PartialFingerprints: map[string]string{"checkfleetTarget/v1": fingerprint(f)},
			Properties:          props,
		})
	}

	log := sarifLog{
		Schema:  sarifSchema,
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "checkfleet",
				Version:        opts.Version,
				InformationURI: "https://github.com/Allan-Nava/checkfleet",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	b, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// sarifLocationFor anchors a result to the config file. GitHub only surfaces a
// result that has a location, and the config is the one file in the repo that
// is genuinely responsible for the target being checked at all. Line 1 because
// we do not track where in the YAML a target was declared.
func sarifLocationFor(configPath string) sarifLocation {
	uri := configPath
	if uri == "" {
		uri = "checkfleet.yml"
	}
	// SARIF URIs are slash-separated regardless of the host OS.
	uri = filepath.ToSlash(uri)
	return sarifLocation{PhysicalLocation: sarifPhysicalLocation{
		ArtifactLocation: sarifArtifactLocation{URI: uri},
		Region:           sarifRegion{StartLine: 1},
	}}
}

// fingerprint identifies *what* a result is about, so a SARIF consumer can
// track one problem across runs. Deliberately built from check+target only:
// including the status would make a target that goes WARN → BAD look like a
// brand-new alert instead of the same one getting worse.
func fingerprint(f engine.Finding) string {
	sum := sha256.Sum256([]byte(f.Check + "\x00" + f.Target))
	return hex.EncodeToString(sum[:])
}

func firstSentenceOf(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i+1]
	}
	return s
}
