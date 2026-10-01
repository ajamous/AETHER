import type { Metadata } from 'next';
import { Shell } from '@/components/Shell';
import {
  Badge,
  Card,
  Empty,
  MoreLink,
  Mono,
  PageHeader,
  StatTile,
  Time,
  Unreachable,
} from '@/components/ui';
import { AuditIcon, CertIcon, DashboardIcon, DeviceIcon, DiscoveryIcon } from '@/components/icons';
import {
  fetchAuditEntries,
  fetchAuditVerify,
  fetchCertmgrHealth,
  fetchCerts,
  fetchGatewayHealth,
  fetchIoTDevices,
  fetchSMDSEvents,
} from '@/lib/api';
import { commonName, expiryLabel, expiryTone, summarizePayload, type Tone } from '@/lib/format';

export const metadata: Metadata = { title: 'Dashboard' };

export default async function DashboardPage() {
  const [gw, cm, certs, audit, verify, smds, eim] = await Promise.all([
    fetchGatewayHealth(),
    fetchCertmgrHealth(),
    fetchCerts(),
    fetchAuditEntries(0),
    fetchAuditVerify(),
    fetchSMDSEvents(),
    fetchIoTDevices(),
  ]);
  const now = Date.now();

  const services: { name: string; up: boolean; detail: string }[] = [
    { name: 'Gateway', up: !!gw?.ready, detail: gw ? 'Ready' : 'Unreachable' },
    {
      name: 'Certificate manager',
      up: !!cm?.ready,
      detail: cm ? `${cm.mode === 'lab' ? 'Lab' : 'Production'} mode` : 'Unreachable',
    },
    {
      name: 'Audit ledger',
      up: !!audit && verify?.ok === true,
      detail: !audit ? 'Unreachable' : verify?.ok ? 'Chain verified' : 'Chain broken',
    },
  ];
  const upCount = services.filter((s) => s.up).length;
  const servicesTone: Tone = upCount === services.length ? 'ok' : upCount === 0 ? 'danger' : 'warn';

  const attention = (certs ?? [])
    .filter((c) => c.days_until_expiry < 30)
    .sort((a, b) => a.days_until_expiry - b.days_until_expiry);

  const recent = audit ? audit.entries.slice(-5).reverse() : [];

  return (
    <Shell>
      <PageHeader
        title="Dashboard"
        description="Health of the Aether RSP stack at a glance."
        meta={
          cm && (
            <Badge tone={cm.mode === 'lab' ? 'warn' : 'info'} dot>
              {cm.mode === 'lab' ? 'Lab mode · test certificates' : 'Production mode'}
            </Badge>
          )
        }
      />

      <div className="grid grid-cols-2 gap-3 xl:grid-cols-5 [&>*:last-child]:col-span-2 xl:[&>*:last-child]:col-span-1">
        <StatTile
          label="Services"
          icon={DashboardIcon}
          value={`${upCount}/${services.length}`}
          hint={upCount === services.length ? 'All healthy' : `${services.length - upCount} down`}
          tone={servicesTone}
        />
        <StatTile
          label="Identity certificates"
          icon={CertIcon}
          href="/certs"
          value={cm ? cm.identities : '—'}
          hint={
            !cm
              ? 'Unreachable'
              : cm.earliest_expiry_days >= 0
                ? `Next expiry ${expiryLabel(cm.earliest_expiry_days)}`
                : 'No expiry data'
          }
          tone={
            !cm
              ? 'danger'
              : cm.earliest_expiry_days >= 0
                ? expiryTone(cm.earliest_expiry_days)
                : 'neutral'
          }
        />
        <StatTile
          label="Audit entries"
          icon={AuditIcon}
          href="/audit"
          value={verify ? verify.length : '—'}
          hint={
            !verify
              ? 'Unreachable'
              : verify.ok
                ? 'Chain verified'
                : `Broken at #${verify.failed_at_seq}`
          }
          tone={!verify ? 'danger' : verify.ok ? 'ok' : 'danger'}
        />
        <StatTile
          label="Pending SM-DS events"
          icon={DiscoveryIcon}
          href="/smds"
          value={smds ? smds.length : '—'}
          hint={smds ? 'Awaiting device poll' : 'Unreachable'}
          tone={smds ? undefined : 'danger'}
        />
        <StatTile
          label="IoT devices"
          icon={DeviceIcon}
          href="/eim"
          value={eim ? eim.length : '—'}
          hint={eim ? 'Registered with eIM' : 'Unreachable'}
          tone={eim ? undefined : 'danger'}
        />
      </div>

      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <Card title="Service health" flush>
          <ul className="divide-y divide-zinc-100 dark:divide-zinc-800/70">
            {services.map((s) => (
              <li key={s.name} className="flex items-center justify-between gap-4 px-5 py-3">
                <span className="text-sm font-medium text-zinc-800 dark:text-zinc-100">
                  {s.name}
                </span>
                <Badge tone={s.up ? 'ok' : 'danger'} dot>
                  {s.detail}
                </Badge>
              </li>
            ))}
          </ul>
          {gw && Object.keys(gw.upstream).length > 0 && (
            <div className="border-t border-zinc-100 px-5 py-3 dark:border-zinc-800/70">
              <div className="mb-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400">
                Gateway routes to
              </div>
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
                {Object.entries(gw.upstream).map(([name, url]) => (
                  <div key={name} className="contents">
                    <dt className="text-zinc-500 dark:text-zinc-400">{name.replace(/_/g, '-')}</dt>
                    <dd className="truncate">
                      <Mono className="text-xs">{url}</Mono>
                    </dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
        </Card>

        <Card
          title="Certificates needing attention"
          description="Identity certificates expiring within 30 days."
          action={<MoreLink href="/certs">All certificates</MoreLink>}
          flush
        >
          {!certs ? (
            <Unreachable service="the certificate manager" />
          ) : attention.length === 0 ? (
            <Empty
              title="Nothing expiring soon"
              hint="Every identity certificate has at least 30 days left."
            />
          ) : (
            <ul className="divide-y divide-zinc-100 dark:divide-zinc-800/70">
              {attention.map((c) => (
                <li key={c.name} className="flex items-center justify-between gap-4 px-5 py-3">
                  <div className="min-w-0">
                    <div className="text-sm font-medium text-zinc-800 dark:text-zinc-100">
                      {c.name}
                    </div>
                    <div className="truncate text-xs text-zinc-500 dark:text-zinc-400">
                      {commonName(c.subject)}
                    </div>
                  </div>
                  <Badge tone={expiryTone(c.days_until_expiry)} dot>
                    {expiryLabel(c.days_until_expiry)}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card
        className="mt-6"
        title="Recent audit activity"
        action={<MoreLink href="/audit">Full audit log</MoreLink>}
        flush
      >
        {!audit ? (
          <Unreachable service="the audit service" />
        ) : recent.length === 0 ? (
          <Empty
            title="No audit events yet"
            hint="Events appear here as services write to the ledger."
          />
        ) : (
          <ul className="divide-y divide-zinc-100 dark:divide-zinc-800/70">
            {recent.map((e) => {
              const { kind, detail } = summarizePayload(e.payload);
              return (
                <li key={e.seq} className="flex items-start gap-4 px-5 py-3 text-sm">
                  <Mono muted className="w-10 shrink-0 pt-0.5 text-xs">
                    #{e.seq}
                  </Mono>
                  <div className="min-w-0 flex-1">
                    {kind && (
                      <div className="truncate font-medium text-zinc-800 dark:text-zinc-100">
                        {kind}
                      </div>
                    )}
                    <div className="truncate font-mono text-xs text-zinc-500 dark:text-zinc-400">
                      {detail}
                    </div>
                  </div>
                  <span className="shrink-0 pt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
                    <Time iso={e.timestamp} now={now} />
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </Card>
    </Shell>
  );
}
