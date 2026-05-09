import { Shell } from '@/components/Shell';
import { Card, Empty } from '@/components/Card';
import { fetchSMDSEvents } from '@/lib/api';

export default async function SMDSPage() {
  const data = await fetchSMDSEvents();

  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-2">Discovery service</h1>
      <p className="text-sm text-zinc-500 mb-6">
        Pending profile events the SM-DS has registered for devices. SGP.22 §5.5.
      </p>

      <Card title={`Registered events${data ? ` (${data.length})` : ''}`}>
        {!data ? (
          <Empty message="smds unreachable via gateway" />
        ) : data.events.length === 0 ? (
          <Empty message="No pending events. Register one via POST /gsma/rsp2/es12/registerEvent on the smds service." />
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-zinc-500 dark:text-zinc-400">
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="py-2 pr-4 font-medium">EID</th>
                <th className="py-2 pr-4 font-medium">Event ID</th>
                <th className="py-2 pr-4 font-medium">SM-DP+</th>
                <th className="py-2 pr-4 font-medium">Registered</th>
              </tr>
            </thead>
            <tbody className="font-mono">
              {data.events.map((e) => (
                <tr
                  key={e.eid + e.event_id}
                  className="border-b border-zinc-100 dark:border-zinc-800/50"
                >
                  <td className="py-2 pr-4 text-xs">{e.eid}</td>
                  <td className="py-2 pr-4 text-xs">{e.event_id}</td>
                  <td className="py-2 pr-4 text-xs">{e.rsp_server_address}</td>
                  <td className="py-2 pr-4 text-xs">
                    {new Date(e.registered_at).toISOString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card title="What's missing">
        <ul className="text-sm list-disc list-inside text-zinc-600 dark:text-zinc-300 space-y-1">
          <li>Postgres-backed event persistence (in-memory today)</li>
          <li>Alternative SM-DS / cascade lookups</li>
          <li>HTTPS + mTLS</li>
          <li>Push notification channel (vs polling)</li>
        </ul>
      </Card>
    </Shell>
  );
}
