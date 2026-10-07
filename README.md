# libinjection 
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)
[![codecov](https://codecov.io/gh/corazawaf/libinjection-go/branch/master/graph/badge.svg?token=RTCQXUDZQQ)](https://codecov.io/gh/corazawaf/libinjection-go)
[![CodeQL](https://github.com/corazawaf/libinjection-go/actions/workflows/codeql.yml/badge.svg)](https://github.com/corazawaf/libinjection-go/actions/workflows/codeql.yml)

libinjection is a Go porting of the libinjection([http://www.client9.com/projects/libinjection/](http://www.client9.com/projects/libinjection/)) and it's thread safe.

## How to use
### SQLi Example
```go
package main

import (
    "fmt"
    "github.com/corazawaf/libinjection-go"
)

func main() {
    result, fingerprint := libinjection.IsSQLi("-1' and 1=1 union/* foo */select load_file('/etc/passwd')--")
    fmt.Println("=========result==========: ", result)
    fmt.Println("=======fingerprint=======: ", string(fingerprint))
}
```

### XSS Example
```go
package main

import (
	"fmt"
	"github.com/corazawaf/libinjection-go"
)

func main() {
	fmt.Println("result: ", libinjection.IsXSS("<script>alert('1')</script>"))
}
```

## Performance and compatibility

SQLi and HTML5 parser states are pooled. HTML5 transitions use method expressions
with an explicit state receiver, so transitions do not allocate bound method
values. `IsXSS` borrows one state for its ordered parse contexts and clears all
input/token references before returning it to the pool.

SQLi fingerprints remain in a fixed byte array during detection. A positive
result returns an independent string, preserving the existing `(bool, string)`
API and allowing callers to keep fingerprints after subsequent calls. ASCII
keyword lookups use a bounded stack buffer; Unicode and oversized keys retain
the original Unicode uppercasing behavior. Candidate compound tokens allocate
only when a merge succeeds. No `unsafe`, CGO, new dependency, or public API is
introduced.

The module requires Go 1.26.0 or newer. Syntax uses integer range and benchmarks
use `b.Loop`; Go 1.26's modernizer has been applied. The public API and detection
behavior are unchanged. Consumers using older Go versions must upgrade their
toolchain.
[Upgrade verification and benchmarks](benchmarks/2026-10-08-go126/module-upgrade/README.md)
compare the same source before and after raising the module minimum: allocation
counts are unchanged and neither batch shows a statistically significant timing
change. No performance improvement is claimed for the version upgrade.

The allocation optimization measurements below compare HEAD `d7836f7` with the
optimized implementation before the style cleanup, using identical benchmark fixtures: Go 1.26.0, Linux/amd64, AMD Ryzen AI 9 HX 375,
`GOMAXPROCS=1`, six samples of 200 ms each. Values are medians. Payload batch
benchmarks process the entire input set per operation: 31 SQLi or 28 XSS inputs.

| Benchmark | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| SQLi payload batch | 12217 | 8214 | 1736 | 176 | 159 | 26 |
| XSS payload batch | 7373.5 | 2537 | 3584 | 0 | 224 | 0 |
| SQLi clean short text | 204.35 | 110 | 48 | 0 | 5 | 0 |
| SQLi clean long text | 644.7 | 329.6 | 208 | 0 | 17 | 0 |
| SQLi clean email | 108.25 | 70.43 | 16 | 0 | 2 | 0 |
| SQLi UNION | 740.8 | 405.75 | 192 | 5 | 14 | 1 |
| XSS clean HTML | 506.5 | 97.605 | 304 | 0 | 19 | 0 |
| XSS script tag | 98.06 | 39.44 | 48 | 0 | 3 | 0 |

Batch latency reductions are statistically significant (`p=0.002`, `n=6`):
32.77% for SQLi and 65.59% for XSS. In that optimization run, no measured case has a statistically significant
slowdown. Boolean, time-based, error-based and ORDER BY SQLi cases show no
statistically significant timing change; no latency improvement is claimed for
those cases. [Raw measurements and benchstat comparison](benchmarks/2026-10-08-go126/benchstat.txt)
include every individual case, including unchanged allocation counts.

Zero-allocation results describe warm-pool calls on the measured inputs. A cold
pool or a pool cleared by GC allocates. SQLi positives generally allocate the
returned fingerprint string, and successful compound-token merges can allocate.
Unicode fallback and caller-created inputs can also allocate. These changes do
not promise zero allocation for every input or workload. Cached state objects
remain subject to the existing `sync.Pool` lifecycle; no unbounded interning cache
is introduced.

Reproduce the measurements with the same fixture on both versions:

```sh
GOTOOLCHAIN=go1.26.0 GOMAXPROCS=1 go test -run '^$' \
  -bench 'BenchmarkIs(SQLi|XSS)(_Payloads)?$' -benchmem -count=6 -benchtime=200ms
```

For a baseline, extract `d7836f7` into an isolated checkout and copy the final
`bench_crs_test.go` into it so both use `b.Loop`. Tests, race tests and vet pass on
Go 1.26; before the module upgrade, race tests also passed on Go 1.24.6. Staticcheck and the configured
golangci complexity gate pass. A deterministic differential check across 105143
inputs (fixtures, NUL/Unicode variants and seeded arbitrary-byte inputs) produced
identical SQLi verdicts, fingerprints and XSS verdicts before and after. Pool reuse
coverage also retains returned fingerprints to check their lifetime, and existing
concurrency tests run under the race detector.

Coraza operator tests and engine profiles under default, no_memoize,
multiphase_evaluation and no_regex_multiline pass using this checkout
through a temporary workspace. Coraza's dependency pin is unchanged; publishing
or selecting the updated library is a separate integration step.

The subsequent style cleanup applies guard clauses and early returns to terminal
branches and uses switches for mutually exclusive branches with shared follow-up
work. Handwritten Go files contain no `else` branches. Ordinary Go comments were
removed under the shared style rules; machine-significant lint directives remain.
Spacing after complete brace statements is consistent, including deferred closures.
Go 1.26's `go fix ./...` reports no remaining applicable rewrites with the module
minimum set to Go 1.26.0. No extra syntax changes are forced when the modernizer
finds no applicable rewrites.

`betteralign -apply ./...` was checked; its three struct-layout rewrites were
restored to keep the measured layout. Style cleanup makes no performance
improvement claim. Its measurements are recorded separately from the allocation
optimization measurements above. An isolated comparison of 24 cases with eight
alternating samples per version found no statistically significant slowdown and
unchanged allocation counts in every case. SQLi batch medians were 8386 -> 8295
ns/op (`p=0.505`); XSS batch medians were 2582 -> 2635 ns/op (`p=0.124`).
[Style measurements, rejected candidates and reproduction instructions](benchmarks/2026-10-08-go126/style/README.md)
include the raw results and the baseline source patch. All detection outputs
remain identical across the same 105143-input differential check after the cleanup.

## License
libinjection-go is distributed under the same license as the [libinjection](http://www.client9.com/projects/libinjection/).