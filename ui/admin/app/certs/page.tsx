import { Shell } from '@/components/Shell';
import { Card, Empty } from '@/components/Card';
import { fetchCerts, fetchCertmgrHealth } from '@/lib/api';

export default async function CertsPage() {
  const [certs, health] = await Promise.all([fetchCerts(), fetchCertmgrHealth()]);

  return (
    <Shell>
      <h1 className="text-2xl font-semibold mb-2">Certificates</h1>
      {health && (
        <p className="text-sm text-zinc-500 mb-6">
          Mode: <span className="font-mono">{health.mode}</span> · Trust store:{' '}
          {health.trust_store_size} root{health.trust_store_size === 1 ? '' : 's'}
        </p>
      )}

      <Card title="Identity certificates">
        {!certs ? (
          <Empty message="certmgr unreachable" />
        ) : certs.length === 0 ? (
          <Empty message="No identity certificates loaded." />
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-zinc-500 dark:text-zinc-400">
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="py-2 pr-4 font-medium">Name</th>
                <th className="py-2 pr-4 font-medium">Subject</th>
                <th className="py-2 pr-4 font-medium">Issuer</th>
                <th className="py-2 pr-4 font-medium">Expires in</th>
              </tr>
            </thead>
            <tbody className="font-mono">
              {certs.map((c) => (
                <tr
                  key={c.name}
                  className="border-b border-zinc-100 dark:border-zinc-800/50 last:border-0"
                >
                  <td className="py-2 pr-4">{c.name}</td>
                  <td className="py-2 pr-4 text-xs">{c.subject}</td>
                  <td className="py-2 pr-4 text-xs">{c.issuer}</td>
                  <td className="py-2 pr-4">
                    <span
                      className={
                        c.days_until_expiry < 30
                          ? 'text-amber-600 dark:text-amber-400'
                          : ''
                      }
                    >
                      {c.days_until_expiry} days
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card title="What's missing">
        <ul className="text-sm list-disc list-inside text-zinc-600 dark:text-zinc-300 space-y-1">
          <li>Cert rotation (in-UI) — pending hsm-broker SoftHSM integration</li>
          <li>OCSP responder integration</li>
          <li>Trust store editor</li>
        </ul>
      </Card>
    </Shell>
  );
}
