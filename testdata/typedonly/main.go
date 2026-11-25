// Command typedonly uses dagro the way D2's dagre adapter does: through the
// typed API alone (LayoutInput, NewLayoutEdge, LayoutTyped, and
// LayoutInput.Graph for the error of an invalid input). The package's tests
// build it to check what such a program links
// (TestTypedOnlyLinksNoGraphEngine).
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/d2lang/dagro"
)

func main() {
	n := len(os.Args) + 3
	in := dagro.LayoutInput{RankDir: os.Getenv("RANKDIR"), RankSep: 100, EdgeSep: 20, NodeSep: 60}
	for i := 0; i < n; i++ {
		parent := -1
		if i%3 == 1 {
			parent = i - 1
		}
		in.Nodes = append(in.Nodes, dagro.LayoutNode{ID: strconv.Itoa(i), Parent: parent, Width: float64(10 + i), Height: 20})
	}
	if os.Getenv("CYCLE") != "" {
		in.Nodes[0].Parent = 1
		if _, err := in.Graph(); err != nil {
			fmt.Println(err)
			return
		}
	}
	for i := 2; i < n; i++ {
		if i%3 == 0 || (i-2)%3 == 0 {
			continue
		}
		e := dagro.NewLayoutEdge(i-2, i)
		e.Width, e.Height, e.LabelPos, e.HasName = 20, 10, "c", true
		in.Edges = append(in.Edges, e)
	}
	var out dagro.LayoutOutput
	if err := dagro.LayoutTyped(&in, &out); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(out.Width, out.Height, out.Nodes, out.Edges)
}
