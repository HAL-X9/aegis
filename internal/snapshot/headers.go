package snapshot

import "net/textproto"

// HeaderID is a compact runtime identifier of a header name.
//
// Well-known headers have fixed IDs below HeaderDynamicStart, defined once in
// the table in this file. Any other header is assigned a stable ID from
// HeaderDynamicStart upward during compilation and is resolved through the
// snapshot's HeaderRegistry.
//
// Runtime must never perform:
//   - string normalization
//   - canonicalization
//   - map lookups by string
type HeaderID uint16

const (
	HeaderUnknown HeaderID = iota

	HeaderHost
	HeaderContentType
	HeaderContentLength

	HeaderAuthorization

	HeaderXForwardedFor
	HeaderXForwardedProto
	HeaderXRequestID

	HeaderServer

	HeaderXContentTypeOptions
	HeaderXFrameOptions
	HeaderXXSSProtection

	// HeaderDynamicStart Dynamic headers begin here.
	HeaderDynamicStart
)

// wellKnownHeaderNames is the single source of truth for the fixed header
// IDs: it maps each one to its wire name, which is also the key under which
// net/http stores the header, so the name is in canonical MIME form. The
// compiler resolves names to IDs through it (WellKnownHeaderID) and the data
// plane resolves IDs back (WellKnownHeaderName). An ID missing from the table
// is caught by TestWellKnownHeaderTable.
var wellKnownHeaderNames = [HeaderDynamicStart]string{
	HeaderHost:                "Host",
	HeaderContentType:         "Content-Type",
	HeaderContentLength:       "Content-Length",
	HeaderAuthorization:       "Authorization",
	HeaderXForwardedFor:       "X-Forwarded-For",
	HeaderXForwardedProto:     "X-Forwarded-Proto",
	HeaderXRequestID:          "X-Request-Id",
	HeaderServer:              "Server",
	HeaderXContentTypeOptions: "X-Content-Type-Options",
	HeaderXFrameOptions:       "X-Frame-Options",
	HeaderXXSSProtection:      "X-Xss-Protection",
}

// wellKnownHeaderIDs is the reverse of wellKnownHeaderNames.
var wellKnownHeaderIDs = invertHeaderNames()

func invertHeaderNames() map[string]HeaderID {
	ids := make(map[string]HeaderID, len(wellKnownHeaderNames))
	for id, name := range wellKnownHeaderNames {
		if name != "" {
			ids[name] = HeaderID(id)
		}
	}
	return ids
}

// WellKnownHeaderName returns the wire name of a well-known header ID. It
// returns "" for HeaderUnknown and for dynamic IDs, which only a
// HeaderRegistry can resolve. It does not allocate.
func WellKnownHeaderName(id HeaderID) string {
	if id >= HeaderDynamicStart {
		return ""
	}
	return wellKnownHeaderNames[id]
}

// WellKnownHeaderID returns the fixed ID of the header called name, matching
// case-insensitively, and reports whether name is a well-known header.
// It is meant for compile time: it canonicalizes name and looks it up in a
// map.
func WellKnownHeaderID(name string) (HeaderID, bool) {
	id, ok := wellKnownHeaderIDs[textproto.CanonicalMIMEHeaderKey(name)]
	return id, ok
}

// HeaderRegistry resolves dynamic HeaderIDs into canonical header names.
//
// Registry is immutable after compilation.
//
// Runtime should use direct indexed lookup:
//
//	name := registry.Names[id-HeaderDynamicStart]
//
// No maps on hot path.
type HeaderRegistry struct {
	Names []string
}
