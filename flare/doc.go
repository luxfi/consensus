// Package flare finalizes DAG cuts via a cascading accept protocol.
//
// Once prism + horizon narrow the candidate set, Flare walks the dependency
// graph and accepts vertices in causal order. It's the controlled detonation
// that commits the prepared transactions—deliberate, irreversible, beautiful.
// In the metaphor, it's the stellar flare that lights up space.
package flare
