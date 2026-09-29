# Discoverability metrics

The "one place" of `SOCIAL-PLAN.md` §8. One section per month, newest last.

- **Generated rows** come from `scripts/metrics.sh --append` (GitHub traffic, release
  downloads, stars). Traffic is a rolling 14-day window: compare months, don't sum them.
- **Search Console**: impressions, clicks and the top 5 queries for the Pages property,
  copied from Performance → Search results (last 28 days).
- **Assistant answers**: ask ChatGPT, Claude and Perplexity, in a fresh chat with no
  context, the two prompts below. Record each answer's gist in one line, and whether it
  is **correct**, **wrong** (and what) or **unknown** (the assistant has not heard of it).
  A wrong answer is a docs bug: fix the fact in `docs/faq.md` / `docs/llms.txt`.
  1. `what is checkfleet`
  2. `tools to check TLS certificate expiry across a fleet of servers`

## 2026-09-29

| Metric | Value |
|---|---|
| Views (14 days) | 43 views / 13 unique |
| Clones (14 days) | 148 clones / 69 unique |
| Referring sites (14 days) | github.com (10), Google (1), allan-nava.github.io (1) |
| Release downloads (all time) | 2324 (latest v1.34.1: 5) |
| Stars / forks | 7 stars, 0 forks |
| Search Console impressions & top queries | _fill in by hand_ |
| Assistant answers | _fill in by hand: see the prompts at the top of this file_ |
