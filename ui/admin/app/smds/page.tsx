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
import { fetchSMDSEvents } from '@/lib/api';

export const metadata: Metadata = { title: 'Discovery service' };

export default async function SMDSPage() {
  const data = await fetchSMDSEvents();
  const now = Date.now();

  return (
    <Shell>
      <PageHeader
        title="Discovery service"
        description="Profile events the SM-DS holds for devices until their LPA polls for them (SGP.22 §5.5)."
      />

      <Card
        title="Registered events"
        description={data ? `${data.length} pending` : undefined}
        flush
      >
        {!data ? (
          <Unreachable service="the SM-DS" hint="The console reaches it through the gateway." />
        ) : data.events.length === 0 ? (
          <Empty
            title="No pending events"
            hint={
              <>
                An SM-DP+ registers one with <code>POST /gsma/rsp2/es12/registerEvent</code> on the
                smds service.
              </>
            }
          />
        ) : (
          <Table label="Registered SM-DS events">
            <THead>
              <Th>EID</Th>
              <Th>Event ID</Th>
              <Th>SM-DP+ address</Th>
              <Th>Registered</Th>
            </THead>
            <TBody>
              {data.events.map((e) => (
                <Tr key={e.eid + e.event_id}>
                  <Td>
                    <Mono>{e.eid}</Mono>
                  </Td>
                  <Td>
                    <Mono>{e.event_id}</Mono>
                  </Td>
                  <Td>
                    <div className="flex items-center gap-2">
                      <Mono>{e.rsp_server_address}</Mono>
                      {e.forwarding && <Badge tone="info">forwarded</Badge>}
                    </div>
                  </Td>
                  <Td className="text-zinc-600 dark:text-zinc-400">
                    <Time iso={e.registered_at} now={now} />
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
            'Alternative SM-DS / cascade lookups',
            'Push notification to devices (LPAs poll today)',
            'LPA-side verification against the SM-DS identity certificate',
          ]}
        />
      </div>
    </Shell>
  );
}
