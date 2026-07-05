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

The current production `new-api` container remains the blue slot. The green
slot is created only when a candidate build is deployed.

## One-Time Preparation

Install the Nginx upstream and switch the site from a direct `proxy_pass` to
the managed upstream:

```powershell
./scripts/install-nexus-canary-nginx.ps1 -Yes
```

This starts with `blue=100`, `green=0`; it should not change production traffic.
The script backs up Nginx config, runs `nginx -t`, and reloads only if valid.

## Deploy A Candidate To Green

Build the current checkout and deploy only the green slot:

```powershell
./scripts/deploy-nexus-green.ps1 -Yes
```

This does not change Nginx traffic. It creates:

```text
/opt/new-api/docker-compose.green.override.yml
/opt/new-api/new-api-green.env
```

The green env is copied from the active `new-api` container, with:

```text
PORT=3000
NODE_NAME=nexus-sg-new-api-green
BATCH_UPDATE_ENABLED=false
```

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

The green slot disables `BATCH_UPDATE_ENABLED` by default. System tasks use the
project's DB lease mechanism, but any new background job must be checked for
multi-instance safety before canary traffic is enabled.

## Notes

- Existing long-running stream requests stay on the slot that accepted them.
- Nginx reload is graceful, but switching weights only affects new requests.
- `hwdrama-proxy` is independent and remains on its existing port/service.
