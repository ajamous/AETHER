import { Shell } from '@/components/Shell';
import { Card, Empty } from '@/components/Card';
import { fetchIoTDevices } from '@/lib/api';

export default async function EIMPage() {
  const data = await fetchIoTDevices();

  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-2">IoT devices (eIM)</h1>
      <p className="text-sm text-zinc-500 mb-6">
        Registered IoT devices and their last seen time. SGP.32 §eIM.
      </p>

      <Card title={`Registered devices${data ? ` (${data.length})` : ''}`}>
        {!data ? (
          <Empty message="eim unreachable via gateway" />
        ) : data.devices.length === 0 ? (
          <Empty message="No devices registered. POST one to /v1/devices on the eim service." />
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-zinc-500 dark:text-zinc-400">
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="py-2 pr-4 font-medium">EID</th>
                <th className="py-2 pr-4 font-medium">Label</th>
                <th className="py-2 pr-4 font-medium">Tags</th>
                <th className="py-2 pr-4 font-medium">Registered</th>
                <th className="py-2 pr-4 font-medium">Last seen</th>
              </tr>
            </thead>
            <tbody className="font-mono">
              {data.devices.map((d) => (
                <tr key={d.eid} className="border-b border-zinc-100 dark:border-zinc-800/50">
                  <td className="py-2 pr-4 text-xs">{d.eid}</td>
                  <td className="py-2 pr-4 text-xs">{d.label || '—'}</td>
                  <td className="py-2 pr-4 text-xs">{(d.tags || []).join(', ') || '—'}</td>
                  <td className="py-2 pr-4 text-xs">
                    {new Date(d.registered_at).toISOString()}
                  </td>
                  <td className="py-2 pr-4 text-xs">
                    {d.last_seen ? new Date(d.last_seen).toISOString() : 'never'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card title="What's missing">
        <ul className="text-sm list-disc list-inside text-zinc-600 dark:text-zinc-300 space-y-1">
          <li>Per-device command queue view + UI to enqueue commands</li>
          <li>IPAe (indirect) profile flow</li>
          <li>Authenticated transport between eIM and IPA (mTLS / signed commands)</li>
          <li>Bulk operations</li>
        </ul>
      </Card>
    </Shell>
  );
}
