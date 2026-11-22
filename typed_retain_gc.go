//go:build !tinygo

package dagro

// poolOversized reports whether putLState hands a state above
// lstateKeepMaxBytes to lstatePool. The gc toolchain's sync.Pool is emptied by
// the garbage collector, so a large state stays reachable only until the
// collector runs, and large graphs laid out one after another keep their
// storage.
const poolOversized = true
