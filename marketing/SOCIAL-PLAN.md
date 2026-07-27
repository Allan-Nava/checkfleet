# checkfleet — visibility plan (SEO, answer engines, social)

Working document, written 2026-07-27. Not published to the docs site.
Companion to the technical SEO/AEO work that ships with it (`docs/`), and to the
backlog items **CF-139…CF-146** in [BACKLOG.md](../BACKLOG.md).

**Starting point (2026-07-27):** 2 stars, 0 forks, docs site live at
`allan-nava.github.io/checkfleet`, 29 modules, a desktop app, a GitHub Action, a
Homebrew tap. The product is far ahead of its distribution. Everything below is
about closing that gap without turning into a spam account.

---

## 1. Positioning

**One line (use this verbatim, everywhere):**

> checkfleet runs domain-aware infrastructure health checks from a single Go
> binary — the checks generic monitoring can't express.

**The wedge — say this second:** *your monitoring tells you Postgres is up; it
doesn't tell you a replication slot is inactive and eating your disk.*
That sentence is the whole pitch. It survives being retold by someone else,
which is the only kind of pitch that spreads.

**Three proof points, in this order:**

1. **One static binary, no agent, no server.** Zero adoption cost — the biggest
   objection to any new monitoring tool is "another thing to run".
2. **29 modules that know their domain.** Not 29 ping checks: JetStream
   meta-leader, Patroni timelines, HLS live-edge freshness, Kafka consumer lag.
3. **It composes with what you have.** `checkfleet serve` is a Prometheus
   exporter. You are adding domain knowledge, not replacing a stack.

**Do not lead with:** the module count alone (sounds like a checklist), the
desktop app (it's a lovely second act, not the hook), or the architecture.

**Never claim:** that it replaces Prometheus/Grafana. It's the fastest way to
lose the exact audience you want, and it isn't true. The
[Why checkfleet](https://allan-nava.github.io/checkfleet/comparison/) page —
including its "when *not* to use checkfleet" section — is the canonical answer.

---

## 2. Audience, ranked by fit

| Audience | Where they are | Hook that lands |
|---|---|---|
| Platform / SRE at 10–200-person companies | HN, r/devops, r/sysadmin, Lobsters | "The check your monitoring can't express" |
| Go developers | r/golang, Golang Weekly, Gophers Slack | Zero-dep protocol implementations (SNTP, RESP, CQL handshake, SigV4 by hand) |
| Ansible / config-management users | r/ansible, Ansible community forum | Point it at your inventory, get fleet-wide TLS expiry |
| Media / streaming ops | r/VIDEOENGINEERING, Streaming Media, mediamtx & NATS communities | HLS live-edge + RTMP/SRT ingest checks — nobody else ships these |
| Homelabbers / self-hosters | r/selfhosted, r/homelab | One binary, cron, Slack report, no Docker stack |

The streaming/media angle is the most underrated one: `stream` + `ingest` +
`nats` is a combination no other tool has, and that community is small, active,
and starved of tooling. Highest signal-per-post of any channel here.

---

## 3. Assets to have ready before launch week

- [x] Social card (`docs/assets/og-card.png`) — link previews on every platform
- [x] `/llms.txt` + `/llms-full.txt` — so assistants describe the tool correctly
- [x] FAQ + comparison pages — the pages people land on from a search
- [ ] **GitHub social preview image** — Settings → General → Social preview →
      upload `docs/assets/og-card.png` (UI-only; the API can't set it)
- [ ] **Repo description + topics** — see §7
- [ ] **A 25–40s terminal recording** — `vhs` or `asciinema`, showing
      `checkfleet check all` on a config with one deliberate BAD finding, then
      `--output markdown`. This single asset carries every channel below.
- [ ] **Three screenshots** of the desktop app (dashboard, detail, trend) — the
      ones in `docs/assets/` are fine, they just need to be reused in posts
- [ ] **A pinned "Show & tell" GitHub Discussion** so replies have somewhere to go

---

## 4. Launch sequence

Do **not** post everywhere on the same day. One channel at a time; each post
gets a day of undivided attention for replies. Answering comments within the
first two hours matters more than the post copy.

| When | Channel | Goal |
|---|---|---|
| Week of 2026-07-27 | Assets + repo metadata + docs deploy | Nothing to launch into a broken shopfront |
| 2026-08-04 (Tue) | **r/devops** — the TLS-across-the-inventory story | First real feedback, low stakes |
| 2026-08-06 (Thu) | **r/golang** — the zero-dep protocol angle | Go audience, code-quality readers |
| 2026-08-11 (Tue) | **Lobsters** (`show`, tags: `devops`, `go`) | Small, high-quality critique before HN |
| 2026-08-13 (Thu) | Submit to **Golang Weekly** + **DevOps'ish** + **SRE Weekly** | Newsletters have long tails |
| 2026-08-18 (Tue) ~09:00 ET | **Show HN** | The big one. Everything above is rehearsal |
| 2026-08-20 | **r/selfhosted** + **r/sysadmin** (different framing each) | Second wave |
| 2026-08-25 | **r/VIDEOENGINEERING** — the stream/ingest modules | The niche nobody else serves |
| From 2026-09-01 | Weekly cadence (§6) | Compounding, not spiking |

Launch-day rules: no "please star", no cross-posting the identical text, reply
to every top-level comment, and treat the harshest comment as the most valuable
one — it's your next FAQ entry.

---

## 5. Channel playbooks and draft copy

### Show HN (2026-08-18)

**Title:** `Show HN: Checkfleet – domain-aware infrastructure checks in one Go binary`

**First comment** (post it yourself, immediately):

> I kept writing the same `curl | jq` scripts to answer questions my monitoring
> couldn't: is the NATS JetStream meta-leader actually elected, is a Postgres
> replication slot inactive and retaining WAL, is the HLS live edge still fresh,
> do all 300 hosts in the Ansible inventory have 30 days of certificate left.
>
> checkfleet is that folder of scripts, rewritten as 29 typed modules in one
> static Go binary. You write a checkfleet.yml, run `checkfleet check all`, and
> get worst-first findings as text, Markdown, JSON, a Slack message, SARIF, or
> Prometheus metrics. No agent on the targets, no server to run.
>
> Two design decisions I'd defend: exit code is 0 even when findings are BAD (a
> check that *ran* is a successful run — you opt into gating with `--exit-on`),
> and ERROR is a distinct status from BAD, so "I couldn't measure" never
> masquerades as "your service is broken".
>
> It's explicitly not a Prometheus replacement — `checkfleet serve` exposes the
> findings as metrics so it plugs into what you already alert on.
>
> Licence: PolyForm Noncommercial, free for personal/research/nonprofit use,
> commercial use needs a separate licence. Happy to talk about that choice —
> I know it's not OSI open source and I'd rather be straight about it up front.

Putting the licence in your own first comment defuses it. Discovered later, it
reads as a bait-and-switch; volunteered, it reads as honesty.

### r/devops (2026-08-04)

Title: *"I got tired of my monitoring telling me Postgres was 'up'"*

Lead with one concrete war story (the inactive replication slot that filled a
disk, or the certificate that expired on a host nobody remembered). Show the
actual terminal output. Put the repo link at the *end*, after the value. Reddit
punishes an opening link and rewards a story.

### r/golang (2026-08-06)

Title: *"Writing SNTP, RESP, the CQL handshake and AWS SigV4 by hand to keep a
CLI dependency-free"*

This is a **Go engineering post that happens to be about checkfleet**, not an
ad. Show `go.mod` — 5 dependencies for 29 modules — and explain the rule for
when a driver is justified (pgx, franz-go, mongo-driver) versus hand-rolled.
That rule is genuinely interesting to Go people.

### Lobsters (2026-08-11)

Post as `show`. Lobsters readers will find the sharp edges (`InsecureSkipVerify`
in the certs module is the obvious one). Have the answer ready and in the docs:
it's an expiry *reader*, not a chain validator — the `tls` module does chain
validation. Being pre-armed here is worth more than the traffic.

### X / Bluesky / Mastodon (`@…@fosstodon.org` is the right instance)

Threads, not one-offs. One thread per module family, each ending with a link to
the module's docs anchor:

1. "Your monitoring says the port is open. Here's what it doesn't say." (7 posts,
   one per surprising failure mode, each with the checkfleet line that catches it)
2. "29 checks, 5 dependencies. Here's the rule." (the zero-dep discipline)
3. "TLS expiry across 300 hosts you already describe in Ansible." (one command)

Mastodon's `#devops`, `#golang`, `#selfhosted` tags actually work. X barely
does for this audience — post there, don't invest there.

### LinkedIn

Different audience: engineering managers and CTOs, not operators. Different
frame: *reduce unknown-unknowns before a maintenance window; gate deploys on
infrastructure state*. One post a month, maximum. Reuse the comparison page's
"where it fits" list.

### dev.to / Hashnode (canonical-linked back to the docs site)

Long-form, evergreen, and a genuine SEO asset — these domains rank fast and
`rel=canonical` sends the authority home. Three to write:

1. "How to check TLS certificate expiry across an Ansible inventory"
2. "Gating a CI pipeline on infrastructure health without breaking every build"
   (baseline + `--fail-on-new`, an under-told feature)
3. "What Prometheus can't tell you about your NATS cluster"

Each targets a real search query and ends with the tool as the answer.

### Awesome lists and directories

Free, permanent, and heavily scraped by both search engines and LLM training
sets. Submit to: `awesome-go` (monitoring), `awesome-sysadmin`,
`awesome-selfhosted` (check the licence policy first — noncommercial may be
excluded, don't waste a PR), `awesome-devops`, `awesome-sre`,
`awesome-prometheus`. Also: AlternativeTo, LibHunt, StackShare, Slant, and a
Wikidata entry (LLMs read Wikidata).

---

## 6. Sustained cadence (from 2026-09-01)

The launch is a spike; this is the compounding part. One shipped thing, one
post, every week — the repo already produces the raw material because every
commit is a tagged release.

**Weekly (30 minutes):** pick the most interesting change since last week and
post it once, to the single best-fitting channel. A release note is not content;
*"here's the failure mode this new module catches"* is.

**Monthly:** one dev.to/Hashnode article (§5), and one docs page that answers a
search query directly. The per-module deep-dive pages are the obvious backlog —
"checkfleet Kafka consumer lag check" is a query nobody is competing for.

**Quarterly:** a "state of checkfleet" post — what shipped, what's next, what
users asked for. Good for the newsletter list and for LinkedIn.

**Reactive, highest value of all:** when someone posts *"how do I check X?"* on
r/devops, r/sysadmin or a Slack/Discord you're in, answer the question properly
**first**, then mention checkfleet only if it genuinely fits. This converts
better than every planned post combined, and it costs nothing but attention.

---

## 7. Repo metadata (run once, today)

GitHub's own search and every LLM that reads repo metadata use these. Current
topics are four near-synonyms of "monitoring"; they should name the
technologies people search for.

```bash
gh repo edit Allan-Nava/checkfleet \
  --description "Domain-aware infrastructure health checks in one Go binary — TLS expiry, HTTP, DNS, NATS, Kafka, PostgreSQL, Consul, HAProxy and 20+ more. No agents, no server." \
  --homepage "https://allan-nava.github.io/checkfleet/" \
  --add-topic go --add-topic golang --add-topic cli --add-topic devops \
  --add-topic sre --add-topic monitoring --add-topic observability \
  --add-topic health-check --add-topic healthcheck --add-topic tls-certificates \
  --add-topic certificate-monitoring --add-topic ansible --add-topic prometheus \
  --add-topic nats --add-topic kafka --add-topic postgresql --add-topic consul \
  --add-topic haproxy --add-topic infrastructure-monitoring
```

(GitHub caps a repo at 20 topics.) Then, in the web UI:
**Settings → General → Social preview →** upload `docs/assets/og-card.png`.

---

## 8. Measurement

Track five numbers, monthly, in one place. Anything more is procrastination.

| Metric | Where | Why it matters |
|---|---|---|
| Unique cloners + views | Insights → Traffic (14-day window, so record it monthly) | Real usage, unlike stars |
| Referring sites | Insights → Traffic | Tells you which channel actually worked |
| Impressions & top queries | Google Search Console (verify the Pages site, submit `/sitemap.xml`) | Whether the SEO work is landing |
| `go install` / release downloads | Releases API, `gh api repos/Allan-Nava/checkfleet/releases` | Adoption, not curiosity |
| Assistant answers | Ask ChatGPT / Claude / Perplexity "what is checkfleet" and "tools to check TLS expiry across a fleet" monthly | The only way to see whether the AEO work took |

That last row is the new one and worth doing seriously: record the answers
verbatim in this file over time. If an assistant describes checkfleet wrongly,
the fix is a docs page that states the fact plainly — that's exactly what
`/llms.txt` and the FAQ are for.

**Realistic 6-month target:** 300–600 stars, a handful of external issues, and
one or two "we use this in production" reports. A Show HN front page is worth
1–3k stars but it is not repeatable; the weekly cadence is.

---

## 9. Risks, and what to do about them

- **The licence.** PolyForm Noncommercial will draw criticism on HN and Lobsters,
  and it disqualifies the repo from some awesome-lists. Mitigation: state it
  first, in your own words, every time (see the Show HN comment). Don't argue —
  explain the reasoning once and move on. Consider whether a dual "free for
  companies under N people" tier would remove most of the friction; that's a
  business decision, but it's the single biggest adoption variable here.
- **"Yet another monitoring tool" fatigue.** Mitigation: never lead with the
  category, always lead with a specific failure mode. The comparison page's
  "when not to use it" section is a credibility asset — link it early.
- **Posting faster than you can support.** A launch that lands brings issues.
  Better to launch a week later with a triage habit than to leave the first
  three issues unanswered — that's visible forever.
- **Sounding like marketing.** Every channel here is allergic to it. The test
  for any post: would you upvote this if someone else wrote it?
