// Desktop app (Wails) — a SEPARATE Go module on purpose: the Wails/web
// toolchain and its large dependency tree stay out of the CLI module, so the
// CLI's `go build/test ./...` and CI never pull them in (CF-15). It reuses the
// core logic via the local replace directive — no forked check logic.
module github.com/Allan-Nava/checkfleet/desktop

go 1.25.0

require (
	github.com/Allan-Nava/checkfleet v0.0.0
	github.com/wailsapp/wails/v2 v2.10.2
)

// Reuse internal/engine, internal/output and internal/registry from the repo
// root. `go mod tidy` inside desktop/ resolves the rest of the Wails deps.
replace github.com/Allan-Nava/checkfleet => ../
