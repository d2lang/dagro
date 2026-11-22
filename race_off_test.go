//go:build !race

package dagro

// raceEnabled reports a -race build (its sync.Pool drops Puts at random).
const raceEnabled = false
