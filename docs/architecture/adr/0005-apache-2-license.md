# 0005. Apache 2.0 license, no CLA

- Status: Accepted
- Date: 2026-05-09
- Authors: Ameed Jamous <a.jamous@telecomsxchange.com>

## Context

The license choice for Aether is consequential. The audience includes:

- Independent engineers contributing on weekends
- MVNOs and small carriers wanting to deploy in production
- Larger carriers that may want to embed Aether into a wider stack
- Vendors who may want to ship Aether-based products

The choice has to balance three concerns:

1. Maximum approachability. Telecom is conservative about licensing.
   Anything that smells like GPL infection scares enterprise lawyers.
2. Patent protection. The RSP space has a complex patent landscape;
   contributors and adopters need explicit grants.
3. Trust. If contributors believe their work might be used to power a
   future commercial spin-off they didn't agree to, they won't
   contribute.

## Decision

Aether is licensed under **Apache License, Version 2.0**.

Contributions are accepted under the **Developer Certificate of Origin**
(`Signed-off-by:` trailer on every commit). **No CLA.**

## Alternatives considered

**MIT.** Permissive, simple, but has no explicit patent grant. For a
project that touches GSMA-controlled IP and vendor patent portfolios,
the explicit Apache 2.0 patent grant is non-negotiable.

**GPL v3 / AGPL.** Would scare a large fraction of the target
audience. Telecom procurement organizations have hard rules against
GPL-family code in production. We want adoption, not ideological
purity.

**MPL 2.0.** Reasonable middle ground but uncommon in telecom; would
add a lawyer-question for every adopter for no real benefit over
Apache 2.0.

**Apache 2.0 with a CLA.** A CLA centralizes ownership in some
entity. There is no entity. There will not be one for at least 24
months (see GOVERNANCE.md). A CLA implies someone could relicense the
code in the future. We commit publicly and bindingly that no one will,
and we back that up by not collecting the rights to do so. DCO
sign-off provides the legal certainty needed without the centralizing
side effect.

**Apache 2.0 + DCO + future Apache Software Foundation donation.**
A possible long-term landing pad if scale demands it. We do not
preclude this; we just do not require it.

## Consequences

Positive:

- Maximum adoptability across both OSS-friendly and OSS-cautious
  organizations
- Explicit patent grant from contributors and to recipients
- Compatible with most other permissive licenses (MIT, BSD)
- DCO sign-off is a 30-second contributor cost vs CLA's days-of-legal-
  review cost
- Aligns with CNCF / LF Networking conventions, smoothing future
  donation if the community ever wants that

Negative:

- Vendors can fork and ship closed-source derivatives. We accept this
  as the cost of being permissive. Our defense is being so good at
  being open that adoption flows through us, not around us.
- Without a CLA, future relicensing requires every contributor's
  consent. This is a feature, not a bug — see GOVERNANCE.md.

Neutral:

- Trademark protection is separate from license. The name "Aether"
  is held in trust, with usage guidelines. Permissive license,
  protected mark.

## References

- [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0)
- [Developer Certificate of Origin](https://developercertificate.org/)
- [Why Apache 2.0 is preferred for new projects](https://opensource.google/documentation/reference/patching#third_party_licenses)
- GOVERNANCE.md commitments on no-commercial-entity
