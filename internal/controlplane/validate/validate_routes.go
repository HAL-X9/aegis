package validate

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
	"github.com/HAL-X9/aegis/internal/httpmethod"
)

// validateRoutes validates a collection of routes.
// It returns the first encountered validation error with the corresponding index.
func validateRoutes(routes []schema.Route) error {
	if routes == nil {
		return fmt.Errorf("routes must be provided: got nil slice")
	}

	for i, route := range routes {
		if err := validateRoute(&route); err != nil {
			return fmt.Errorf("invalid route at index %d: %w", i, err)
		}
	}
	return nil
}

// validateRoute performs validation of a single route definition,
// including identity, match conditions, and upstream configuration.
func validateRoute(route *schema.Route) error {
	if route == nil {
		return fmt.Errorf("route validation failed: route is nil")
	}

	if strings.TrimSpace(route.Name) == "" {
		return fmt.Errorf("route validation failed: name is required and must not be blank")
	}

	if err := validatePathPrefix(route.Match.PathPrefix); err != nil {
		return fmt.Errorf("route validation failed: match.path_prefix %w", err)
	}

	for _, method := range route.Match.Methods {
		if _, ok := httpmethod.Bit(method); !ok {
			return fmt.Errorf("route validation failed: match.methods contains unsupported method %q", method)
		}
	}

	for k, values := range route.Match.Headers {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("invalid header key: empty string")
		}

		if len(k) > 256 {
			return fmt.Errorf("invalid header key %q: exceeds 256 characters", k)
		}

		for _, value := range values {
			if value == "" {
				return fmt.Errorf("invalid header key %q: contains an empty allowed value", k)
			}
		}
	}

	return nil
}

// validatePathPrefix checks match.path_prefix syntax: "/" or "/"-separated
// segments, each a literal, a ":name" parameter, or a "*" / "*name"
// wildcard that must be the last segment. A single trailing slash is
// allowed; normalization drops it.
//
// Empty and dot segments are rejected because request paths are
// canonicalized before routing and never contain them.
func validatePathPrefix(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("is required and must not be blank")
	}
	if path[0] != '/' {
		return errors.New("must start with '/'")
	}
	if strings.ContainsAny(path, "?#") {
		return fmt.Errorf("%q must not contain '?' or '#'", path)
	}
	// Normalization trims Unicode whitespace, so allowing it would let "/ "
	// normalize to "/" and silently turn a one-segment route into a catch-all.
	for _, r := range path {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%q must not contain whitespace or control characters", path)
		}
	}
	if path == "/" {
		return nil
	}

	segments := strings.Split(strings.TrimSuffix(path[1:], "/"), "/")
	for i, segment := range segments {
		switch {
		case segment == "":
			return fmt.Errorf("%q has an empty segment", path)
		case segment == "." || segment == "..":
			return fmt.Errorf("%q has a dot segment %q", path, segment)
		case segment[0] == ':':
			if !isParamName(segment[1:]) {
				return fmt.Errorf("%q has an invalid parameter %q: want :name of letters, digits or '_'", path, segment)
			}
		case segment[0] == '*':
			if i != len(segments)-1 {
				return fmt.Errorf("%q has a wildcard %q that is not the last segment", path, segment)
			}
			if name := segment[1:]; name != "" && !isParamName(name) {
				return fmt.Errorf("%q has an invalid wildcard %q: want * or *name of letters, digits or '_'", path, segment)
			}
		}
	}

	return nil
}

// isParamName reports whether name is a non-empty run of ASCII letters,
// digits and '_'.
func isParamName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}
