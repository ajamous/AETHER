# HSM vendor configuration

The Aether `hsm-broker` exposes a single PKCS#11 façade. The same
backend code path serves every PKCS#11 v2.40-compatible HSM the
SAS-SM Standard accepts. This page documents the per-vendor
plumbing — `.so` path, slot/token initialisation, PIN handling,
known quirks — so an operator can plug their FIPS 140-2/3 cluster
in without re-discovering the wiring.

The Aether code does not ship vendor-specific shims. It ships the
SoftHSM v2 lab default (verified end-to-end in CI) and a PKCS#11
backend that calls into whatever `.so` the operator points at.
Per-vendor *integration* — running the broker against a real
production cluster and checking SGP.22 §5.7.13 ServerSigned1
verification end-to-end — is a hardware-in-the-loop bench follow-up
called out under [common-findings.md](common-findings.md). The
sections below give every vendor a concrete starting point so that
follow-up bench is short.

## Configuration shape

Three pieces apply to every backend:

```
--backend=pkcs11
--pkcs11-lib=<path/to/vendor.so>
--slot=<token slot id>
--pin=<token user PIN>            # prefer HSM_PIN env var
```

The Helm chart wires these via `hsmBroker.backend`,
`hsmBroker.pkcs11.libraryPath`, `hsmBroker.pkcs11.slot`, and
`hsmBroker.pkcs11.pinSecret` — see the chart's README.

`HSM_PIN` is the recommended way to deliver the PIN; it lets the
chart's pinSecret stay opaque to the cluster RBAC story.

## SoftHSM v2 (lab)

The lab default. Used by CI and `make lab-up`.

| Setting           | Value                                    |
| ----------------- | ---------------------------------------- |
| `--pkcs11-lib`    | `/usr/lib/softhsm/libsofthsm2.so` (Debian); `/usr/local/lib/softhsm/libsofthsm2.so` (build-from-source) |
| `--slot`          | Output of `softhsm2-util --show-slots`   |
| Token init        | `softhsm2-util --init-token --slot 0 --label aether --pin 1234 --so-pin 5678` |
| FIPS rating       | None (software). NOT for production.     |

Quirks: SoftHSM v2 supports `CKD_NULL` for `CKM_ECDH1_DERIVE` but
not the SGP.22-mandated `CKD_SHA256_KDF`. The Aether SoftHSM
backend takes the raw shared secret out of the HSM, runs
`pkg/crypto/kdf` in process, and zeroes the intermediate. Real
HSMs that implement `CKD_SHA256_KDF` natively will run the KDF
on-chip; the production backends below assume that path is
available and fall back to the in-process KDF when it is not.

## AWS CloudHSM

FIPS 140-2 Level 3 (Marvell LiquidSecurity adapter). The Aether
Terraform module under `deployments/terraform/aws/modules/cloudhsm/`
provisions the cluster.

| Setting           | Value                                                       |
| ----------------- | ----------------------------------------------------------- |
| Client install    | `aws s3 cp s3://aws-cloudhsm-pkcs11/AmazonCloudHsmPkcs11-latest.x86_64.rpm` then `yum install` |
| `--pkcs11-lib`    | `/opt/cloudhsm/lib/libcloudhsm_pkcs11.so`                   |
| `--slot`          | `1` (CloudHSM exposes a single token at slot 1)             |
| Cluster cert      | `/opt/cloudhsm/etc/customerCA.crt` — the cluster's bootstrap cert from the activation ceremony |
| Crypto user       | Created via `cloudhsm-cli user create-user` after activation; the user PIN is the `HSM_PIN` |
| Activation        | Two-person procedure per `docs/sas-sm/key-ceremony.md`; CloudHSM is created in INITIALIZED state, the ceremony moves it to ACTIVE |

Pod plumbing: the AWS CloudHSM client uses `cloudhsm-jce`/`cloudhsm-cli`
to authenticate. For the Aether broker we mount
`/opt/cloudhsm/etc/cloudhsm-pkcs11.cfg` from a ConfigMap and
`/opt/cloudhsm/etc/customerCA.crt` from a Secret. The `HSM_IP_RETRY`
and `HSM_CA_CERT` env vars in the cfg point at the cluster's
ENI IPs (Terraform output: `cloudhsm_cluster_id` resolves the
ENIs via `aws cloudhsmv2 describe-clusters`).

Quirks: AWS CloudHSM enforces strict mechanism templates; the
Aether SoftHSM backend's attribute defaults work on CloudHSM
without changes. `CKM_ECDH1_DERIVE` with `CKD_SHA256_KDF` IS
supported on-chip — the in-process KDF fallback will not run.

## GCP Cloud HSM (via Cloud KMS PKCS#11)

FIPS 140-2 Level 3 (Marvell LiquidSecurity behind Cloud KMS).
The Aether Terraform module under
`deployments/terraform/gcp/modules/cloudhsm/` provisions the
KMS key ring at HSM protection level.

| Setting           | Value                                                                          |
| ----------------- | ------------------------------------------------------------------------------ |
| Library install   | Download `libkmsp11.so` from Google's GitHub releases; verified-vendor signing |
| `--pkcs11-lib`    | `/opt/google/lib/libkmsp11.so`                                                 |
| `--slot`          | `0`                                                                            |
| Config            | YAML at `KMS_PKCS11_CONFIG=/etc/google/kmsp11.yaml` listing the key ring URI and Workload Identity SA |
| Auth              | Workload Identity (the Aether `hsm-broker-id` GSA created by the Terraform IAM submodule) |
| `HSM_PIN`         | Empty — Cloud KMS PKCS#11 authenticates via WI, not a PIN                      |

The `kmsp11.yaml` shape:

```yaml
---
tokens:
  - key_ring: "projects/<project>/locations/<region>/keyRings/aether-hsm"
    label: "aether"
log_directory: "/var/log/kmsp11"
```

Quirks: Cloud KMS PKCS#11 is read-mostly — `CKM_EC_KEY_PAIR_GEN`
requires elevated IAM. The key ceremony generates the identity
keys via `gcloud kms keys create`, not via the broker's
`GenerateKeyPair`. The broker's `Sign` and `DeriveKey` work
unchanged.

## Azure Managed HSM

FIPS 140-3 Level 3 (single-tenant). The Aether Terraform module
under `deployments/terraform/azure/modules/hsm/` provisions the
Managed HSM. Activation is a manual two-person security-domain
ceremony — see `docs/sas-sm/key-ceremony.md`.

| Setting           | Value                                                       |
| ----------------- | ----------------------------------------------------------- |
| PKCS#11 wrapper   | Azure does not ship a first-party PKCS#11 module for Managed HSM. Use [`pkcs11-azure`](https://github.com/Azure/pkcs11-azure-shim) — a community shim that translates PKCS#11 to the Managed HSM data-plane REST API |
| `--pkcs11-lib`    | `/opt/aether/lib/pkcs11-azure.so` (operator-staged)         |
| `--slot`          | `0`                                                         |
| `HSM_PIN`         | An AAD token or workload-identity bearer (the shim accepts these where PKCS#11 expects a PIN) |
| Activation        | Security-domain download + restore, two-person quorum       |

This is the SAS-SM-conformant Azure path. Azure Key Vault Premium
HSM-backed keys are FIPS 140-2 Level 2 only and are NOT sufficient
for the Standard.

Quirks: round-trip latency is higher than CloudHSM/Cloud KMS
because the data plane is REST-over-HTTPS rather than
PCIe/network-attached PKCS#11. Tune the gateway's HSM-timeouts
upward in production. `CKM_ECDH1_DERIVE` is supported on-chip;
`CKD_SHA256_KDF` may need the in-process KDF fallback depending
on the shim version.

## Thales Luna Network HSM

FIPS 140-3 Level 3. Common in on-prem deployments. See
[`reference-onprem.md`](reference-onprem.md) for the surrounding
topology.

| Setting           | Value                                                      |
| ----------------- | ---------------------------------------------------------- |
| Client install    | Luna Client SDK from Thales (license-gated)                |
| `--pkcs11-lib`    | `/usr/safenet/lunaclient/lib/libCryptoki2_64.so`           |
| `--slot`          | Output of `cmu list -s 0` after token assignment           |
| Partition init    | `lunacm: par init -label aether-prod-partition` (HSM admin) |
| Crypto Officer PIN| `HSM_PIN` — set during `par init`                          |
| HA group          | Configured via `vtl haAdmin` to spread across 2+ HSMs      |

Quirks: Luna's `Crypto Officer` and `Crypto User` roles map onto
PKCS#11 user types differently from SoftHSM. The Aether broker's
template assumes the `Crypto Officer` PIN; this matches Luna's
default. `CKD_SHA256_KDF` is supported on-chip.

## Utimaco SecurityServer

FIPS 140-3 Level 3. Common in EU regulated deployments.

| Setting           | Value                                                  |
| ----------------- | ------------------------------------------------------ |
| Client install    | Utimaco SecurityServer SDK from the vendor             |
| `--pkcs11-lib`    | `/etc/utimaco/libcs_pkcs11_R3.so` (legacy: `R2`)       |
| Config file       | `CS_PKCS11_R3_CFG=/etc/utimaco/cs_pkcs11_R3.cfg`       |
| `--slot`          | Per the configured CryptoServer `Logical CPU` slot     |
| Token init        | `csadm CreateUser=USR_NAME,1234` against the appliance |
| `HSM_PIN`         | The created user's password                            |

Quirks: Utimaco serialises `Sign` calls per logical CPU; for high
ServerSigned1 throughput, allocate multiple logical CPUs and
front them with multiple `hsm-broker` replicas. SGP.22 §H.5
algorithms are all supported on-chip including
`CKD_SHA256_KDF`.

## What "Implemented" honestly means

The Aether code path that talks PKCS#11 is one path, exercised
end-to-end against SoftHSM v2 in CI. The vendor sections above
document the plumbing each cluster needs to bring up that same
code path. We deliberately do NOT claim "tested against AWS
CloudHSM in CI" or similar — running against a real cluster is
expensive, the hardware bench is documented in
[common-findings.md](common-findings.md) as a hardware-in-the-loop
follow-up, and the SAS-SM auditor will run their own cluster
acceptance test as part of accreditation.

If your bench surfaces a quirk on top of the docs above, the
right response is:
1. Report it in a GitHub issue with the vendor + lib version.
2. Submit a focused PR that either documents the quirk here or
   adds a vendor-specific code path under
   `services/hsm-broker/internal/backend/<vendor>/`.

## Cross-references

- [ADR 0003 — PKCS#11 abstraction](../architecture/adr/0003-pkcs11-abstraction.md)
- [Key ceremony](key-ceremony.md) — the activation procedure each vendor's cluster needs after Terraform brings it up
- [Common audit findings](common-findings.md) — including the hardware-in-the-loop bench follow-up
- [`services/hsm-broker/README.md`](https://github.com/ajamous/aether/blob/main/services/hsm-broker/README.md) — broker design and SoftHSM CI integration
- [Reference AWS](reference-aws.md) / [Reference GCP](reference-gcp.md) / [Reference Azure](reference-azure.md) / [Reference on-prem](reference-onprem.md) — surrounding topology per cloud
