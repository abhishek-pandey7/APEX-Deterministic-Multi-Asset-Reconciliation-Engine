# 0002. Go for the engine and the API

Status: Accepted

## Context

The engine parses large files, matches hundreds of thousands of entries, and must give
byte-identical results on every run (ADR 0005). It also serves an HTTP API. The work is
CPU-bound within a partition and naturally parallel across partitions, and the matching logic
is custom, stateful, branchy code rather than columnar arithmetic.

## Decision

Write ingestion, matching, classification, persistence and the API in Go.

## Alternatives considered

| Option | Why not |
|---|---|
| Python with Polars or pandas | Polars is fast for columnar joins, but the matching phases are custom loops with early exits, greedy assignment and subset search. In Python those loops run in the interpreter, and the GIL limits threading for them. Python is still used where it fits: the data generator and the AI worker. |
| Java with Spring Boot | Apollo's own stack, and a strong choice. Rejected for this project on delivery speed and footprint: a single static binary, millisecond startup and goroutines made the partitioned scheduler simpler to write and test than an executor service. The engine's design (partitioning, canonical order, integer money) would carry over to Java unchanged. |
| Rust | Best raw performance and memory safety, but slower to iterate on while the matching rules were still being discovered through evaluation (ADR 0015). Go was fast enough: about 2M rows/s. |
| C++ | Same delivery cost as Rust without its safety guarantees. |

## Consequences

- One binary (`apex`) does ingestion, runs, evaluation and serving.
- Goroutine-per-partition parallelism with no locks on the matching path.
- Go is not in Apollo's listed stack (Java, Python, Spark). The trade-off is stated here rather
  than hidden, and the algorithms are language-independent.
- Go's garbage collector and map iteration randomness need care: the second is handled by ADR 0005.
