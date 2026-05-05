package validate

import (
	"fmt"

	"github.com/aegis/internal/controlplane/model"
)

func validatesPolicies(policy *model.Policies) error {
	if policy == nil {
		return fmt.Errorf("")
	}

	for _, header := range policy.Headers {
		if err := validateHeaders(&header); err != nil {
			return fmt.Errorf("")
		}
	}

	return nil
}

func validateHeaders(headers *model.Headers) error {

	// TODO: validate request/response header operations and reject conflicting names across add/set/remove.

	return nil
}
