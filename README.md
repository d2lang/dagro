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

This is not a claim to implement every Dagre 3.1.1 or Graphlib 4.0.5 feature.
In particular, dynamic remembered layout state, per-cluster direction, manual
or custom ranking and ordering, constraints, and the `rankalign` option are
outside the verified D2 profile.

## Usage

```go
package main

import (
	"fmt"

	"github.com/d2lang/dagro"
)

func main() {
	in := dagro.DefaultLayoutInput() // rankdir TB, ranksep 50, edgesep 20, nodesep 50
	in.Nodes = []dagro.LayoutNode{
		{ID: "a", Parent: -1, Width: 80, Height: 40},
		{ID: "b", Parent: -1, Width: 80, Height: 40},
	}
	e := dagro.NewLayoutEdge(0, 1) // Dagre's default edge attributes
	e.Name, e.HasName = "a-to-b", true
	in.Edges = []dagro.LayoutEdge{e}

	var out dagro.LayoutOutput
	if err := dagro.LayoutTyped(&in, &out); err != nil {
		panic(err)
	}
	fmt.Println(out.Nodes[0].X, out.Nodes[0].Y, out.Edges[0].Points)
}
```

`LayoutTyped` lays out a compound multigraph described by typed fields and
produces exactly the numbers Dagre 3.1.1 produces for the equivalent Graphlib
graph (see the `LayoutInput` documentation). Nodes refer to their parents and
edges to their endpoints by index; the output is indexed like the input.

It returns an error instead of a layout for:

- inputs it does not lay out, with an error wrapping `ErrUnsupportedInput`: no
  nodes, a node id that is empty or contains `"\x01"`, two nodes with the same
  id, a parent out of range or parents forming a cycle, an edge endpoint out of
  range or with children, two edges with the same endpoints and name, or more
  than 2^26 ranks;
- inputs on which Dagre itself fails, and any other panic inside the layout,
  with a `*PanicError` whose `Value` is the panic value (for example
  `dagro: layout failed: dagre fails on this input: it sets "order" on a node
  missing from a layer`);
- Dagre's own error `not possible to find intersection inside of the
  rectangle`.

`LayoutTyped` is safe for concurrent use and does not panic; only fatal runtime
errors, such as running out of memory, end the program.

### Changes in v0.4.0

v0.4.0 removes the original line-by-line port of Dagre's JavaScript: `Graph`,
`Attrs`, `Layout`, `LayoutOptions`, `Edge`, `GraphOptions`, `DebugOrdering`,
`Time`, `NoTime` and `LayoutInput.Graph`. `LayoutTyped`, which D2 uses, was
already a separate engine; it used to hand inputs it did not take, and inputs
on which it panicked, to the Graph engine. Those inputs now give the errors
above. Every input that the typed engine laid out in v0.3.1 gets the same
layout, bit for bit (see the golden results below).

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

The normal Go suite runs the complete 311-input corpus through `LayoutTyped`.
`TestGolden` checks 14,111 inputs (the corpus, D2-shaped random inputs and
random inputs with every option and unusual values) against
`testdata/golden`, results recorded with v0.3.1's typed engine when the Graph
engine and Dagre still checked it: the kind of outcome, the dummy-name counter
draws and a hash of the exact bits of each layout. CI additionally
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

See [UPSTREAM.md](UPSTREAM.md) for source pins, hashes and the precise
compatibility boundary.

## Versioning

`Version` identifies the Dagre behavioral source (`3.1.1`) and
`GraphlibVersion` identifies the Graphlib behavioral source (`4.0.5`) whose
graph semantics (node, edge and child order) the engine reproduces. They do
not expand the verified API surface beyond the D2 profile above.

## License

Dagro source is MIT. D2-derived corpus JSON retains D2's MPL-2.0 coverage;
expected files additionally contain generated layout geometry from the MIT
oracle.
See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[`testdata/differential/D2-CORPUS-NOTICE.md`](testdata/differential/D2-CORPUS-NOTICE.md).
