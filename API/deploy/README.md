# Deploy: outlook.json generator

Builds and installs the systemd timer that regenerates `outlook.json`
every ~4h, and the Caddy config that serves it statically at a fixed
path on the VPS.

## Deploy

Every push to `main` that touches `API/**` builds, tests, and ships the
binary via `.github/workflows/deploy.yml`: test → cross-compile → scp
binary + unit files to the VPS → SSH step that creates the `cydsolar`
user/dirs if missing, installs the binary, installs the unit files,
and restarts the timer + runs the service once. That SSH step is
idempotent (`id -u cydsolar || useradd ...`, `mkdir -p`, `chown -R`), so
it doubles as first-time bootstrap — no separate manual install step
needed on a fresh VPS, as long as Caddy is already pointed at
`/var/www/dashboard-solar` (see below).

The manual path (`go build` + `install`/`useradd` by hand) still works
if you need to deploy without CI, e.g. from a laptop with direct VPS
access:

```sh
go build -o cydsolar-api .            # from API/
sudo useradd --system --no-create-home cydsolar   # if it doesn't exist yet
sudo mkdir -p /opt/cydsolar-api /var/www/dashboard-solar /var/lib/cydsolar
sudo cp cydsolar-api /opt/cydsolar-api/
sudo chown -R cydsolar:cydsolar /opt/cydsolar-api /var/www/dashboard-solar /var/lib/cydsolar

sudo cp deploy/cydsolar-outlook.service deploy/cydsolar-outlook.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cydsolar-outlook.timer
sudo systemctl start cydsolar-outlook.service   # first run, don't wait 4h
```

Merge `Caddyfile.snippet` into the real Caddyfile (adjust `root` to match
`-out`'s directory), then `sudo systemctl reload caddy`. CI/CD does not
touch Caddy config — it only ships the Go binary and the two systemd unit
files, so a Caddyfile change is still a manual, one-time step.

The binary requires `-lat`/`-lon` and refuses to run without them.
`cydsolar-outlook.service` reads them from `/etc/cydsolar/coords.env`,
which is not versioned and not touched by CI/CD — create it once on the
VPS:

```sh
sudo mkdir -p /etc/cydsolar
sudo tee /etc/cydsolar/coords.env <<'EOF'
LATITUDE=<your site's latitude>
LONGITUDE=<your site's longitude>
EOF
sudo chmod 600 /etc/cydsolar/coords.env
```

### GitHub repo secrets required for the `Deploy API` workflow

`VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` — already configured on this repo.
These are per-repo secrets — they don't carry over automatically between
repos even when the values match.

## Verify

```sh
systemctl status cydsolar-outlook.timer
journalctl -u cydsolar-outlook.service -n 50
curl https://<your-domain>/outlook.json
tail -f /var/lib/cydsolar/forecast-history.ndjson
```

`/var/lib/cydsolar/forecast-history.ndjson` gets one line appended per run
(not served by Caddy — it's outside `/var/www`). Worth including in whatever
backs up the VPS: unlike `outlook.json`, it isn't reproducible after the
fact if lost.

Pull `(run_at, date, psh)` triples out for a spreadsheet:

```sh
jq -r '.run_at as $r | .days[] | [$r, .date, .psh] | @csv' \
  /var/lib/cydsolar/forecast-history.ndjson
```

If Open-Meteo is down, the service exits non-zero (see journal) and
`outlook.json` is left exactly as it was — Caddy keeps serving the last
good payload. That path is covered locally by
`TestRun_OpenMeteoDown_LeavesLastGoodPayloadUntouched` in `API/main_test.go`;
this README's `curl` step is the one piece of the Done criterion that has
to be checked on the actual VPS, since Claude Code has no access to it.
