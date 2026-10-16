import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type BreakDetail as Detail, type BreakStatus, type Entry } from "../api";
import { categoryHelp, categoryName, shortTime, statusName } from "../vocabulary";

type Action = { label: string; to: BreakStatus; needsReason?: boolean; quiet?: boolean };

// What an analyst can do from each status. The database enforces the same state machine and
// the two-person rule; this only decides which buttons to offer.
function actionsFor(b: Detail, actor: string): Action[] {
  switch (b.status) {
    case "OPEN":
      return [{ label: "Assign to me", to: "ASSIGNED" }, { label: "Close as cleared", to: "CLOSED", needsReason: true, quiet: true }];
    case "ASSIGNED":
      return [
        { label: "Propose resolution", to: "PROPOSED", needsReason: true },
        ...(b.status_by !== actor ? [{ label: "Take over", to: "ASSIGNED" as BreakStatus, quiet: true }] : []),
        { label: "Close as cleared", to: "CLOSED", needsReason: true, quiet: true },
      ];
    case "PROPOSED":
      return [{ label: "Approve", to: "APPROVED" }, { label: "Send back", to: "REJECTED", needsReason: true, quiet: true }];
    case "REJECTED":
      return [{ label: "Assign to me", to: "ASSIGNED" }];
    case "APPROVED":
      return [{ label: "Close", to: "CLOSED" }];
    default:
      return [];
  }
}

type Props = { id: number; actor: string; onClose: () => void };

export function BreakDetail({ id, actor, onClose }: Props) {
  const client = useQueryClient();
  const detail = useQuery({ queryKey: ["break", id], queryFn: () => api.breakDetail(id) });
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState<Action | null>(null);
  const closeButton = useRef<HTMLButtonElement>(null);

  const act = useMutation({
    mutationFn: (a: Action) => api.addEvent(id, actor, a.to, reason),
    onSuccess: () => {
      setReason("");
      setPending(null);
      client.invalidateQueries({ queryKey: ["break", id] });
      client.invalidateQueries({ queryKey: ["breaks"] });
      client.invalidateQueries({ queryKey: ["counts"] });
    },
  });

  useEffect(() => {
    closeButton.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    setPending(null);
    setReason("");
    act.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const b = detail.data;
  const ledger = b?.entries.filter((e) => e.side === "LEDGER") ?? [];
  const custodian = b?.entries.filter((e) => e.side === "CUSTODIAN") ?? [];

  return (
    <div className="fixed inset-0 z-10 flex justify-end bg-ink/20" onClick={onClose}>
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="break-title"
        onClick={(e) => e.stopPropagation()}
        className="h-full w-full max-w-3xl overflow-y-auto border-l border-rule bg-sheet shadow-[-12px_0_32px_-24px_rgba(29,42,54,0.6)]"
      >
        <div className="flex items-start justify-between gap-4 border-b border-rule px-6 py-4">
          <div>
            <h2 id="break-title" className="text-xl font-semibold">
              {b ? categoryName[b.category] ?? b.category : "Loading break"}
            </h2>
            {b && (
              <p className="text-sm text-ink-soft">
                Break {b.id} in account {b.account_id}, {statusName[b.status].toLowerCase()}
              </p>
            )}
          </div>
          <button ref={closeButton} type="button" onClick={onClose} className="rounded px-2 py-1 text-sm text-ink-soft hover:bg-paper">
            Close panel
          </button>
        </div>

        {detail.isError && <p className="px-6 py-4 text-diff">This break could not be loaded. It may belong to a deleted run.</p>}

        {b && (
          <div className="space-y-8 px-6 py-6">
            <p className="max-w-prose text-[1.05rem] leading-relaxed">{capitalise(b.reason)}.</p>

            <TAccount ledger={ledger} custodian={custodian} difference={b.amount} differs={b.amount_minor !== 0} />
            {categoryHelp[b.category] && <p className="-mt-5 text-sm text-ink-soft">{categoryHelp[b.category]}</p>}

            <section aria-labelledby="actions-title">
              <h3 id="actions-title" className="font-semibold">
                {b.status === "CLOSED" ? "This break is closed" : "Next step"}
              </h3>
              {b.status === "PROPOSED" && b.status_by === actor && (
                <p className="mt-1 text-sm text-ink-soft">
                  You proposed this resolution, so someone else has to approve it. The database refuses
                  self-approval whichever client asks.
                </p>
              )}
              <div className="mt-3 flex flex-wrap gap-2">
                {actionsFor(b, actor).map((a) => (
                  <button
                    key={a.label}
                    type="button"
                    disabled={act.isPending}
                    onClick={() => (a.needsReason ? setPending(a) : act.mutate(a))}
                    className={
                      a.quiet
                        ? "rounded border border-rule px-3 py-1.5 text-sm hover:bg-paper"
                        : "rounded bg-ink px-3 py-1.5 text-sm font-semibold text-sheet disabled:opacity-50"
                    }
                  >
                    {a.label}
                  </button>
                ))}
              </div>
              {pending && (
                <form
                  className="mt-3 max-w-prose"
                  onSubmit={(e) => {
                    e.preventDefault();
                    act.mutate(pending);
                  }}
                >
                  <label className="block text-sm">
                    <span className="text-ink-soft">
                      {pending.to === "PROPOSED" ? "What should be done, and why?" : "Why?"} This goes into the audit trail.
                    </span>
                    <textarea
                      required
                      autoFocus
                      value={reason}
                      onChange={(e) => setReason(e.target.value)}
                      rows={3}
                      className="mt-1 block w-full rounded border border-rule bg-sheet px-2 py-1.5"
                    />
                  </label>
                  <div className="mt-2 flex gap-2">
                    <button type="submit" disabled={act.isPending} className="rounded bg-ink px-3 py-1.5 text-sm font-semibold text-sheet">
                      {pending.label}
                    </button>
                    <button type="button" onClick={() => setPending(null)} className="rounded px-3 py-1.5 text-sm text-ink-soft">
                      Cancel
                    </button>
                  </div>
                </form>
              )}
              {act.isError && (
                <p role="alert" className="mt-3 rounded bg-diff-wash px-3 py-2 text-sm text-diff">
                  Refused: {act.error instanceof ApiError ? act.error.message : "the server did not respond"}.
                </p>
              )}
            </section>

            <section aria-labelledby="history-title">
              <h3 id="history-title" className="font-semibold">Audit trail</h3>
              <p className="text-sm text-ink-soft">Append-only. Nothing here can be edited or deleted.</p>
              <ol className="mt-3 border-l border-rule pl-5">
                {b.history.map((e) => (
                  <li key={e.id} className="relative pb-4 last:pb-0">
                    <span
                      className={`absolute -left-[1.6rem] top-1.5 size-2.5 rounded-full ${
                        e.status === "CLOSED" || e.status === "APPROVED" ? "bg-settled" : e.status === "REJECTED" ? "bg-diff" : "bg-ledger"
                      }`}
                    />
                    <p className="text-sm">
                      <span className="font-semibold">{statusName[e.status]}</span> by {e.actor}
                      <span className="text-ink-soft">, {shortTime(e.created_at)}</span>
                    </p>
                    {e.reason && e.actor !== "engine" && <p className="text-sm text-ink-soft">{e.reason}</p>}
                  </li>
                ))}
              </ol>
            </section>
          </div>
        )}
      </section>
    </div>
  );
}

// The T-account: what the ledger holds on the left, what the custodian reported on the right,
// and the difference between them in red ink in the gutter.
function TAccount({ ledger, custodian, difference, differs }: { ledger: Entry[]; custodian: Entry[]; difference: string; differs: boolean }) {
  return (
    <div className="grid grid-cols-1 overflow-hidden rounded-md border border-rule md:grid-cols-[1fr_9rem_1fr]">
      <Column title="Ledger" tone="ledger" entries={ledger} empty="The ledger has no entry for this." />
      <div
        className={`flex flex-col items-center justify-center gap-1 border-y border-rule px-3 py-4 text-center md:border-x md:border-y-0 ${
          differs ? "bg-diff-wash" : "bg-paper"
        }`}
      >
        <span className="text-xs text-ink-soft">Ledger minus custodian</span>
        <span className={`figures text-lg font-semibold leading-tight ${differs ? "text-diff" : "text-ink-soft"}`}>{difference}</span>
      </div>
      <Column title="Custodian" tone="custodian" entries={custodian} empty="The custodian reported nothing for this." />
    </div>
  );
}

function Column({ title, tone, entries, empty }: { title: string; tone: "ledger" | "custodian"; entries: Entry[]; empty: string }) {
  const head = tone === "ledger" ? "bg-ledger-wash text-ledger" : "bg-custodian-wash text-custodian";
  return (
    <div className="min-w-0">
      <h4 className={`px-4 py-2 text-sm font-semibold ${head}`}>{title}</h4>
      {entries.length === 0 ? (
        <p className="px-4 py-4 text-sm text-ink-soft">{empty}</p>
      ) : (
        <ul className="divide-y divide-rule-soft">
          {entries.map((e) => (
            <li key={e.entry_id} className="px-4 py-3 text-sm">
              <p className="figures text-base font-semibold">{e.amount}</p>
              <p className="text-ink-soft">
                Value date <span className="figures">{e.value_date}</span>
              </p>
              <p className="mt-1 break-words">{e.reference || <span className="text-ink-soft">No reference</span>}</p>
              <p className="text-xs text-ink-soft">{e.external_id}</p>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function capitalise(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
