# mapo80/gnata — fork notes

Fork of [RecoLabs/gnata](https://github.com/RecoLabs/gnata) used by the ARC
engine (arc-workspace) through
`replace github.com/recolabs/gnata => github.com/mapo80/gnata <tag>`.
The module path stays `github.com/recolabs/gnata`. License unchanged: MIT,
Copyright (c) 2026 RecoLabs Inc. (`LICENSE`).

## Releases

| tag | base upstream | branch |
|---|---|---|
| `v0.5.0-arc.1` | `v0.5.0` (`775e5fbfac81e17b7abd37d3f01021e729a4e888`) | `arc/perf` |

## Changes in `v0.5.0-arc.1`

1. `fix: keep ARC's projection changes to v0.5.0` — the changes ARC carried
   in its in-repo projection of v0.5.0: `$append` of two empty arrays returns
   `[]`, not null; a string literal without escapes is lexed as a slice of the
   source; path steps collected into one slice, dot nodes rewritten in place.
2. `perf: size $merge's result and copy a first ordered object whole` —
   `OrderedMap.CloneWithCapacity`; `TestMergeMatchesTheMemberByMemberMerge`
   (oracle: the previous `$merge`).
3. `perf: grow an owned accumulator in an accumulating $reduce` — a reduce
   function `function($acc,...){$append($acc,E)}` or `{$merge([$acc,E])}`
   with `E` not reading `$acc` grows one array/object instead of copying it
   at every step (n instead of n²/2 copies); anything outside the mirrored
   cases runs the general reduction. `EvalLambdaPart`.
   `TestReduceOwnedAccumulationMatchesTheGeneralReduce` (21 cases against the
   same reduction forced through the general path).
4. `perf: reuse the environment of an ended lambda call nothing captured` —
   pooled call environments, kept when a closure (lambda, partial, composition,
   transform, `$eval`) captured them. `TestCallEnvironmentsCapturedByClosuresAreKept`
   (fails with the escape marking disabled).

The upstream test suite passes unchanged (`go test ./...`).

## Measurements (ARC, Go 1.27.1, Apple M2 Pro, nice 19, GOMAXPROCS=2)

| benchmark | before | after |
|---|---|---|
| `BenchmarkMergeFrame` (200 members, n=8) | 12.7 µs, 27.6 KiB, 23 allocs | 5.3 µs, 12.7 KiB, 6 allocs |
| `BenchmarkReduceAppend` (500 members, n=8) | 453 µs, 2.21 MiB | 86 µs, 113 KiB |
| ARC SPA confirmation turn (replay, warm) | 857–871 MB | 698–712 MB |
| ARC SPA correction turn (replay, warm) | 416–433 MB | 312–333 MB |
