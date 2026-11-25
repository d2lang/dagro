//go:build dagro_typed_only

package dagro

// This file takes the place of typed_fallback.go in a build that sets the
// dagro_typed_only tag: LayoutTyped runs the typed engine and nothing else
// (see LayoutTyped). No build gets it without asking. Nothing here, and so nothing
// LayoutTyped reaches, refers to Layout, to the Attrs engine or to any Graph
// operation other than those LayoutInput.Graph builds a Graph with, so a
// program that only uses the typed API links none of them.
// TestTypedOnlyLinksNoGraphEngine holds the build to that.

// graphFallback reports whether LayoutTyped falls back to the Graph engine.
const graphFallback = false

// typedGreedy reports whether the typed engine takes graphs with acyclicer
// "greedy". Its greedy acyclicer runs the Graph implementation, so here it
// does not: buildTyped leaves such an input to layoutNotTyped, and build
// leaves such a graph to the Attrs engine, which lays it out the same.
const typedGreedy = false

// greedyFAS is not reached in this build (see typedGreedy).
func (s *lstate) greedyFAS() []edgeRef {
	panic("dagro: the typed engine has no greedy acyclicer in this build")
}

// layoutNotTyped is LayoutTyped for an input the typed engine does not take:
// the error in.Graph() gives for an invalid one, which is the error of
// LayoutTyped in every build, and ErrGraphEngine otherwise.
func layoutNotTyped(in *LayoutInput, out *LayoutOutput) error {
	if _, err := in.Graph(); err != nil {
		return err
	}
	return ErrGraphEngine
}

// layoutBuilt runs the typed engine on the state buildTyped made of in. A
// panic of the engine goes to the caller: there is no Graph engine to hand
// the input to (see lstate.runGuarded for what such a panic stands for), and
// the state is dropped, as after every call that panics.
func layoutBuilt(s *lstate, in *LayoutInput, out *LayoutOutput) error {
	return s.finishTyped(in, out, s.run())
}
