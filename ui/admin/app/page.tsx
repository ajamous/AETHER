import { Shell } from '@/components/Shell';
import { Card, StatusDot } from '@/components/Card';
import {
  fetchGatewayHealth,
  fetchCertmgrHealth,
  fetchAuditEntries,
  fetchAuditVerify,
} from '@/lib/api';

export default async function DashboardPage() {
  const [gw, cm, audit, verify] = await Promise.all([
    fetchGatewayHealth(),
    fetchCertmgrHealth(),
    fetchAuditEntries(0),
    fetchAuditVerify(),
  ]);

  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-6">Dashboard</h1>

      <Card title="System health">
        <div className="grid grid-cols-2 md:grid-cols-3 gap-3 text-sm">
          <Status name="Gateway" ok={!!gw && gw.ready} detail={gw ? 'reachable' : 'unreachable'} />
          <Status
            name="Certificate manager"
            ok={!!cm && cm.ready}
            detail={cm ? `${cm.mode} mode · ${cm.identities} identities` : 'unreachable'}
          />
          <Status
            name="Audit log"
            ok={!!audit && verify?.ok === true}
            detail={
              audit
                ? `${audit.length} entries · ${verify?.ok ? 'chain ok' : 'chain broken'}`
                : 'unreachable'
            }
          />
        </div>
      </Card>

      <Card title="Certificate expiry">
        {cm ? (
          <div className="text-sm">
            <p>
              Mode:{' '}
              <span className="font-mono">
                {cm.mode}
                {cm.mode === 'lab' && ' (test certs only)'}
              </span>
            </p>
            <p className="mt-2">
              Earliest expiry:{' '}
              <span className="font-mono">
                {cm.earliest_expiry_days >= 0 ? `${cm.earliest_expiry_days} days` : 'n/a'}
              </span>
            </p>
            {cm.expiring_soon.length > 0 && (
              <p className="mt-2 text-amber-600 dark:text-amber-400">
                Expiring within 30 days: {cm.expiring_soon.join(', ')}
              </p>
            )}
          </div>
        ) : (
          <p className="text-sm text-zinc-500">certmgr unreachable</p>
        )}
      </Card>

      <Card title="Audit chain integrity">
        {verify ? (
          <div className="text-sm">
            <p>
              <StatusDot ok={verify.ok} />
              {verify.ok ? `Chain OK · ${verify.length} entries` : `Broken at seq ${verify.failed_at_seq}: ${verify.reason}`}
            </p>
          </div>
        ) : (
          <p className="text-sm text-zinc-500">audit unreachable</p>
        )}
      </Card>

      <Card title="What this UI is and isn't" footer="See README · Status table">
        <ul className="text-sm space-y-1 list-disc list-inside text-zinc-600 dark:text-zinc-300">
          <li>Phase 2 admin scaffold. No authentication. Lab use only.</li>
          <li>Read paths are wired to the gateway, certmgr, and audit services.</li>
          <li>Profile activation, cert rotation, and HSM admin are not yet implemented.</li>
        </ul>
      </Card>
    </Shell>
  );
}

function Status({ name, ok, detail }: { name: string; ok: boolean | null; detail: string }) {
  return (
    <div className="border border-zinc-200 dark:border-zinc-800 rounded p-3">
      <div className="font-medium">
        <StatusDot ok={ok} />
        {name}
      </div>
      <div className="text-xs text-zinc-500 mt-1">{detail}</div>
    </div>
  );
}
