# Security policy

## Reporting a vulnerability

**Use [GitHub's private vulnerability reporting](https://github.com/Allan-Nava/checkfleet/security/advisories/new)** — the *Report a vulnerability* button in the repository's Security tab. The report stays private until a fix is available.

Please do **not** open a public issue for a vulnerability.

What helps, in rough order of usefulness:

- the version (`checkfleet version`) and how it was installed
- a minimal `checkfleet.yml` and command that reproduces it — **with secrets removed**
- what an attacker gets out of it

**Redact before you send.** A checkfleet config points at real infrastructure and its `${VAR}` references name real secrets. Replace hostnames, DSNs, tokens and webhook URLs with placeholders: a vulnerability report should not be the thing that leaks your fleet.

## What to expect

checkfleet is maintained by one person, so this is a best-effort commitment rather than an SLA:

| Stage | Target |
|---|---|
| Acknowledgement | within 5 working days |
| Assessment and severity | within 10 working days |
| Fix for a confirmed high-severity issue | in the next release, as a patch on the current minor |

Reports are credited in the release notes and in the advisory unless you ask otherwise. There is no bug bounty.

## Supported versions

Only the **latest release** receives security fixes. Backports to older tags are not provided — checkfleet is a single static binary, so upgrading is replacing one file.

## Scope

In scope — a defect in checkfleet itself:

- **credential leakage**: a secret from the environment, a config `${VAR}`, or a DSN appearing in output, logs, a rendered report, an issue body, a webhook payload or an error message. checkfleet handles database DSNs, API tokens and webhook URLs, and *never printing them* is a design rule, not a preference — a leak here is a real vulnerability even at low severity
- **command or code injection** through a config value, an inventory file, or a check response
- **path traversal or arbitrary file access** via config paths, `${file:/path}` interpolation, or `--out-file`
- **crash, hang, or unbounded memory** from a malicious response to a check — the protocol parsers (m3u8, DNS wire, HAProxy CSV, RESP, NTP, CQL, RTMP) read untrusted input from whatever the target actually is, which is why they are fuzzed
- **TLS verification being skipped where it shouldn't be**, or a check reporting a target as healthy when it is not verifiable
- **supply-chain issues** in the release pipeline: the signature, the SBOM, or the published artifacts

Out of scope:

- **`InsecureSkipVerify` in the `certs` module.** It is deliberate and documented: reading a certificate's expiry must work even when the chain does not validate from where you are standing, which is frequently the whole point. The `tls` module is the one that verifies chains. Reporting this as a vulnerability will be closed as by design.
- **A check being able to reach hosts you configured.** Making network connections to the targets in the config is the program's function, not an SSRF.
- **Secrets you put in the config file yourself.** checkfleet supports `${VAR}` and `${file:/path}` precisely so you don't have to; a plaintext password in your own YAML is a configuration mistake.
- **Vulnerabilities in the targets** checkfleet monitors. Report those to their vendors.
- **Findings from a scanner with no demonstrated impact.** A `govulncheck` hit on a dependency whose vulnerable path is unreachable is worth mentioning, but it is not itself an exploit.

## What checkfleet does with secrets

Useful context for judging a report:

- Credentials are read **from the environment only** (`${VAR}`, `${VAR:-default}`, `${file:/path}`), never accepted as command-line flags where they would land in shell history and process listings.
- Webhook URLs and alerting keys come from env var *names* given on the command line (`--webhook-env`, `--key-env`), so the value itself never appears in a CI log.
- Findings and outputs are built to carry the **hostname**, not the DSN. Where a target is a connection string, only the host and port are rendered.
- checkfleet has **no server and no agent**: it runs where you invoke it, for as long as the run takes. `serve` is the exception and exposes only metrics, `/healthz` and `/readyz`.
