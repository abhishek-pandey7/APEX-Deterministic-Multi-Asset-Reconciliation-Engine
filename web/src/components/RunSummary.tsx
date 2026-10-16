import { useQuery } from "@tanstack/react-query";
import { api, type Run, type RunSummary as Summary } from "../api";
import type { Filters } from "./BreakQueue";
import { categoryHelp, categoryName, number, phaseName, statusName } from "../vocabulary";

type Props = { run: Run; summary: Summary; filters: Filters; onFilters: (f: Filters) => void };

export function RunSummary({ run, summary, filters, onFilters }: Props) {
  // Counts per status come from the live audit log, so they move as analysts work.
  const counts = useQuery({
    queryKey: ["counts", run.id],
    queryFn: () => api.breaks(run.id, { offset: 0, limit: 1 }),
  });

  const matched = Object.entries(summary.matches);
  const totalMatches = matched.reduce((n, [, v]) => n + v, 0);
  const proposed = matched.filter(([k]) => k.endsWith("/PROPOSED")).reduce((n, [, v]) => n + v, 0);
  const byPhase = new Map<string, number>();
  for (const [k, v] of matched) {
    const phase = k.split("/")[0];
    byPhase.set(phase, (byPhase.get(phase) ?? 0) + v);
  }
  const breakTotal = Object.values(summary.breaks).reduce((n, v) => n + v, 0);
  const categories = Object.entries(summary.breaks).sort((a, b) => b[1] - a[1]);

  const toggle = (key: "category" | "status", value: string) =>
    onFilters({ ...filters, [key]: filters[key] === value ? "" : value });

  return (
    <aside className="pt-6 text-sm">
      <h2 className="text-base font-semibold">Run {run.id}</h2>
      <p className="mt-1 text-ink-soft">
        {number.format(summary.ledger_entries)} ledger entries against{" "}
        {number.format(summary.custodian_entries)} custodian lines, in {(summary.duration_ms / 1000).toFixed(1)} s.
      </p>

      <h3 className="mt-6 font-semibold">{number.format(totalMatches)} matches</h3>
      <div className="mt-2 flex h-2 overflow-hidden rounded-full bg-rule-soft" aria-hidden>
        {[...byPhase].map(([phase, n], i) => (
          <span
            key={phase}
            style={{ width: `${(100 * n) / totalMatches}%`, opacity: 1 - i * 0.28 }}
            className="bg-settled"
          />
        ))}
      </div>
      <dl className="figures mt-2 space-y-0.5">
        {[...byPhase].map(([phase, n]) => (
          <div key={phase} className="flex justify-between">
            <dt className="text-ink-soft">{phaseName[phase] ?? phase}</dt>
            <dd>{number.format(n)}</dd>
          </div>
        ))}
        {proposed > 0 && (
          <div className="flex justify-between">
            <dt className="text-ink-soft">of which need review</dt>
            <dd>{number.format(proposed)}</dd>
          </div>
        )}
      </dl>

      <h3 className="mt-6 font-semibold">{number.format(breakTotal)} breaks</h3>
      <ul className="mt-2 space-y-0.5">
        {categories.map(([cat, n]) => (
          <li key={cat}>
            <button
              type="button"
              title={categoryHelp[cat]}
              aria-pressed={filters.category === cat}
              onClick={() => toggle("category", cat)}
              className={`figures -mx-2 flex w-[calc(100%+1rem)] justify-between rounded px-2 py-1 text-left hover:bg-rule-soft ${
                filters.category === cat ? "bg-ledger-wash font-semibold" : ""
              }`}
            >
              <span>{categoryName[cat] ?? cat}</span>
              <span>{number.format(n)}</span>
            </button>
          </li>
        ))}
      </ul>

      {counts.data && (
        <>
          <h3 className="mt-6 font-semibold">Where they stand</h3>
          <ul className="mt-2 space-y-0.5">
            {(Object.keys(statusName) as (keyof typeof statusName)[])
              .filter((s) => counts.data.by_status[s])
              .map((s) => (
                <li key={s}>
                  <button
                    type="button"
                    aria-pressed={filters.status === s}
                    onClick={() => toggle("status", s)}
                    className={`figures -mx-2 flex w-[calc(100%+1rem)] justify-between rounded px-2 py-1 text-left hover:bg-rule-soft ${
                      filters.status === s ? "bg-ledger-wash font-semibold" : ""
                    }`}
                  >
                    <span>{statusName[s]}</span>
                    <span>{number.format(counts.data.by_status[s])}</span>
                  </button>
                </li>
              ))}
          </ul>
        </>
      )}

      <p className="mt-8 text-xs leading-relaxed text-ink-soft">
        Rule version {run.rule_version}. Output fingerprint {run.output_sha256.slice(0, 16)}: rerunning
        these files with these rules reproduces it exactly.
      </p>
    </aside>
  );
}
