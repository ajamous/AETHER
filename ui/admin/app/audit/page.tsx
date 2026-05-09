import { Shell } from '@/components/Shell';
import { Card, StatusDot, Empty } from '@/components/Card';
import { fetchAuditEntries, fetchAuditVerify } from '@/lib/api';

export default async function AuditPage() {
  const [list, verify] = await Promise.all([fetchAuditEntries(0), fetchAuditVerify()]);

  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-6">Audit log</h1>

      <Card title="Chain integrity">
        {verify ? (
          <p className="text-sm">
            <StatusDot ok={verify.ok} />
            {verify.ok
              ? `Chain OK · ${verify.length} entries`
              : `Chain broken at seq ${verify.failed_at_seq}: ${verify.reason}`}
          </p>
        ) : (
          <Empty message="audit service unreachable" />
        )}
      </Card>

      <Card title="Recent events" footer="Newest at the top. Hash chain links each entry to the previous.">
        {!list ? (
          <Empty message="audit service unreachable" />
        ) : list.entries.length === 0 ? (
          <Empty message="No audit events yet. POST one to /v1/events on the audit service." />
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-zinc-500 dark:text-zinc-400">
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="py-2 pr-4 font-medium">Seq</th>
                <th className="py-2 pr-4 font-medium">Timestamp</th>
                <th className="py-2 pr-4 font-medium">Payload</th>
                <th className="py-2 pr-4 font-medium">Hash</th>
              </tr>
            </thead>
            <tbody className="font-mono">
              {list.entries
                .slice()
                .reverse()
                .slice(0, 50)
                .map((e) => (
                  <tr
                    key={e.seq}
                    className="border-b border-zinc-100 dark:border-zinc-800/50"
                  >
                    <td className="py-2 pr-4">{e.seq}</td>
                    <td className="py-2 pr-4 text-xs">
                      {new Date(e.timestamp).toISOString()}
                    </td>
                    <td className="py-2 pr-4 text-xs">
                      <code>{JSON.stringify(e.payload).slice(0, 80)}</code>
                    </td>
                    <td className="py-2 pr-4 text-xs text-zinc-500" title={e.hash}>
                      {String(e.hash).slice(0, 16)}…
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        )}
      </Card>
    </Shell>
  );
}
