import type { BreakStatus } from "./api";

// The analyst's words for what the engine stores. Every screen uses these, so an action keeps
// the same name from the queue to the detail panel to the audit trail.

export const categoryName: Record<string, string> = {
  TIMING: "Settled late",
  AMOUNT: "Amount differs",
  DUPLICATE: "Reported twice",
  MISSING_IN_LEDGER: "Not in ledger",
  MISSING_IN_CUSTODIAN: "Not at custodian",
  CORPORATE_ACTION: "Unbooked income",
  UNCLASSIFIED: "Unclassified",
};

export const categoryHelp: Record<string, string> = {
  TIMING: "Same reference and amount, but the custodian settled outside the matching window.",
  AMOUNT: "Same reference, different amounts.",
  DUPLICATE: "The custodian reported an identical line twice.",
  MISSING_IN_LEDGER: "The custodian moved cash the ledger has no entry for.",
  MISSING_IN_CUSTODIAN: "The ledger expects cash the custodian hasn't moved.",
  CORPORATE_ACTION: "A dividend or coupon arrived that the ledger hasn't booked.",
};

export const statusName: Record<BreakStatus, string> = {
  OPEN: "Open",
  ASSIGNED: "Assigned",
  PROPOSED: "Awaiting approval",
  APPROVED: "Approved",
  REJECTED: "Sent back",
  CLOSED: "Closed",
};

export const phaseName: Record<string, string> = {
  EXACT: "Exact",
  TOLERANCE: "Within tolerance",
  N_TO_M: "Grouped",
  FUZZY: "Likely, to review",
};

export const people = ["ana", "ben", "chen"];

export const number = new Intl.NumberFormat("en-US");

export function shortTime(iso: string) {
  return new Date(iso).toLocaleString("en-GB", {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export const actionName: Record<string, string> = {
  BOOK_ENTRY: "Book the missing entry",
  QUERY_CUSTODIAN: "Query the custodian",
  WAIT_FOR_SETTLEMENT: "Wait for settlement",
  WRITE_OFF: "Write off the difference",
  CANCEL_DUPLICATE: "Have the duplicate cancelled",
  INVESTIGATE: "Investigate",
};
