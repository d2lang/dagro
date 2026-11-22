//go:build tinygo

package dagro

// poolOversized reports whether putLState hands a state above
// lstateKeepMaxBytes to lstatePool. TinyGo's sync.Pool is a plain list that
// never frees what it holds, so such a state is dropped instead.
const poolOversized = false
