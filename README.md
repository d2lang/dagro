# Dagro

Dagro is a native Go implementation of the [Dagre](https://github.com/dagrejs/dagre)
layout behavior used by D2. Its behavioral source target is Dagre **3.1.1** and
the D2-used subset of Graphlib **4.0.5**. Dagro has no JavaScript runtime or
third-party Go dependency in production.

The compatibility boundary is D2's default Dagre adapter profile:

- directed, compound, named-multiedge graphs;
- graph `rankdir`, `nodesep`, `edgesep`, and `ranksep` attributes;
- node IDs, parent relationships, width, and height;
- named edge endpoints, width, height, and `labelpos`;
- graph bounds, node coordinates, edge-label coordinates, and ordered routes.

The public graph operations already exposed by Dagro remain available. This is
not a claim to implement every Dagre 3.1.1 or Graphlib 4.0.5 feature. In
particular, dynamic remembered layout state, per-cluster direction, manual or
custom ranking and ordering, constraints, and the `rankalign` option are
outside the verified D2 profile.

## Usage

```go
package main

import (
	"fmt"

	"github.com/d2lang/dagro"
)

func main() {
	g := dagro.NewGraph(dagro.GraphOptions{
		Compound:   true,
		Multigraph: true,
	}).SetGraph(dagro.Attrs{"rankdir": "TB"})

	g.SetDefaultEdgeLabel(func(string, string, *string) any {
		return dagro.Attrs{}
	})
	g.SetNode("a", dagro.Attrs{"width": 80, "height": 40})
	g.SetNode("b", dagro.Attrs{"width": 80, "height": 40})
	g.SetEdge("a", "b", dagro.Attrs{}, "a-to-b")

	if err := dagro.Layout(g); err != nil {
		panic(err)
	}

	a := g.Node("a").(dagro.Attrs)
	fmt.Println(a["x"], a["y"])
}
```

The API uses `Attrs` maps because Dagre and Graphlib labels are open JavaScript
objects. Recognized numeric layout attributes accept Go numeric types and are
coerced to `float64` for JavaScript `Number` compatibility; arbitrary label
values are preserved.

## Typed API

`LayoutTyped` lays out a `LayoutInput` (plain Go values: node sizes, parents
by index, edges by endpoint index) into a `LayoutOutput`, without building a
`Graph` or `Attrs` labels. It returns exactly what `Layout` writes for the
graph `LayoutInput.Graph` describes. D2 uses this API.

The typed engine behind it lays out every valid input whose node ids are
distinct, non-empty and free of `"\x01"` and whose edges are distinct, have
a whole-number `minlen` of at least 1 and touch no node with children. For
anything else `LayoutTyped` falls back to the `Graph` engine, the engine
behind `Layout` for graphs the typed engine does not take.

That fallback is the only thing that ties the typed API to the `Graph`
engine. A program that only calls the typed API and knows its inputs never
need the fallback can leave it out, and with it the engine it would never
run, by building with `-tags dagro_typed_only` (with TinyGo:
`tinygo build -tags=dagro_typed_only`). No build leaves it out on its own.

With that tag `LayoutTyped` returns `ErrGraphEngine` for a valid input the
typed engine does not accept (the error of `LayoutInput.Graph` for an invalid
one), which then includes inputs that ask for the `"greedy"` acyclicer,
and a panic of the typed engine reaches the caller instead of being handed
to the `Graph` engine. `Layout` and the rest of the `Graph` API
work in every build.

## Compatibility and tests

The checked-in D2 corpus contains 311 unique layouts captured at D2 commit
`1a60d69e4df9b9557923e61bf10f9aa3aa5422e1`:

- 308 finite outputs match official Dagre 3.1.1 bit-for-bit after the D2 JSON
  bridge normalization;
- three named layouts use a pinned compatibility result because official
  Dagre 3.1.1 either emits non-finite geometry or throws.

The compatibility correction only pairs parallel dummy nodes when exactly one
edge is reversed, preserves every reversed partner when several map to the
same forward dummy, and keeps degenerate zero-size rectangle intersections
finite. Dagro also collision-checks generated reversed-edge names so a caller
edge such as `rev1` cannot be overwritten. The first three changes are
recorded as a reviewable source patch in
`testdata/differential/dagre-3.1.1-d2-compat.patch`.

A fourth synthetic compatibility case minimizes the multiple-reversed-partner
failure using only D2-profile inputs. It is separate from the 311 captured
layouts and pins the finite result of the source patch.

The normal Go suite runs the complete 311-input corpus;
`go test -tags dagro_typed_only ./...` runs it without the `Graph` engine
fallback of `LayoutTyped`. CI additionally
installs the exact JavaScript oracle from `package-lock.json`, checks ordinary
fixtures against official Dagre 3.1.1 using exact `float64` bits, and verifies
the three captured upstream failures plus the minimized synthetic divergence:

```sh
cd testdata/differential
npm ci --ignore-scripts
cd ../..
DAGRO_DAGRE_JS="$PWD/testdata/differential/node_modules/@dagrejs/dagre/dist/dagre.cjs" go test ./...

./testdata/differential/build-dagre-3.1.1-d2-compat.sh /tmp/dagre-3.1.1-d2-compat.cjs
DAGRO_DAGRE_JS_COMPAT=/tmp/dagre-3.1.1-d2-compat.cjs \
  go test -run '^TestD2ProfileRandomLayoutsMatchCompatibilityOracle$' -count=1 .
```

The compatibility builder fetches the exact upstream commit, verifies its
lockfile and the source patch, builds with the upstream-pinned toolchain, and
checks the final CommonJS SHA-256 before writing it. No JavaScript bundle is
checked into Dagro or used in production.

See [UPSTREAM.md](UPSTREAM.md) for source pins, hashes, the port map, and the
precise compatibility boundary.

## Versioning

`Version` identifies the Dagre behavioral source (`3.1.1`) and
`GraphlibVersion` identifies the Graphlib behavioral source (`4.0.5`). They do
not expand the verified API surface beyond the D2 profile above.

## License

Dagro source is MIT. D2-derived corpus JSON retains D2's MPL-2.0 coverage;
expected files additionally contain generated layout geometry from the MIT
oracle.
See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[`testdata/differential/D2-CORPUS-NOTICE.md`](testdata/differential/D2-CORPUS-NOTICE.md).
