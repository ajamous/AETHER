# How RSP actually works

A high-level walkthrough of the consumer profile download flow defined
by GSMA SGP.22. This page is intentionally pragmatic: enough detail to
read the codebase, not so much that you might as well be reading the
spec directly.

If you want the canonical description, the GSMA spec is the source of
truth. Spec section references are inline so you can cross-check.

## The actors

| Actor    | What it is                                                                |
| -------- | ------------------------------------------------------------------------- |
| eUICC    | The embedded SIM chip on the device. Holds profiles and crypto keys.      |
| LPA      | Local Profile Assistant. Software on the device that talks to the eUICC and to the server. |
| SM-DP+   | The server side of the profile preparation and delivery. This is Aether's main service. |
| SM-DS    | A discovery service that tells a device which SM-DP+ has a profile waiting for it. |
| CI       | Certificate Issuer. The GSMA-rooted PKI that everyone trusts.             |
| BSS      | Business Support Systems. The carrier's order management; talks to SM-DP+ via ES2+. |

## The high-level flow

A profile download is essentially a four-step dance:

1. The carrier's BSS tells the SM-DP+ "prepare a profile for IMSI X
   to be downloaded by activation code Y"
2. The user enters the activation code (or scans a QR) on their device
3. The LPA contacts the SM-DP+, mutually authenticates, and asks for
   the profile
4. The SM-DP+ encrypts and signs the profile so that *only* this
   specific eUICC can decrypt it, and ships it down

The cryptographic interesting part is step 4: the profile is
"bound" to the eUICC, hence Bound Profile Package (BPP). Even if you
intercept it on the wire, you can't decrypt it without that specific
eUICC's private key, and that key never leaves the chip.

## ES interfaces

GSMA names the interfaces between actors. The ones you'll see most:

| Interface | Endpoints                | Used for                                     |
| --------- | ------------------------ | -------------------------------------------- |
| ES2+      | BSS ↔ SM-DP+             | Order, confirm, release, cancel a profile    |
| ES8+      | SM-DP+ ↔ eUICC (via LPA) | Application-layer secured profile delivery   |
| ES9+      | LPA ↔ SM-DP+             | HTTPS transport for the download protocol    |
| ES10b/c   | LPA ↔ eUICC              | Talking to the chip                          |
| ES11      | LPA ↔ SM-DS              | Discovery: "is there a profile for me?"      |
| ES12      | SM-DP+ ↔ SM-DS           | Event registration with the discovery service |

Aether implements the server side of ES2+, ES8+, ES9+, ES11, and ES12.
ES10b/c live on the eUICC; we don't implement those — we talk to them.

## A profile download, step by step

This is the consumer profile download flow per SGP.22 §3.1. Simplified.

```
BSS                 SM-DP+               LPA                eUICC
 |                    |                   |                   |
 |--DownloadOrder---->|                   |                   |
 |<--ICCID------------|                   |                   |
 |                    |                   |                   |
 |--ConfirmOrder----->|                   |                   |
 |<--MatchingID-------|                   |                   |
 |                    |                   |                   |
 |  (activation code passed to user, e.g. via QR)             |
 |                    |                   |                   |
 |                    |<-initiateAuth-----|                   |
 |                    |--auth challenge-->|--ESTK-------------|
 |                    |                   |<--signed challenge|
 |                    |<-authenticate-----|                   |
 |                    |--confirmation---->|                   |
 |                    |                   |                   |
 |                    |<-getBPP-----------|                   |
 |                    |--Bound Profile--->|--install--------->|
 |                    |                   |<--notification----|
 |                    |<-handleNotif------|                   |
```

The bound profile package (BPP) is the heart of it. It is the carrier's
profile, encrypted with a key derived from a fresh ECDH between the
SM-DP+ and the specific eUICC's keys, signed by the SM-DP+'s
certificate, and only decryptable by that one eUICC.

## Cryptography in 60 seconds

- **ECKA** (Elliptic Curve Key Agreement): both sides have a key pair,
  they exchange public keys, each derives the same shared secret. SGP.22
  uses NIST P-256 and Brainpool P-256 r1. Both must be supported.
- **BSP** (Bound Profile Protection, SGP.22 §2.6): given the shared
  secret, derive session keys, encrypt the profile blob with AES-128 in
  GCM, and authenticate it. The eUICC repeats the derivation locally to
  decrypt.
- **ECDSA**: the SM-DP+ signs its certificate chain and the BPP. The
  eUICC verifies against a CI root certificate it already trusts.

Everything sensitive happens in an HSM on the server side. The eUICC is
itself a tamper-resistant chip; private keys never leave it. This is
why "lift the key" attacks against properly-deployed RSP are hard —
there is no key to lift, on either end.

## Where to read the code

Once Phase 1 is in:

- `services/smdp-plus/` — the ES9+ HTTP endpoints and the ES8+
  application protocol state machine
- `pkg/crypto/` — BSP, ECKA, ECDSA helpers
- `pkg/saip/` — TCA SAIP profile package types
- `services/profile-builder/` — UPP → PPP → BPP transformation

Each spec-implementing function carries a comment naming its SGP.22
section. That isn't compliance hygiene — it's so this codebase
doubles as the textbook the field needs.

## Where to read the spec

The GSMA SGP.22 specification is published openly on the GSMA website.
For the consumer flow: §3.1 (procedures), §5.6–5.7 (ES9+ messages),
§5.8 (ES8+ messages), Annex A (state machines), Annex B (data types).
