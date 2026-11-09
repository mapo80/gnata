# mapo80/gnata — fork notes (branch `arc/json-v2`)

Fork of [RecoLabs/gnata](https://github.com/RecoLabs/gnata) used by the ARC
engine (arc-workspace) through
`replace github.com/recolabs/gnata => github.com/mapo80/gnata <tag>`.
The module path stays `github.com/recolabs/gnata`. License unchanged: MIT,
Copyright (c) 2026 RecoLabs Inc. (`LICENSE`).

## Releases

| tag | base | branch |
|---|---|---|
| `v0.5.0-arc.3` | `v0.5.0-arc.2` + custom environments over the shared standard library | `arc/json-v2` |
| `v0.5.0-arc.2` | `perf/v0.5.0` at `7fa870c99593` (upstream `v0.5.0` `775e5fbf` + "parser: flatten dotted paths into one step slice" + "functions: $append of two empty arrays returns an empty array"), the version ARC pinned as `v0.5.1-0.20261027144911-7fa870c99593` | `arc/json-v2` |

`v0.5.0-arc.1` (branch `arc/perf`) is a separate line with other performance
changes; `v0.5.0-arc.2` does not contain it.

## Changes in `v0.5.0-arc.3`

1. `perf: build custom environments over the shared standard library` —
   `NewCustomEnvironment`, `NewCustomEnv` and the stream evaluator's custom
   environment are a child of `builtinEnv` (the library registered once, at
   init) binding only the custom functions, instead of a new root that
   registers the whole library again. A custom function still shadows the
   standard one of the same name; every evaluation still binds its variables
   in a child with its own call counter (`evalCore`), so environments are
   shared read-only as before. Building an environment of 20 functions: 10 154
   B, 48 allocations, 4.9 µs → 2 568 B, 28 allocations, 1.2 µs
   (`BenchmarkNewCustomEnvironment`, Apple M2 Pro, go1.27.1, 3 runs).
   ARC builds one per turn and zone; with concurrent conversations its
   environments were rebuilt per evaluation (about 20 KB each).
   Tests: `TestCustomEnvironmentOverTheSharedLibrary` (shadowing, standard
   functions, `%`, lambdas, recursion, variables; green before and after),
   `TestCustomEnvironmentsEvaluateConcurrently` (64 environments, race),
   `TestCustomEnvironmentDoesNotRegisterTheLibraryAgain` (31 allocations
   before, red; within 12 after).

## Changes in `v0.5.0-arc.2`

1. `perf: read the byte fast paths with encoding/json/jsontext instead of gjson`
   (`f2396ee`) — `internal/rawjson` reads values inside a JSON document with
   `encoding/json/jsontext` from the standard library, with the semantics the
   gjson fast paths had: the first of duplicate names is found, a numeric step
   indexes an array, string contents keep invalid UTF-8 when they have no
   escape, the document is read only as far as a lookup needs. A pure path is
   resolved by its field names (`PathSteps`, also kept in
   `GroupPlan.FastPathSteps`) in one streaming pass that copies only the value
   found. `github.com/tidwall/gjson`, `match` and `pretty` are no longer
   dependencies; `go 1.27`.

## Equivalence

- `TestByteEvaluationMatchesGjsonGolden`: 20 documents (nested objects and
  arrays, duplicate names, invalid UTF-8 with and without escapes, escaped
  keys, big/exponent/out-of-range numbers, lone surrogates, top-level arrays
  and scalars, trailing data, malformed and empty input) × 158 expressions
  (pure paths, comparisons, the fast-path functions) × `EvalBytes`,
  `EvalBytesWithVars`, `EvalBytesWithCustomEnvironmentAndVars`, `EvalMap`:
  7 888 outcomes (type, value as encoding/json writes it, error) equal to
  `testdata/bytes_golden.json`, written by the gjson implementation at
  `7fa870c`. The 3 outcomes that differ are on `{"a":1,"b":[1,2}`, path `b`:
  gjson returned the malformed array, which then decoded to nothing (null);
  jsontext refuses it and the evaluation falls back to the decoder, which
  reports the syntax error, as `Eval` of the decoded document would.
- `TestByteEvaluationAgreesWithEval`: on valid documents without duplicates the
  byte entry points agree with `Eval`, except 10 outcomes where they already
  disagreed with gjson (numbers that are not canonical integers read as
  float64, a numeric step indexing an array), unchanged.
- Upstream suite green (`go test ./...`).

## Measures (8 November 2026, Apple M2 Pro, go1.27.1, n=6, machine shared with other loads)

ARC does not call the byte entry points (it evaluates decoded values with
`EvalWithCustomEnvironmentAndVars`), so these paths are off its turn.

| benchmark | gjson | jsontext | |
|---|---|---|---|
| `EvalBytes/Account.Name` | 146 ns, 32 B, 2 allocs | 307 ns, 32 B, 2 allocs | +110 % time |
| `EvalBytes/Account.Order.Product.SKU` | 5.7 µs, 2 930 B, 26 allocs | 10.4 µs, 2 860 B, 28 allocs | +82 % time |
| `EvalBytes_WideArrays/*` (walker over arrays) | 80–170 µs, 78–81 KB | 170–185 µs, 72–75 KB | about 2× time, −8 % bytes |
| `StreamEvaluator` | 22.1 µs, 9 961 B, 254 allocs | 27.4 µs, 10 029 B, 256 allocs | +24 % time |

jsontext validates every byte it reads (strings, numbers, nesting), where gjson
only matched brackets and quotes: the byte paths are slower, with the same or
fewer bytes allocated.
