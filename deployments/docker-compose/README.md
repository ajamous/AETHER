# Aether lab — Docker Compose

The local development stack. From a fresh clone:

```
make lab-up      # ~60 seconds; builds and starts everything
make lab-logs    # tail combined logs
make lab-down    # tear down + remove volumes
```

## What runs

| Service          | Port  | Purpose                                              |
| ---------------- | ----- | ---------------------------------------------------- |
| postgres         | —     | State store (placeholder — services use it once persistence lands) |
| redis            | —     | Session/state cache (planned for smdp-plus)          |
| nats             | —     | Event bus (planned for audit subscribers)            |
| certgen          | —     | One-shot: generates the lab cert chain               |
| hsm-broker       | 8443  | PKCS#11 façade, memory backend                       |
| certmgr          | 8444  | Cert store, /metrics for Prometheus                  |
| smdp-plus        | 8445  | SM-DP+ ES9+ endpoints                                |
| profile-builder  | 8446  | Profile templates                                    |
| audit            | 8447  | Hash-chained audit log                               |
| gateway          | 8080  | ES2+ for BSS, REST for UI; entrypoint                |
| admin-ui         | 3000  | Next.js operator console (read-only, no auth)        |

## Smoke test

After `make lab-up`:

```
curl http://localhost:8080/v1/health
curl http://localhost:8080/v1/templates
curl http://localhost:8080/v1/certs
curl http://localhost:8444/metrics | head -30
```

Or open the admin UI in a browser at <http://localhost:3000>.

## What this is honestly NOT

- Not a production deployment. Use `prod-reference.yml` (forthcoming)
  for that.
- Not running against a real HSM. The hsm-broker uses the in-memory
  backend; SoftHSM integration ships when its backend is fleshed out.
- Not running real Postgres/Redis/NATS workloads. Those services are
  up and healthy; the application code that uses them lands in
  Phase 1 task follow-ups.
- Not driving a real eUICC. The smdp-plus endpoints accept the right
  shapes but BPP generation returns 501 NotImplemented until the SAIP
  codec lands. See services/smdp-plus/README.md.

The lab is enough to walk through every HTTP surface, watch the
audit chain grow as you poke endpoints, and read the certmgr metrics
update. It is not yet enough to talk to a sysmoEUICC.
