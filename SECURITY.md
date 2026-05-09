# Security Policy

## Reporting a vulnerability

Aether handles cryptographic key material and protocols that protect
real telecom subscribers. We take vulnerability reports seriously.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, report privately by one of:

- GitHub Security Advisory: use the "Report a vulnerability" button on
  the repository's Security tab. This is the preferred channel.
- Email: `security@aether-rsp.org` (PGP key to be published once the
  project has a hosted infrastructure footprint)

When you report, please include:

- A description of the issue and the impact you believe it has
- Steps to reproduce, or a proof-of-concept if you have one
- The version, commit hash, or branch you tested against
- Whether you have already disclosed this elsewhere

## What to expect

- Acknowledgement within 3 business days
- An initial assessment and severity rating within 10 business days
- Coordinated disclosure timeline agreed with the reporter
- Public credit in the release notes for the fix, unless you prefer
  to remain anonymous

We follow a 90-day disclosure window by default, shortened or extended
based on the severity and the reporter's preference.

## Supported versions

Once we have tagged releases, the last two minor versions of the most
recent two majors will receive security fixes for 18 months from
release. Until v0.1, only `main` is supported.

## Scope

In scope:

- All code in this repository
- The default Helm chart and Docker Compose configurations
- The reference Terraform modules

Out of scope:

- Issues in third-party HSMs, cloud KMS services, or operating systems
  themselves (please report those upstream)
- Issues that require a privileged operator with legitimate access to
  perform actions they are already authorized to perform (those are
  policy concerns, not vulnerabilities)
- Denial-of-service findings against the lab profile, which is not
  hardened for hostile networks

## Hall of fame

Reporters who help us land a fix will be listed in the release notes
and in `docs/security/researchers.md` once that page exists, with
their permission.
