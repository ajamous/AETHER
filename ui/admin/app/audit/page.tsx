import type { Metadata } from 'next';
import { Shell } from '@/components/Shell';
import {
  Card,
  Empty,
  Mono,
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
import { AlertIcon, CertIcon } from '@/components/icons';
import { fetchAuditEntries, fetchAuditVerify } from '@/lib/api';
import { shortHash, summarizePayload } from '@/lib/format';

export const metadata: Metadata = { title: 'Audit log' };

const SHOWN = 50;

export default async function AuditPage() {
  const [list, verify] = await Promise.all([fetchAuditEntries(0), fetchAuditVerify()]);
  const now = Date.now();
  const entries = list ? list.entries.slice().reverse().slice(0, SHOWN) : [];

  return (
    <Shell>
      <PageHeader
        title="Audit log"
        description="Append-only, hash-chained ledger. Each entry commits to the hash of the one before it."
      />

      {!verify ? (
        <div className="mb-6 rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-ink-900">
          <Unreachable service="the audit service" />
        </div>
      ) : verify.ok ? (
        <div className="mb-6 flex items-start gap-3 rounded-xl border border-emerald-200 bg-emerald-50 px-5 py-4 dark:border-emerald-500/25 dark:bg-emerald-500/10">
          <CertIcon className="mt-0.5 size-5 shrink-0 text-emerald-600 dark:text-emerald-400" />
          <div>
            <p className="text-sm font-semibold text-emerald-900 dark:text-emerald-200">
              Chain verified
            </p>
            <p className="mt-0.5 text-sm text-emerald-800/80 dark:text-emerald-200/70">
              All {verify.length} entries re-hash to the stored values and link to their
              predecessor.
            </p>
          </div>
        </div>
      ) : (
        <div
          role="alert"
          className="mb-6 flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 px-5 py-4 dark:border-red-500/30 dark:bg-red-500/10"
        >
          <AlertIcon className="mt-0.5 size-5 shrink-0 text-red-600 dark:text-red-400" />
          <div>
            <p className="text-sm font-semibold text-red-900 dark:text-red-200">
              Chain broken at entry #{verify.failed_at_seq}
            </p>
            <p className="mt-0.5 text-sm text-red-800/80 dark:text-red-200/70">{verify.reason}</p>
          </div>
        </div>
      )}

      <Card
        title="Recent events"
        description={
          list
            ? list.entries.length > SHOWN
              ? `Newest ${SHOWN} of ${list.entries.length}`
              : `${list.entries.length} total`
            : undefined
        }
        flush
      >
        {!list ? (
          <Unreachable service="the audit service" />
        ) : entries.length === 0 ? (
          <Empty
            title="No audit events yet"
            hint={
              <>
                Services append with <code>POST /v1/events</code> on the audit service.
              </>
            }
          />
        ) : (
          <Table label="Audit events">
            <THead>
              <Th className="w-16">Seq</Th>
              <Th>Event</Th>
              <Th>When</Th>
              <Th>Hash</Th>
            </THead>
            <TBody>
              {entries.map((e) => {
                const { kind, detail } = summarizePayload(e.payload);
                return (
                  <Tr key={e.seq}>
                    <Td>
                      <Mono muted>#{e.seq}</Mono>
                    </Td>
                    <Td>
                      {kind && (
                        <div className="font-medium text-zinc-900 dark:text-zinc-100">{kind}</div>
                      )}
                      <div
                        className="max-w-xl truncate font-mono text-xs text-zinc-500 dark:text-zinc-400"
                        title={JSON.stringify(e.payload)}
                      >
                        {detail}
                      </div>
                    </Td>
                    <Td className="text-zinc-600 dark:text-zinc-400">
                      <Time iso={e.timestamp} now={now} />
                    </Td>
                    <Td>
                      <Mono muted className="text-xs">
                        <span title={e.hash}>{shortHash(e.hash)}</span>
                      </Mono>
                    </Td>
                  </Tr>
                );
              })}
            </TBody>
          </Table>
        )}
      </Card>
    </Shell>
  );
}
