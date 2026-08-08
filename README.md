# Dagro

Dagro is a native Go port of [Dagre](https://github.com/dagrejs/dagre), the
directed graph layout engine. It intentionally replicates Dagre **0.8.5** and
the Graphlib **2.1.8** behavior used by that release, including layout order,
compound graphs, named multiedges, self-loops, and edge-label routing.

Dagro targets the exact Dagre 0.8.5 behavior embedded by D2. Later Dagre
releases retain the same broad layout pipeline but include behavior-changing
ordering, positioning, compound-layout, API, and dependency changes, so
compatibility is measured against 0.8.5.

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
coerced to `float64` in the internal layout graph for JavaScript `Number`
compatibility; arbitrary label values are preserved as supplied.

## Compatibility and tests

The Go source follows the Dagre 0.8.5 module boundaries:

- cycle removal and greedy feedback-arc selection;
- compound nesting, normalization, and dummy-chain parenting;
- longest-path, tight-tree, and network-simplex ranking;
- weighted crossing minimization;
- Brandes-Köpf coordinate assignment;
- self-edge, label, border, direction, and translation passes.

The normal suite contains direct Go ports of the upstream tests. An optional
differential suite replays ordered fixtures through both implementations and
compares topology, point order, attribute presence, and all numeric output:

```sh
DAGRO_DAGRE_JS=/absolute/path/to/dagre-0.8.5.js go test ./...
```

The differential test uses `node` only as a test oracle. Dagro itself has no
JavaScript runtime or third-party Go dependencies.

## Versioning

`Version` reports the replicated Dagre version (`0.8.5`). Until the first
tagged release, consumers developing Dagro and D2 together can use a Go
workspace or a temporary local `replace` directive.

## License

MIT. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for upstream attribution and
the exact compatibility revisions.
