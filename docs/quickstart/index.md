# Quick start

!!! warning "Phase 0 — not yet runnable"
    The lab stack does not yet exist. This page documents the experience
    we are building toward. Today, `make lab-up` prints a placeholder
    message and exits. The first runnable lab lands in Phase 1.

The promise — once Phase 1 lands — is:

```bash
git clone https://github.com/ajamous/aether
cd aether
make lab-up
```

In under 60 seconds, this brings up:

- PostgreSQL 16 (state)
- Redis 7 (RSP session state, ephemeral keys)
- NATS JetStream (events)
- SoftHSM v2, pre-initialized with the SGP.26 test cert chain
- All Aether services (`smdp-plus`, `smds`, `eim`, `profile-builder`,
  `certmgr`, `hsm-broker`, `audit`, `gateway`)
- The admin UI at `https://aether.local` (self-signed TLS)

Pre-seeded data:

- 10 sample profile templates
- SGP.26 test certs in the lab cert mode
- A demo operator account for the UI

## Tear down

```bash
make lab-down
```

Removes the stack and its volumes. There is nothing to clean up by hand.

## What's next

Once the lab is up:

1. Open the UI, browse the sample profiles
2. Run an order from the UI to generate a Bound Profile Package
3. With a sysmoEUICC test card, drive a profile download from a real
   Android device against your local Aether instance

The full walkthrough lives at [learn/how-rsp-works.md](../learn/how-rsp-works.md).
