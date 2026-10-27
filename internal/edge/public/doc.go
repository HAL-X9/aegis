// Package public assembles the HTTP handler of the public listener.
//
// It adapts the proxy executor to net/http and composes the middleware chain
// selected by Options: request timeout, metrics, and request identification.

package public
