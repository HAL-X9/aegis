// Package app is the composition root of the gateway process.
//
// It parses command-line flags, loads the runtime and routes configuration,
// builds the compiled snapshot, wires the dataplane and edge components, and
// owns the lifecycle of the public and system HTTP listeners from startup to
// graceful shutdown. No listener is opened until Run is called.

package app
