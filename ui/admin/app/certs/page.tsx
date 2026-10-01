import type { Metadata } from 'next';
import { Shell } from '@/components/Shell';
import {
  Badge,
  Card,
  Empty,
  Mono,
  NotYet,
  PageHeader,
  Table,
  TBody,
  Td,
  Th,
  THead,
  Tr,
  Unreachable,
} from '@/components/ui';
import { fetchCertmgrHealth, fetchCerts } from '@/lib/api';
import { absoluteTime, commonName, expiryLabel, expiryTone } from '@/lib/format';

export const metadata: Metadata = { title: 'Certificates' };

export default async function CertsPage() {
  const [certs, health] = await Promise.all([fetchCerts(), fetchCertmgrHealth()]);
  const sorted = certs
    ? [...certs].sort((a, b) => a.days_until_expiry - b.days_until_expiry)
    : null;

  return (
    <Shell>
      <PageHeader
        title="Certificates"
        description="Identity certificates loaded by the certificate manager, soonest expiry first."
        meta={
          health && (
            <>
              <Badge tone={health.mode === 'lab' ? 'warn' : 'info'} dot>
                {health.mode === 'lab' ? 'Lab mode · test certificates' : 'Production mode'}
              </Badge>
              <Badge>
                Trust store: {health.trust_store_size} root
                {health.trust_store_size === 1 ? '' : 's'}
              </Badge>
            </>
          )
        }
      />

      <Card
        title="Identity certificates"
        description={sorted ? `${sorted.length} loaded` : undefined}
        flush
      >
        {!sorted ? (
          <Unreachable service="the certificate manager" />
        ) : sorted.length === 0 ? (
          <Empty
            title="No identity certificates loaded"
            hint={
              <>
                certmgr loads them from <code>--dp-tls-cert</code>, <code>--dp-auth-cert</code> and{' '}
                <code>--dp-pb-cert</code>. For a lab chain, run{' '}
                <code>certmgr --generate-lab &lt;dir&gt;</code> first.
              </>
            }
          />
        ) : (
          <Table label="Identity certificates">
            <THead>
              <Th>Name</Th>
              <Th>Expires</Th>
              <Th>Subject</Th>
              <Th>Issuer</Th>
              <Th>Serial</Th>
            </THead>
            <TBody>
              {sorted.map((c) => (
                <Tr key={c.name}>
                  <Td className="font-medium text-zinc-900 dark:text-zinc-100">{c.name}</Td>
                  <Td>
                    <Badge tone={expiryTone(c.days_until_expiry)} dot>
                      {expiryLabel(c.days_until_expiry)}
                    </Badge>
                    <div className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">
                      {absoluteTime(c.not_after).slice(0, 10)}
                    </div>
                  </Td>
                  <Td>
                    <div className="text-zinc-800 dark:text-zinc-200">{commonName(c.subject)}</div>
                    <div
                      className="mt-0.5 max-w-xs truncate text-xs text-zinc-500 dark:text-zinc-400"
                      title={c.subject}
                    >
                      {c.subject}
                    </div>
                  </Td>
                  <Td>
                    <div className="text-zinc-800 dark:text-zinc-200" title={c.issuer}>
                      {commonName(c.issuer)}
                    </div>
                  </Td>
                  <Td>
                    <Mono className="text-xs">{c.serial_number}</Mono>
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Card>

      <div className="mt-6">
        <NotYet
          items={[
            'Certificate rotation from the console',
            'OCSP / revocation status',
            'Trust store editing',
          ]}
        />
      </div>
    </Shell>
  );
}
