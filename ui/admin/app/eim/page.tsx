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
  Time,
  Tr,
  Unreachable,
} from '@/components/ui';
import { fetchIoTDevices } from '@/lib/api';
import { lastSeenTone } from '@/lib/format';

export const metadata: Metadata = { title: 'IoT devices' };

export default async function EIMPage() {
  const data = await fetchIoTDevices();
  const now = Date.now();

  return (
    <Shell>
      <PageHeader
        title="IoT devices"
        description="Devices registered with the eIM and when their IPA last polled for commands (SGP.32)."
      />

      <Card
        title="Registered devices"
        description={data ? `${data.length} registered` : undefined}
        flush
      >
        {!data ? (
          <Unreachable service="the eIM" hint="The console reaches it through the gateway." />
        ) : data.devices.length === 0 ? (
          <Empty
            title="No devices registered"
            hint={
              <>
                Register one with <code>POST /v1/devices</code> on the eim service.
              </>
            }
          />
        ) : (
          <Table label="Registered IoT devices">
            <THead>
              <Th>Device</Th>
              <Th>Last seen</Th>
              <Th>EID</Th>
              <Th>Tags</Th>
              <Th>Registered</Th>
            </THead>
            <TBody>
              {data.devices.map((d) => (
                <Tr key={d.eid}>
                  <Td className="font-medium text-zinc-900 dark:text-zinc-100">
                    {d.label || (
                      <span className="font-normal text-zinc-500 dark:text-zinc-400">
                        Unlabelled
                      </span>
                    )}
                  </Td>
                  <Td>
                    <Badge tone={lastSeenTone(d.last_seen, now)} dot>
                      {d.last_seen ? <Time iso={d.last_seen} now={now} /> : 'Never'}
                    </Badge>
                  </Td>
                  <Td>
                    <Mono>{d.eid}</Mono>
                  </Td>
                  <Td>
                    {d.tags && d.tags.length > 0 ? (
                      <div className="flex flex-wrap gap-1">
                        {d.tags.map((t) => (
                          <Badge key={t}>{t}</Badge>
                        ))}
                      </div>
                    ) : (
                      <span className="text-zinc-400">—</span>
                    )}
                  </Td>
                  <Td className="text-zinc-600 dark:text-zinc-400">
                    <Time iso={d.registered_at} now={now} />
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
            'Per-device command queue view, and enqueueing commands from the console',
            'IPAe (indirect) profile flow',
            'Authenticated eIM ↔ IPA transport (mTLS / signed commands)',
            'Bulk operations',
          ]}
        />
      </div>
    </Shell>
  );
}
