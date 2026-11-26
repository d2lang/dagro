// Package dagro lays out directed compound multigraphs exactly as Dagre 3.1.1
// does, in the profile D2 uses: LayoutTyped takes a LayoutInput (graph
// attributes, nodes with sizes and parents, named edges with label sizes) and
// stores node centers, edge routes and edge label positions in a
// LayoutOutput, bit for bit as dagre.js computes them.
//
// LayoutTyped returns an error wrapping ErrUnsupportedInput for inputs it
// does not lay out and a *PanicError for inputs on which Dagre itself fails;
// it does not panic. See the README for the verified profile and the tests
// that pin it.
package dagro
