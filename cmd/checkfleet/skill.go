package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Allan-Nava/checkfleet/skills"
)

// runSkill installs or prints the agent skill embedded in the binary (CF-151).
// Whoever has checkfleet has the skill at the matching version, without cloning
// the repo — which matters because the skill's whole value is citing flags that
// actually exist in the binary you are running.
//
// Exit-code semantics: this is not a check. It returns 0 on success and only
// errors on a systemic failure (unwritable directory, unknown subcommand).
func runSkill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: checkfleet skill <install|print> [--global] [--dir PATH]")
	}
	switch args[0] {
	case "print":
		return printSkill(os.Stdout)
	case "install":
		return installSkill(args[1:])
	default:
		return fmt.Errorf("unknown skill subcommand %q (want install or print)", args[0])
	}
}

// printSkill writes SKILL.md to w, for piping somewhere the installer does not
// know about.
func printSkill(w *os.File) error {
	body, err := skills.FS.ReadFile(skills.Root + "/SKILL.md")
	if err != nil {
		return fmt.Errorf("read embedded skill: %w", err)
	}
	_, err = w.Write(body)
	return err
}

func installSkill(args []string) error {
	fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
	global := fs.Bool("global", false, "install into the user-level skills directory (default)")
	dir := fs.String("dir", "", "install into this directory instead")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dest, err := skillDest(*dir, *global)
	if err != nil {
		return err
	}
	written, err := writeSkill(dest)
	if err != nil {
		return err
	}
	fmt.Printf("installed %d file(s) to %s\n", written, dest)
	return nil
}

// skillDest resolves where to install. An explicit --dir wins; otherwise the
// user-level Claude Code skills directory, which is where a skill about a tool
// used *from other repos* belongs.
func skillDest(dir string, _ bool) (string, error) {
	if dir != "" {
		return filepath.Join(dir, skills.Root), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "skills", skills.Root), nil
}

// writeSkill copies the embedded tree to dest and returns the file count.
// Overwriting is intentional: re-running after an upgrade must replace the old
// version, and the operation is idempotent for the same binary.
func writeSkill(dest string) (int, error) {
	count := 0
	err := fs.WalkDir(skills.FS, skills.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(skills.Root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := skills.FS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}
