//go:build race

package markdown

// raceEnabled reports whether the test binary has the race detector, which
// makes times and memory unlike those of a normal build.
const raceEnabled = true
