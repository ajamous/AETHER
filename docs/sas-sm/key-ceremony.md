# Key ceremony procedure

The key ceremony is the moment your production HSM private keys
are generated. It is the single most scrutinised event in a SAS-SM
audit. This document gives you a procedure that satisfies the
audit's expectations and a chain-of-custody form to fill in during
the ceremony.

This procedure assumes:

- An offline workstation dedicated to the ceremony (booted from
  read-only media, no network).
- A FIPS-140-2 (or 140-3) rated HSM that exposes PKCS#11. AWS
  CloudHSM, GCP Cloud HSM, Azure Managed HSM, Thales Luna, and
  Utimaco all qualify.
- A physical secure room with restricted access logging.
- A two-person quorum for every operation that touches the HSM.
- A designated **scribe** who is *not* one of the key custodians,
  to record the ceremony as it happens.

The roles below correspond to the [RBAC template](rbac.md):

- **Key custodians (2)** — hold the physical/logical credentials
  needed to authenticate to the HSM.
- **Scribe** — writes the chain-of-custody form. Cannot be a
  custodian.
- **Witness (optional)** — auditor or compliance officer. Signs
  the form.

## Before the ceremony

A week before:

- [ ] Schedule the ceremony. Inform all attendees.
- [ ] Confirm HSM is initialised and the security officers (SO)
      can log in.
- [ ] Print three copies of the chain-of-custody form below.
- [ ] Verify the offline workstation's filesystem is clean. Wipe
      and reinstall if uncertain.
- [ ] Prepare the configuration values:
      - Slot ID
      - Key labels (`DPtls`, `DPauth`, `DPpb`)
      - Curve (`prime256v1` for SGP.22 P-256)
      - PIN policy (length, complexity)

## During the ceremony

Time-stamp every step. The scribe writes "HH:MM | step description"
on the form as each step completes.

### 1. Room access verified

Both custodians, the scribe, and any witnesses arrive together.
The room access log is verified to show every entry.

Record on form: time of arrival, who is present.

### 2. Workstation booted

Boot the offline workstation from read-only media. Confirm:

- No network interfaces up (`ip a`)
- HSM is reachable on its bus (`pkcs11-tool --module $LIB --list-slots`)

Record on form: workstation hostname, kernel version, PKCS#11
module path.

### 3. SO login

Both custodians enter their halves of the SO PIN. The HSM is
unlocked.

Record on form: time, slot label.

### 4. Generate keys

For each of `DPtls`, `DPauth`, `DPpb`:

```
pkcs11-tool --module $LIB \
            --login --pin "$USER_PIN" \
            --keypairgen --key-type EC:prime256v1 \
            --label "DPauth" \
            --id "01"
```

Or, equivalently, run the Aether broker against the same module
and call `GenerateKeyPair` per [services/hsm-broker/README.md](https://github.com/ajamous/aether/tree/main/services/hsm-broker).

For each key, record on the form:

- Label
- Curve
- Public key (full uncompressed point, hex; reading it from the
  HSM does not extract the private half)
- SHA-256 fingerprint of the DER-encoded SubjectPublicKeyInfo

### 5. CSR generation (for keys that will host CI-issued certs)

For DPtls / DPauth / DPpb that need CI-signed certs:

```
openssl req -new \
            -engine pkcs11 -keyform engine \
            -key "pkcs11:object=DPauth;type=private;pin-value=$USER_PIN" \
            -subj "/CN=Aether SM-DP+ DPauth/O=Your MVNO Ltd" \
            -out DPauth.csr
```

Record on form: SHA-256 of each CSR.

### 6. SO logout

Both custodians log out. Verify with the HSM admin tool that
the session is closed.

Record on form: time of logout.

### 7. Form signing

All present sign the chain-of-custody form. Three copies:

- One in the audit pack.
- One in the operations safe.
- One handed to the witness (if external).

## After the ceremony

- [ ] Submit the CSRs to your CI (Atos, DigiCert, etc.).
- [ ] Once the CI returns the certs, load them into certmgr via
      its `production` mode (see [ADR 0004](../architecture/adr/0004-lab-vs-prod-cert-mode.md)).
- [ ] File the chain-of-custody form copies.
- [ ] Schedule the next rotation.

## Chain-of-custody form

Cut here. Print and use.

---

```
                AETHER KEY CEREMONY — CHAIN OF CUSTODY

Ceremony ID:       ____________________________________________
Date:              ____________________________________________
Location:          ____________________________________________
HSM model + s/n:   ____________________________________________
PKCS#11 lib path:  ____________________________________________
Slot ID:           ____________________________________________

Personnel
---------
Custodian A:       ____________________________________________
                   ID:                                            
                   Signature:

Custodian B:       ____________________________________________
                   ID:                                            
                   Signature:

Scribe:            ____________________________________________
                   ID:                                            
                   Signature:

Witness:           ____________________________________________
                   Affiliation:                                  
                   Signature:

Workstation
-----------
Hostname:          ____________________________________________
Kernel version:    ____________________________________________
Boot media SHA-256:____________________________________________
Network verified down (initials of A and B):  ___ ___

Time log
--------
Arrival:           ____________________________________________
Workstation boot:  ____________________________________________
SO login:          ____________________________________________
Key generation:    ____________________________________________
CSR generation:    ____________________________________________
SO logout:         ____________________________________________
Departure:         ____________________________________________

Keys generated
--------------
Label:             ____________________________________________
Curve:             ____________________________________________
Public point (hex, full uncompressed):
____________________________________________________________________
____________________________________________________________________
SPKI SHA-256:      ____________________________________________

Label:             ____________________________________________
Curve:             ____________________________________________
Public point (hex, full uncompressed):
____________________________________________________________________
____________________________________________________________________
SPKI SHA-256:      ____________________________________________

Label:             ____________________________________________
Curve:             ____________________________________________
Public point (hex, full uncompressed):
____________________________________________________________________
____________________________________________________________________
SPKI SHA-256:      ____________________________________________

CSRs generated (file SHA-256)
-----------------------------
DPtls.csr:         ____________________________________________
DPauth.csr:        ____________________________________________
DPpb.csr:          ____________________________________________

Anomalies / deviations
----------------------
____________________________________________________________________
____________________________________________________________________
____________________________________________________________________
____________________________________________________________________

Final attestation
-----------------
We attest that the procedure documented in
docs/sas-sm/key-ceremony.md was followed without deviation
except as noted above. Private key material did not leave the
HSM. The two-person quorum was maintained throughout.

Custodian A signature: _________________________  Date: ________
Custodian B signature: _________________________  Date: ________
Scribe     signature: _________________________  Date: ________
Witness    signature: _________________________  Date: ________
```

---

## What goes in the audit pack

- One signed copy of this form per ceremony.
- The CSRs (DER + SHA-256 in the form).
- The CI-issued certs that came back.
- The certmgr config showing those certs loaded in `production`
  mode.
- The hsm-broker config showing the keys referenced by their
  PKCS#11 URIs.
- Audit log entries from `services/audit` showing the first
  Sign call against each key (proves the key works without
  exposing it).

## What does NOT go in the audit pack

- The HSM SO PIN. Ever.
- The HSM user PIN. Ever.
- Any private-key material. The platform makes this impossible
  by design (see [ADR 0003](../architecture/adr/0003-pkcs11-abstraction.md)).
- Personal identifying information beyond names and roles.
