# 0001. Record architecture decisions

Status: Accepted

## Context

A reconciliation engine is judged as much on why it was built a certain way as on what it does.
Reviewers ask "why Go and not Java?", "why not Kafka?", "why not an LLM for matching?". Answers
reconstructed months later tend to be rationalisations; answers written when the decision was
made include the alternatives that were actually on the table and the evidence at the time.

## Decision

Every significant decision gets a short record in `docs/adr/`, written when it is made, using the
same sections:

- **Context**: the problem and the forces at play.
- **Decision**: what was chosen.
- **Alternatives considered**: each serious option and the specific reason it lost.
- **Consequences**: what the decision costs, including limitations knowingly accepted.

Records are append-only, like the audit log. A reversed decision gets a new record marked as
superseding the old one.

## Alternatives considered

| Option | Why not |
|---|---|
| Design notes only in `ARCHITECTURE.md` | Describes the current shape, not why other shapes were rejected; edits erase history |
| Rationale in commit messages | Scattered, unsearchable, and commits here are deliberately short |
| Write it all up at the end | Loses the alternatives and the measurements that drove each choice |

## Consequences

A little writing per decision. In return, every choice can be defended with the reasoning that
produced it, and a reader can tell deliberate trade-offs from accidents.
