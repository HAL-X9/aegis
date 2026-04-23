package controlplane

import "fmt"

/*
var allowedHTTPMethods = map[string]struct{}{
	"GET":     {},
	"POST":    {},
	"PUT":     {},
	"DELETE":  {},
	"PATCH":   {},
	"OPTIONS": {},
	"HEAD":    {},
}
*/

func Validate(cfg *AegisManifest) error {
	if cfg == nil {
		return fmt.Errorf("validate: config is nil")
	}

	return nil
}

func ValidateRoute() error {
	return nil
}

func ValidateMatch(cfg *Match) error {
	if cfg.PathPrefix == "" {
		return fmt.Errorf("validate match: path_prefix must not be empty")
	}

	if len(cfg.Methods) == 0 {
		return fmt.Errorf("validate match: methods must contain at least one HTTP method")
	}

	return nil
}
