// Package compile translates the normalized control-plane model into the
// compiled representation defined by package snapshot.
//
// It resolves symbolic references to numeric identifiers, compiles header and
// rate-limit policies, and constructs the radix-trie route index, which is
// flattened into a contiguous, allocation-free structure for request-time
// lookup. All work is performed at build time; nothing here runs on the
// request path.

package compile
