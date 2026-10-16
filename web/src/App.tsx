import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Run, type RunSummary as Summary } from "./api";
import { RunBar } from "./components/RunBar";
import { RunSummary } from "./components/RunSummary";
import { BreakQueue, type Filters } from "./components/BreakQueue";
import { BreakDetail } from "./components/BreakDetail";
import { people } from "./vocabulary";

function storedActor() {
  try {
    return localStorage.getItem("apex.actor") ?? people[0];
  } catch {
    return people[0];
  }
}

// ?run=3&break=95 opens straight onto a run and a break, so a break can be linked to.
function fromURL(key: string) {
  const v = Number(new URLSearchParams(window.location.search).get(key));
  return Number.isInteger(v) && v > 0 ? v : null;
}

export default function App() {
  const runs = useQuery({ queryKey: ["runs"], queryFn: api.runs });
  const [runId, setRunId] = useState<number | null>(() => fromURL("run"));
  const [actor, setActor] = useState(storedActor);
  const [filters, setFilters] = useState<Filters>({ category: "", status: "", account: "" });
  const [openBreak, setOpenBreak] = useState<number | null>(() => fromURL("break"));

  useEffect(() => {
    const params = new URLSearchParams();
    if (runId) params.set("run", String(runId));
    if (openBreak) params.set("break", String(openBreak));
    const query = params.toString();
    window.history.replaceState(null, "", query ? `?${query}` : window.location.pathname);
  }, [runId, openBreak]);

  const finished = (runs.data ?? []).filter((r) => r.status === "SUCCEEDED");
  const run: Run | undefined = finished.find((r) => r.id === runId) ?? finished[0];

  useEffect(() => {
    try {
      localStorage.setItem("apex.actor", actor);
    } catch {
      // Private windows can refuse storage; the choice just won't be remembered.
    }
  }, [actor]);

  const selectRun = (id: number) => {
    setRunId(id);
    setFilters({ category: "", status: "", account: "" });
    setOpenBreak(null);
  };

  return (
    <div className="min-h-screen">
      <RunBar runs={finished} run={run} onSelectRun={selectRun} actor={actor} onActor={setActor} />

      {runs.isError && (
        <p className="mx-6 mt-6 max-w-prose text-diff">
          The API isn't answering. Start it with <code>apex serve</code> and reload this page.
        </p>
      )}

      {runs.isSuccess && !run && (
        <section className="mx-6 mt-16 max-w-prose">
          <h1 className="text-2xl font-semibold">Nothing reconciled yet</h1>
          <p className="mt-2 text-ink-soft">
            Load a ledger file and a custodian statement with <code>apex ingest</code>, then choose Run
            reconciliation above. The breaks it finds will appear here.
          </p>
        </section>
      )}

      {run && (
        <div className="grid grid-cols-1 gap-x-10 px-6 pb-16 lg:grid-cols-[17rem_1fr]">
          <RunSummary
            run={run}
            summary={run.stats as Summary}
            filters={filters}
            onFilters={(f) => {
              setFilters(f);
              setOpenBreak(null);
            }}
          />
          <BreakQueue
            runId={run.id}
            filters={filters}
            onFilters={setFilters}
            selected={openBreak}
            onOpen={setOpenBreak}
          />
        </div>
      )}

      {openBreak !== null && (
        <BreakDetail id={openBreak} actor={actor} onClose={() => setOpenBreak(null)} onOpen={setOpenBreak} />
      )}
    </div>
  );
}
