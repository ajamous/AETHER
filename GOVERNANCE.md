# Governance

This document describes how Aether makes decisions, who is responsible for
what, and what the project commits to (and refuses to do) over its first
24 months of public life.

## License and ownership

- License: Apache 2.0
- Trademark: held in trust by a neutral foundation (target: LF Networking)
- Contribution model: DCO sign-off, no CLA
- Copyright: each contributor retains copyright in their own contribution

## Decision-making

Day-to-day decisions are made by lazy consensus on pull requests and
issues. If a maintainer reviews a change and approves, and no other
maintainer objects within a reasonable window (typically 72 hours for
non-trivial changes), the change is accepted.

For larger architectural choices we use Architectural Decision Records
(ADRs) under `docs/architecture/adr/`. An ADR is proposed via a PR. If
maintainers reach consensus, it is merged as `Accepted`. If they don't,
the TSC decides.

## Technical Steering Committee (TSC)

Once we have at least three maintainers from at least two organizations,
we form a TSC of 5 to 7 members. TSC composition rules:

- No more than two members from any single company
- One seat reserved for an academic or independent contributor
- Term: 12 months, renewable
- Decisions by simple majority; ties broken by the chair

Until the TSC exists, the project is run by the maintainers listed in
[MAINTAINERS.md](MAINTAINERS.md) by lazy consensus.

## Maintainer process

- Maintainers are recognized by being added to `MAINTAINERS.md`
- New maintainers are nominated by an existing maintainer based on a
  sustained track record of high-quality contributions and good judgment
  on issues and PRs
- Existing maintainers are expected to stay active; an inactive maintainer
  (no contribution or review for 6 months) is moved to "emeritus" status
  with thanks

## What this project commits to

These are not aspirational. They are binding for the first 24 months.

1. **Apache 2.0, all of it, all the time.** No GPL deps, no closed-source
   modules, no relicensing.
2. **No commercial entity.** No Aether Inc. No founder-controlled company.
   The trademark is held in trust by a neutral steward.
3. **No Enterprise edition, no Cloud edition, no Pro features.** Everything
   ships in one repo. If a feature is valuable, it is valuable to the OSS.
4. **No paid support tier.** Independent consultants who help adopters
   deploy Aether are encouraged and welcome to list themselves in a
   community directory. We do not gatekeep or certify them.
5. **No CLA.** DCO sign-off is sufficient.
6. **No "Aether-certified" partner program.** No paid badges, no tiers.
7. **No closed-source plugins endorsed by the project.** A plugin API
   exists; what people build with it is their business; the project does
   not bless or endorse closed plugins.
8. **No promises about Tier-1 carrier adoption timelines.** We don't know
   when or if it will happen, and pretending we do is the kind of
   marketing language we will not tolerate in the repo.
9. **No fundraising under the project name.** Individual contributors
   can do whatever they want with their own consulting; the project
   itself does not accept commercial sponsorships beyond infrastructure
   (CI minutes, hosting, conference booths).
10. **All SAS-SM templates, deployment guides, and audit evidence
    examples ship in the OSS.** Free, no gatekeeping. The MVNO that
    walks into an audit using these and passes is the marketing.

After 24 months, if the OSS is thriving and there is unprompted
commercial demand, the community decides together — through a public
RFC process — how (or whether) to structure a commercial layer. The
default is "no, keep it pure." Anyone proposing change carries the
burden of proof.

## Release cadence

- Quarterly minor releases
- Yearly major releases
- Security patches on demand
- LTS: last two major releases get security fixes for 18 months

Releases are named after famous communications-era physicists and
engineers (v0.1 *Marconi*, v1.0 *Shannon*, v2.0 *Hertz*, v3.0 *Maxwell*).

## Code of conduct enforcement

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Reports go to the maintainers
listed in `MAINTAINERS.md`. Maintainers will recuse from any case where
they have a conflict of interest.

## Changing this document

This document is changed by the same RFC process used for ADRs. Open a
PR, get consensus from maintainers, merge. The 24-month commitments in
"What this project commits to" are deliberately hard to change — they
require unanimous TSC approval (or, before the TSC exists, unanimous
maintainer approval) and a 30-day public comment period.
