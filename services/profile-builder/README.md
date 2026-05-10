# services/profile-builder

Generates SAIP (Subscription Manager Application for Profile) profile
packages from carrier-supplied data.

## Status

| Piece                                            | Status        |
| ------------------------------------------------ | ------------- |
| YAML profile template loader                     | Implemented   |
| Template validation (IMSI, ICCID, MSISDN, OTA)   | Implemented   |
| HTTP API: list / get / build                     | Implemented   |
| UPP (Unprotected Profile Package) generation     | Partial       |
| TCA SAIP v2.x ASN.1 codec                        | Partial — minimum-viable subset live in [`pkg/saip`](../../pkg/saip) (ProfileHeader + PEEnd, DER round-trip, AppendRaw for spare elements); richer ProfileElements (PE-USIM, PE-PinCodes, PE-FileSystem, PE-AKAParameter, …) land as the catalogue grows |
| UPP → PPP (Protected) → BPP (Bound) pipeline     | Not started — depends on the BPP wrapping landing in `services/smdp-plus` (ECKA + X9.63-SHA-256 KDF + AES-128-GCM segmentation around the SAIP UPP) |

The generator now emits a real DER-encoded SAIP ProfilePackage
in the `saip_der` field of the UPP envelope alongside the
existing JSON-shaped inputs (kept for human-readable inspection
through the admin UI). The encoding uses [`pkg/saip`](../../pkg/saip);
its current scope is ProfileHeader + PEEnd, which is enough to
produce a syntactically-valid (if narrow) profile that
syntax-validates against the SGP.22 §B grammar. Wider element
coverage lands incrementally without changing this service's
HTTP shape.
