# Deploy: outlook.json generator (CYDSOL-3)

Builds and installs the systemd timer that regenerates `outlook.json`
(Spec §4) every ~4h, and the Caddy config that serves it statically at
the Spec §9 endpoint.

## Install (on the VPS)

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
`-out`'s directory), then `sudo systemctl reload caddy`.

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
