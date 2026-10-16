import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type ProgressEvent, type Run, type RunSummary } from "../api";
import { number, people, shortTime } from "../vocabulary";

// The stages recon.Run reports, in order. Shown only while a run is in progress.
const stages = [
  { key: "load", label: "Load entries" },
  { key: "match", label: "Match" },
  { key: "classify", label: "Classify breaks" },
  { key: "save", label: "Save with audit trail" },
  { key: "done", label: "Done" },
];

type Props = {
  runs: Run[];
  run?: Run;
  onSelectRun: (id: number) => void;
  actor: string;
  onActor: (a: string) => void;
};

export function RunBar({ runs, run, onSelectRun, actor, onActor }: Props) {
  const client = useQueryClient();
  const [progress, setProgress] = useState<ProgressEvent[] | null>(null);
  const [error, setError] = useState("");

  const start = async () => {
    setError("");
    setProgress([]);
    try {
      const job = await api.startRun();
      const source = new EventSource(job.events);
      const seen: ProgressEvent[] = [];
      const onEvent = (e: MessageEvent) => {
        const ev = JSON.parse(e.data) as ProgressEvent;
        seen.push(ev);
        setProgress([...seen]);
        if (ev.stage === "summary" || ev.stage === "failed") {
          source.close();
          if (ev.stage === "failed") {
            setError(`The run failed: ${ev.message}`);
          } else {
            const summary = JSON.parse(ev.message) as RunSummary;
            client.invalidateQueries({ queryKey: ["runs"] }).then(() => onSelectRun(summary.run_id));
            setTimeout(() => setProgress(null), 2500);
          }
        }
      };
      for (const s of [...stages.map((s) => s.key), "summary", "failed"]) source.addEventListener(s, onEvent);
      source.onerror = () => {
        source.close();
        setError("Lost contact with the server while the run was in progress. Reload to see whether it finished.");
      };
    } catch (e) {
      setProgress(null);
      setError(e instanceof ApiError ? capitalise(e.message) + "." : "The run could not be started.");
    }
  };

  const reached = new Set(progress?.map((p) => p.stage));
  const current = progress?.at(-1);

  return (
    <header className="border-b border-rule bg-sheet">
      <div className="flex flex-wrap items-center gap-x-8 gap-y-3 px-6 py-3">
        <span className="text-lg font-bold tracking-tight">APEX</span>

        {runs.length > 0 && (
          <label className="flex items-center gap-2 text-sm">
            <span className="text-ink-soft">Showing</span>
            <select
              className="rounded border border-rule bg-sheet px-2 py-1"
              value={run?.id}
              onChange={(e) => onSelectRun(Number(e.target.value))}
            >
              {runs.map((r) => (
                <option key={r.id} value={r.id}>
                  Run {r.id}, {shortTime(r.started_at)}
                  {r.stats && "ledger_entries" in r.stats
                    ? `, ${number.format(r.stats.ledger_entries + r.stats.custodian_entries)} entries`
                    : ""}
                </option>
              ))}
            </select>
          </label>
        )}

        <div className="ml-auto flex items-center gap-6">
          <label className="flex items-center gap-2 text-sm">
            <span className="text-ink-soft">Acting as</span>
            <select
              className="rounded border border-rule bg-sheet px-2 py-1"
              value={actor}
              onChange={(e) => onActor(e.target.value)}
            >
              {people.map((p) => (
                <option key={p}>{p}</option>
              ))}
            </select>
          </label>
          <button
            type="button"
            onClick={start}
            disabled={progress !== null}
            className="rounded bg-ink px-4 py-1.5 text-sm font-semibold text-sheet disabled:opacity-50"
          >
            Run reconciliation
          </button>
        </div>
      </div>

      {(progress || error) && (
        <div className="border-t border-rule-soft px-6 py-3" aria-live="polite">
          {progress && (
            <ol className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
              {stages.map((s, i) => {
                const done = reached.has(s.key) && (current?.stage !== s.key || s.key === "done");
                const active = current?.stage === s.key && s.key !== "done";
                return (
                  <li key={s.key} className="flex items-center gap-2">
                    <span
                      className={`grid size-5 place-items-center rounded-full text-[11px] font-semibold ${
                        done ? "bg-settled text-sheet" : active ? "bg-ledger text-sheet" : "border border-rule text-ink-soft"
                      }`}
                    >
                      {i + 1}
                    </span>
                    <span className={active ? "font-semibold" : done ? "" : "text-ink-soft"}>{s.label}</span>
                  </li>
                );
              })}
            </ol>
          )}
          {current && current.stage !== "summary" && <p className="mt-1 text-sm text-ink-soft">{current.message}</p>}
          {error && <p className="text-sm text-diff">{error}</p>}
        </div>
      )}
    </header>
  );
}

function capitalise(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
