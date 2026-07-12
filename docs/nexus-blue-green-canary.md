# Nexus SG Blue/Green Canary Runbook

This runbook prepares `llm.ai.nexus-reach.com` for low-downtime new-api
deployments on the current single production server.

## Topology

```text
Nginx new_api_backend
  blue  -> 127.0.0.1:3000  current stable new-api container
  green -> 127.0.0.1:3002  candidate new-api-green container

Both slots use the same PostgreSQL, Redis, data volume, and logs directory.
```

Blue and Green alternate as the active slot. A new release is always deployed
to whichever slot Nginx currently marks `down`.

## One-Time Preparation

Install the Nginx upstream and switch the site from a direct `proxy_pass` to
the managed upstream:

```powershell
./scripts/install-nexus-canary-nginx.ps1 -Yes
```

This starts with `blue=100`, `green=0`; it should not change production traffic.
The script backs up Nginx config, runs `nginx -t`, and reloads only if valid.

## Deploy A Candidate To The Inactive Slot

Build the current checkout and automatically deploy only the inactive slot:

```powershell
./scripts/deploy-nexus-slot.ps1 -Slot Auto -BatchUpdateMode Direct -Yes
```

The script reads `/etc/nginx/conf.d/new-api-upstream.conf`. If Blue is `down`,
it deploys Blue; if Green is `down`, it deploys Green. It refuses to run while
both slots receive traffic or if an explicitly selected slot is active. The
selection is checked again on the server immediately before container
replacement to prevent a concurrent traffic change from creating a race.

This does not change Nginx traffic. Depending on the selected slot it creates:

```text
/opt/new-api/docker-compose.blue.override.yml
/opt/new-api/new-api-blue.env
/opt/new-api/docker-compose.green.override.yml
/opt/new-api/new-api-green.env
```

The candidate env is copied from the active slot, with slot-specific values:

```text
PORT=3000
NODE_NAME=nexus-sg-new-api-blue|green
BATCH_UPDATE_ENABLED=false
BATCH_UPDATE_INTERVAL=5
```

`Direct` is the release default because it leaves no in-memory quota deltas
when a candidate is restarted or rolled back. `Batch` must be selected
explicitly. The legacy `deploy-nexus-green.ps1` command remains as a wrapper,
but refuses to overwrite Green while Green is active.

## Canary Traffic

Shift traffic by changing Nginx upstream weights:

```powershell
./scripts/set-nexus-canary.ps1 -GreenWeight 1 -Yes
./scripts/set-nexus-canary.ps1 -GreenWeight 5 -Yes
./scripts/set-nexus-canary.ps1 -GreenWeight 50 -Yes
./scripts/set-nexus-canary.ps1 -Promote -Yes
```

Rollback is just a weight change:

```powershell
./scripts/set-nexus-canary.ps1 -Rollback -Yes
```

## Checks

Before sending traffic to green:

```powershell
ssh nexus-sg "curl -fsS http://127.0.0.1:3002/api/status"
ssh nexus-sg "docker ps --filter name=new-api-green"
ssh nexus-sg "docker logs --tail 120 new-api-green"
```

After shifting traffic:

```powershell
curl -fsS https://llm.ai.nexus-reach.com/api/status
ssh nexus-sg "sudo nginx -t"
ssh nexus-sg "docker logs --tail 120 new-api"
ssh nexus-sg "docker logs --tail 120 new-api-green"
```

## Migration Rules

Blue and green run against the same database during canary. Therefore release
changes must be backward compatible:

- Add tables/columns/indexes first.
- Do not remove columns or change existing column meaning in the same release.
- Do not require old and new code to write incompatible data shapes.
- For destructive schema cleanup, use a later release after all traffic is on
  the new version and rollback is no longer needed.

## Background Task Rules

Candidate slots disable `BATCH_UPDATE_ENABLED` by default. System tasks use the
project's DB lease mechanism, but any new background job must be checked for
multi-instance safety before canary traffic is enabled.

## Notes

- Existing long-running stream requests stay on the slot that accepted them.
- Nginx uses consistent client-IP hashing so one browser stays on one slot while
  loading HTML and hashed frontend assets during a weighted canary.
- Nginx reload is graceful, but switching weights only affects new requests.
- `hwdrama-proxy` is independent and remains on its existing port/service.
