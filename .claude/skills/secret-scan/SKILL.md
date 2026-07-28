---
name: secret-scan
description: Use when asked to check this repo for leaked secrets/credentials, before making a repo public, before a big push, or after any commit made with `--no-verify`. Also read this when the pre-commit hook at .githooks/pre-commit fires and the finding is NOT a false positive -- it explains the required incident-response steps (rotate + purge), not just "remove the line." The hook (.githooks/pre-commit) only screens staged diffs of new commits; it does not see files already merged into history, so a one-off deep scan is still needed periodically.
---

# Secret scan (felicity-cyd-v0 is public)

## Quick full-repo + history scan

The pre-commit hook only looks at staged diffs going forward. To check
everything already in history/working tree:

```bash
# tracked working tree, filename-based
git ls-files | grep -iE '\.env|\.pem|\.key|\.p12|\.pfx|id_rsa|id_ed25519|secrets\.h$'

# full commit history, content-based (slow but thorough)
git log --all -p | grep -inE \
  'AKIA[0-9A-Z]{16}|-----BEGIN (RSA|EC|OPENSSH|PGP) PRIVATE KEY|gh[pousr]_[A-Za-z0-9]{36,}|xox[baprs]-|AIza[0-9A-Za-z_-]{35}|sk_live_|(api[_-]?key|secret|passwd|password|token)[[:space:]]*[:=][[:space:]]*["'"'"'][^"'"'"'[:space:]]{8,}'
```

Ignore matches against `.example` files, `SECRET_WIFI_SSID`/`SECRET_HOST`
placeholder names, and `secrets.VPS_*` in `.github/workflows/deploy.yml`
(those are GitHub Actions secret *references*, not values).

## If a real secret is found in history

Removing the line in a new commit is NOT enough -- it's still readable in
prior commits on a public repo, and search engines / bots crawl GitHub
history.

1. **Rotate the credential first.** Assume it's already compromised the
   moment it was pushed. Rotating makes every later step non-urgent.
2. Then decide whether to purge history (`git filter-repo`) and force-push,
   or just leave the dead credential in history since it's rotated. For this
   repo (solo dev, small history), purging is cheap -- but **confirm with
   Marcos before force-pushing** main; it rewrites shared history.
3. Note the incident in a `create_log` entry via the planner MCP if it's
   tied to a task, so there's a record of what leaked and when it was
   rotated.

## Reminder

Never advise `git commit --no-verify` to get past this hook without
confirming with Marcos that the flagged content genuinely isn't a secret.
If it's a false positive, fix it at the source: `# allow-secret` on the
line, or add the path to `.secretsallow`.
