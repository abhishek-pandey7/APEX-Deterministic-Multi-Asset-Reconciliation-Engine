import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createColumnHelper, flexRender, getCoreRowModel, useReactTable } from "@tanstack/react-table";
import { api, type Break } from "../api";
import { categoryName, number, statusName } from "../vocabulary";

export type Filters = { category: string; status: string; account: string };

const pageSize = 50;
const col = createColumnHelper<Break>();

const columns = [
  col.accessor("opened_on", { header: "Value date", cell: (c) => <span className="figures">{c.getValue()}</span> }),
  col.accessor("account_id", { header: "Account" }),
  col.accessor("category", { header: "What happened", cell: (c) => categoryName[c.getValue()] ?? c.getValue() }),
  col.accessor("amount", {
    header: () => <span className="block text-right">Ledger minus custodian</span>,
    cell: (c) => (
      <span className={`figures block text-right ${c.row.original.amount_minor !== 0 ? "text-diff" : "text-ink-soft"}`}>
        {c.getValue()}
      </span>
    ),
  }),
  col.accessor("status", {
    header: "Status",
    cell: (c) => (
      <span className={c.getValue() === "CLOSED" ? "text-settled" : c.getValue() === "PROPOSED" ? "font-semibold" : ""}>
        {statusName[c.getValue()]}
        {c.getValue() !== "OPEN" && <span className="text-ink-soft">, {c.row.original.status_by}</span>}
      </span>
    ),
  }),
];

type Props = {
  runId: number;
  filters: Filters;
  onFilters: (f: Filters) => void;
  selected: number | null;
  onOpen: (id: number) => void;
};

export function BreakQueue({ runId, filters, onFilters, selected, onOpen }: Props) {
  const [offset, setOffset] = useState(0);
  const [lastKey, setLastKey] = useState("");
  const key = JSON.stringify([runId, filters]);
  if (key !== lastKey) {
    // New run or filter: start from the first page.
    setLastKey(key);
    setOffset(0);
  }

  const page = useQuery({
    queryKey: ["breaks", runId, filters, offset],
    queryFn: () => api.breaks(runId, { ...filters, offset, limit: pageSize }),
    placeholderData: keepPreviousData,
  });

  const table = useReactTable({
    data: page.data?.breaks ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (b) => String(b.id),
  });

  const total = page.data?.total ?? 0;
  const filtered = filters.category || filters.status || filters.account;

  return (
    <main className="min-w-0 pt-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold">Breaks to work</h1>
          <p className="text-sm text-ink-soft">
            {page.isLoading
              ? "Loading breaks"
              : total === 0
                ? filtered
                  ? "No breaks match these filters."
                  : "Every entry in this run matched."
                : `${number.format(offset + 1)} to ${number.format(Math.min(offset + pageSize, total))} of ${number.format(total)}${
                    filters.category ? `, ${categoryName[filters.category]?.toLowerCase()}` : ""
                  }${filters.status ? `, ${statusName[filters.status as keyof typeof statusName].toLowerCase()}` : ""}`}
          </p>
        </div>
        <div className="flex items-center gap-3 text-sm">
          <label className="flex items-center gap-2">
            <span className="text-ink-soft">Account</span>
            <input
              value={filters.account}
              onChange={(e) => onFilters({ ...filters, account: e.target.value.trim().toUpperCase() })}
              placeholder="ACC00001"
              className="w-28 rounded border border-rule bg-sheet px-2 py-1"
            />
          </label>
          {filtered && (
            <button
              type="button"
              className="text-ledger underline underline-offset-2"
              onClick={() => onFilters({ category: "", status: "", account: "" })}
            >
              Clear filters
            </button>
          )}
        </div>
      </div>

      <div className="mt-4 overflow-x-auto rounded-md border border-rule bg-sheet">
        <table className="w-full text-sm">
          <thead className="border-b border-rule text-left text-ink-soft">
            {table.getHeaderGroups().map((g) => (
              <tr key={g.id}>
                {g.headers.map((h) => (
                  <th key={h.id} className="px-3 py-2 font-medium">
                    {flexRender(h.column.columnDef.header, h.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody className={page.isPlaceholderData ? "opacity-60" : ""}>
            {table.getRowModel().rows.map((row) => (
              <tr
                key={row.id}
                tabIndex={0}
                aria-selected={selected === row.original.id}
                onClick={() => onOpen(row.original.id)}
                onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), onOpen(row.original.id))}
                className={`cursor-pointer border-b border-rule-soft last:border-0 hover:bg-paper ${
                  selected === row.original.id ? "bg-ledger-wash" : ""
                }`}
              >
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className="whitespace-nowrap px-3 py-2">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {total > pageSize && (
        <div className="mt-3 flex justify-end gap-2 text-sm">
          <button
            type="button"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - pageSize))}
            className="rounded border border-rule bg-sheet px-3 py-1 disabled:opacity-40"
          >
            Previous
          </button>
          <button
            type="button"
            disabled={offset + pageSize >= total}
            onClick={() => setOffset(offset + pageSize)}
            className="rounded border border-rule bg-sheet px-3 py-1 disabled:opacity-40"
          >
            Next
          </button>
        </div>
      )}
    </main>
  );
}
