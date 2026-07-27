# Deploy: outlook.json generator (CYDSOL-3)

Builds and installs the systemd timer that regenerates `outlook.json`
(Spec §4) every ~4h, and the Caddy config that serves it statically at
the Spec §9 endpoint.

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
sudo mkdir -p /opt/cydsolar-api /var/www/dashboard-solar
sudo cp cydsolar-api /opt/cydsolar-api/
sudo chown -R cydsolar:cydsolar /opt/cydsolar-api /var/www/dashboard-solar

sudo cp deploy/cydsolar-outlook.service deploy/cydsolar-outlook.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cydsolar-outlook.timer
sudo systemctl start cydsolar-outlook.service   # first run, don't wait 4h
```

Merge `Caddyfile.snippet` into the real Caddyfile (adjust `root` to match
`-out`'s directory), then `sudo systemctl reload caddy`. CI/CD does not
touch Caddy config — it only ships the Go binary and the two systemd unit
files, so a Caddyfile change is still a manual, one-time step.

### GitHub repo secrets required for the `Deploy API` workflow

`VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` — already configured on this repo,
reusing the same Contabo VPS/root key the `planner` repo deploys with
(`dashboard-solar.marcosvpj.xyz` and planner's domain are the same box).
These are per-repo secrets — they don't carry over automatically between
repos even when the values match.

## Verify

```sh
systemctl status cydsolar-outlook.timer
journalctl -u cydsolar-outlook.service -n 50
curl https://dashboard-solar.marcosvpj.xyz/outlook.json
```

If Open-Meteo is down, the service exits non-zero (see journal) and
`outlook.json` is left exactly as it was — Caddy keeps serving the last
good payload. That path is covered locally by
`TestRun_OpenMeteoDown_LeavesLastGoodPayloadUntouched` in `API/main_test.go`;
this README's `curl` step is the one piece of the Done criterion that has
to be checked on the actual VPS, since Claude Code has no access to it.
