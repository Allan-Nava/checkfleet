// Package inventory parses Ansible INI inventories just enough to extract
// hosts and their ansible_host addresses, so checks can target a whole fleet
// from the same source of truth used by the playbooks.
package inventory

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Host is one inventory entry.
type Host struct {
	Name    string
	Address string // ansible_host when present, otherwise Name
	Group   string
}

var hostVarRe = regexp.MustCompile(`([\w.]+)=("[^"]*"|'[^']*'|\S+)`)

// LoadPath parses a single INI inventory file or every file in a directory.
func LoadPath(path string) ([]Host, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return parseFile(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var hosts []Host
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		parsed, err := parseFile(filepath.Join(path, e.Name()))
		if err != nil {
			continue // non-inventory files inside the dir are skipped
		}
		hosts = append(hosts, parsed...)
	}
	return dedupe(hosts), nil
}

func parseFile(path string) ([]Host, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hosts []Host
	group := "ungrouped"
	skipSection := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := stripComment(scanner.Text())
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.Trim(line, "[]")
			// [group:vars] and [group:children] sections contain no host lines.
			skipSection = strings.Contains(section, ":")
			if !skipSection {
				group = section
			}
			continue
		}
		if skipSection {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		host := Host{Name: fields[0], Address: fields[0], Group: group}
		for _, m := range hostVarRe.FindAllStringSubmatch(line[len(fields[0]):], -1) {
			if m[1] == "ansible_host" {
				host.Address = strings.Trim(m[2], `"'`)
			}
		}
		hosts = append(hosts, host)
	}
	return dedupe(hosts), scanner.Err()
}

func stripComment(line string) string {
	inS, inD := false, false
	for i, c := range line {
		switch {
		case c == '\'' && !inD:
			inS = !inS
		case c == '"' && !inS:
			inD = !inD
		case (c == '#' || c == ';') && !inS && !inD:
			if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
				return line[:i]
			}
		}
	}
	return line
}

func dedupe(hosts []Host) []Host {
	seen := map[string]bool{}
	var out []Host
	for _, h := range hosts {
		if seen[h.Name] {
			continue
		}
		seen[h.Name] = true
		out = append(out, h)
	}
	return out
}
