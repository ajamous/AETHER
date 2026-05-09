# services/hsm-broker

Single PKCS#11 façade for all of Aether. Every cryptographic operation
that touches a long-lived private key in the SM-DP+ identity hierarchy
(DPtls, DPauth, DPpb) routes through here. Backends are pluggable.

See [ADR 0003](../../docs/architecture/adr/0003-pkcs11-abstraction.md)
for the rationale.

## Status

| Piece                                    | Status        |
| ---------------------------------------- | ------------- |
| Broker Go interface (`internal/broker`)  | Implemented   |
| Memory backend (test / CI fallback)      | Implemented   |
| SoftHSM v2 backend (PKCS#11)             | Skeleton      |
| AWS CloudHSM / GCP / Azure / Thales / Utimaco backends | Not started |
| HTTP+JSON server                         | Implemented   |
| gRPC server                              | Pending — see "Wire format" below |
| Health probe                             | Implemented   |

The memory backend holds keys in process memory. **It is for tests and
local development only.** Production deployments use SoftHSM (lab) or a
PKCS#11-conforming HSM (production).

## Wire format

The broker exposes its API over HTTP+JSON for now. The plan calls for
gRPC, and the migration is mechanical:

1. The Go interface in `internal/broker/broker.go` is the canonical
   contract.
2. The `.proto` definitions in `api/v1/hsm.proto` describe the same
   contract in protobuf shape.
3. When `protoc` is part of the build environment, generated stubs
   replace the hand-written HTTP server.

We chose to ship the HTTP+JSON path first because the `.proto` →
generated-Go pipeline needs protoc tooling that doesn't belong in
every contributor's environment. HTTP+JSON unblocks development; gRPC
is a follow-up PR that touches the wire layer only.

## Operations exposed

| Operation         | Purpose                                                             |
| ----------------- | ------------------------------------------------------------------- |
| `Sign`            | ECDSA signature over caller-provided digest                          |
| `Decrypt`         | RSA-OAEP / ECIES decryption (HSM-internal private key)               |
| `DeriveKey`       | ECKA + KDF inside the HSM, returning a session key handle            |
| `GenerateKeyPair` | Generate a fresh keypair on the configured curve                     |
| `ListKeys`        | Enumerate available keys (metadata only — never key material)        |
| `Health`          | Liveness probe (does the broker have a live PKCS#11 session?)        |

`Sign` accepts a digest, not the message — the broker does not hash
on your behalf. This matches PKCS#11 semantics and keeps hash-algorithm
choice with the caller, which is where SGP.22 §H.5 puts it.

There is **no** `Export` or `GetPrivateKey`. Private key material does
not cross the broker boundary. Ever. Sessions derived inside the HSM
are referenced by handle.

## Running

```
go run ./cmd/hsm-broker --backend=memory --listen=:8443
```

For SoftHSM:

```
go run ./cmd/hsm-broker \
    --backend=softhsm \
    --pkcs11-lib=/usr/lib/softhsm/libsofthsm2.so \
    --slot=0 \
    --pin=$PIN
```

The lab Docker Compose wires SoftHSM and the broker together
out of the box. See `deployments/docker-compose/lab.yml`.

## Why HTTP+JSON now

A working RPC layer with full test coverage is more useful than a
gRPC stub blocked on tooling. When protoc lands in the build, the
swap is mechanical: the Go interface and the proto file already
agree. CI will lock that compatibility once the gRPC migration PR
ships.
