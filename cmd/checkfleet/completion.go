package main

import (
	"fmt"
	"strings"
)

// completionScript returns a shell completion script for bash, zsh or fish.
// Module and output lists are injected so completion stays in sync.
func completionScript(shell string) (string, error) {
	modules := "all " + strings.Join(moduleNames(), " ")
	const subcmds = "check serve validate report-issues explain completion version"
	const outputs = "text markdown json junit html prometheus slack discord teams webhook"
	const flags = "--config --stack --output --out-file --webhook-env --only --min-severity --target --history --flap-changes --flap-window --ping-url-env --exit-on-bad"

	switch shell {
	case "bash":
		return fmt.Sprintf(`# checkfleet bash completion — source this, or drop it in /etc/bash_completion.d/
_checkfleet() {
  local cur prev; cur="${COMP_WORDS[COMP_CWORD]}"; prev="${COMP_WORDS[COMP_CWORD-1]}"
  if [ "$COMP_CWORD" -eq 1 ]; then COMPREPLY=( $(compgen -W %q -- "$cur") ); return; fi
  case "$prev" in
    check|explain) COMPREPLY=( $(compgen -W %q -- "$cur") ); return;;
    --output) COMPREPLY=( $(compgen -W %q -- "$cur") ); return;;
    completion) COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") ); return;;
  esac
  if [[ "$cur" == -* ]]; then COMPREPLY=( $(compgen -W %q -- "$cur") ); fi
}
complete -F _checkfleet checkfleet
`, subcmds, modules, outputs, flags), nil

	case "zsh":
		return fmt.Sprintf(`#compdef checkfleet
# checkfleet zsh completion — put on your $fpath as _checkfleet, or source it.
_checkfleet() {
  local -a subcmds modules outputs
  subcmds=(%s)
  modules=(%s)
  outputs=(%s)
  if (( CURRENT == 2 )); then compadd -- $subcmds; return; fi
  case ${words[2]} in
    check|explain) if (( CURRENT == 3 )); then compadd -- $modules; return; fi;;
    completion) compadd -- bash zsh fish; return;;
  esac
  case ${words[CURRENT-1]} in
    --output) compadd -- $outputs; return;;
  esac
  compadd -- %s
}
compdef _checkfleet checkfleet
`, subcmds, modules, outputs, flags), nil

	case "fish":
		var b strings.Builder
		b.WriteString("# checkfleet fish completion — source this or drop in ~/.config/fish/completions/checkfleet.fish\n")
		b.WriteString("complete -c checkfleet -f\n")
		fmt.Fprintf(&b, "complete -c checkfleet -n __fish_use_subcommand -a %q\n", subcmds)
		fmt.Fprintf(&b, "complete -c checkfleet -n '__fish_seen_subcommand_from check explain' -a %q\n", modules)
		fmt.Fprintf(&b, "complete -c checkfleet -l output -a %q\n", outputs)
		b.WriteString("complete -c checkfleet -l config -r\n")
		b.WriteString("complete -c checkfleet -l out-file -r\n")
		b.WriteString("complete -c checkfleet -l stack -l only -l min-severity -l target -l webhook-env -l history\n")
		b.WriteString("complete -c checkfleet -l exit-on-bad\n")
		return b.String(), nil

	default:
		return "", fmt.Errorf("unsupported shell %q (use bash, zsh or fish)", shell)
	}
}

// runCompletion prints the completion script for the requested shell.
//
//	checkfleet completion <bash|zsh|fish>
func runCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: checkfleet completion <bash|zsh|fish>")
	}
	script, err := completionScript(args[0])
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
}
