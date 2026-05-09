# services/profile-builder

Generates SAIP (Subscription Manager Application for Profile) profile
packages from carrier-supplied data.

## Status

| Piece                                            | Status        |
| ------------------------------------------------ | ------------- |
| YAML profile template loader                     | Implemented   |
| Template validation (IMSI, ICCID, MSISDN, OTA)   | Implemented   |
| HTTP API: list / get / build                     | Implemented   |
| UPP (Unprotected Profile Package) generation     | Skeleton      |
| UPP → PPP (Protected) → BPP (Bound) pipeline     | Not started — depends on TCA SAIP codec |
| TCA SAIP v2.x ASN.1 codec                        | Not started   |

The generator emits a JSON envelope describing what a UPP would
contain. The actual SAIP-encoded UPP is produced by the codec under
`pkg/saip` (not yet present); landing the codec is the dedicated
follow-up that turns this service from skeleton into producer.
