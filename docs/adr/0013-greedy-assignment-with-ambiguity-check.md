# 0013. Greedy assignment with an ambiguity check

Status: Accepted

## Context

When several ledger entries could pair with several custodian entries (three $25 fees on one day,
with timing differences), each entry must end up in at most one match. This is an assignment
problem on a bipartite graph of candidate pairs.

## Decision

Sort all candidate edges by (reference score, amount difference, date gap, external ids) and take
them greedily, skipping any edge whose entries are already used. An edge is auto-confirmed only if
its reference evidence is strong and it was the unique best option for both of its entries. Any
tie produces a proposal for a person, not a guess.

## Alternatives considered

| Option | Why not |
|---|---|
| Hungarian algorithm (optimal one-to-one assignment, O(n³)) | Optimal for a single summed cost, but "optimal" under a made-up cost function isn't more correct than a ranked greedy choice, and it can trade a clearly right pair for a better total. Components here are tiny (usually one pair), so optimality rarely changes anything. |
| Min-cost max-flow | Generalises to capacities and grouped matches, but adds complexity the evaluation didn't show a need for. |
| Greedy without an ambiguity check | Picks one of two equally good pairs by id order: deterministic, but still a guess presented as a fact. |

## Consequences

- Greedy is a ½-approximation of maximum-weight matching in theory. In practice, on the evaluation
  sets, every exact, timing and rounding scenario is recovered at 100%.
- Ambiguity becomes visible as proposals instead of hidden as arbitrary choices.
- If evaluation ever shows greedy losing real matches, Hungarian can be applied per connected
  component without changing the interface.
