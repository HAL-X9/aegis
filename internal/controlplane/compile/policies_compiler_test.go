package compile

import (
	"reflect"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/snapshot"
	"golang.org/x/time/rate"
)

func TestPolicies_nilReturnsError(t *testing.T) {
	_, err := Policies(nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPolicies_emptyReturnsEmptySnapshot(t *testing.T) {
	out, err := Policies(&ir.Policies{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out == nil {
		t.Fatal("expected non-nil compiled policies")
	}

	if len(out.RateLimits) != 0 {
		t.Fatalf("expected no compiled rate limits, got %d", len(out.RateLimits))
	}
}

func TestPolicies_compilesRateLimits(t *testing.T) {
	out, err := Policies(&ir.Policies{
		RateLimits: map[string]ir.RateLimit{
			"api": {
				Rate:  10,
				Burst: 20,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.RateLimits) != 1 {
		t.Fatalf("expected 1 compiled rate limit, got %d", len(out.RateLimits))
	}

	got := out.RateLimits[0]

	if got.Limit != rate.Limit(10) {
		t.Errorf("Limit = %v, want %v", got.Limit, rate.Limit(10))
	}

	if got.Burst != 20 {
		t.Errorf("Burst = %d, want 20", got.Burst)
	}
}

// Rate limits must be emitted in deterministic name order.
func TestPolicies_rateLimitsDeterministicOrder(t *testing.T) {
	out, err := Policies(&ir.Policies{
		RateLimits: map[string]ir.RateLimit{
			"z-limit": {
				Rate:  30,
				Burst: 3,
			},
			"a-limit": {
				Rate:  10,
				Burst: 1,
			},
			"m-limit": {
				Rate:  20,
				Burst: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.RateLimits) != 3 {
		t.Fatalf("expected 3 compiled rate limits, got %d", len(out.RateLimits))
	}

	want := []snapshot.CompiledRateLimit{
		{
			Limit: rate.Limit(10),
			Burst: 1,
		},
		{
			Limit: rate.Limit(20),
			Burst: 2,
		},
		{
			Limit: rate.Limit(30),
			Burst: 3,
		},
	}

	if !reflect.DeepEqual(out.RateLimits, want) {
		t.Fatalf("compiled rate limits = %+v, want %+v", out.RateLimits, want)
	}
}

func TestSortedStringMapKeys(t *testing.T) {
	t.Run("nil map returns nil", func(t *testing.T) {
		if got := sortedStringMapKeys(nil); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		if got := sortedStringMapKeys(map[string]string{}); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("single entry", func(t *testing.T) {
		want := []string{"Authorization"}

		if got := sortedStringMapKeys(map[string]string{
			"Authorization": "v",
		}); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("multiple entries are lexicographically sorted", func(t *testing.T) {
		m := map[string]string{
			"Content-Type":  "a",
			"Authorization": "b",
			"Host":          "c",
		}
		want := []string{
			"Authorization",
			"Content-Type",
			"Host",
		}

		if got := sortedStringMapKeys(m); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestSortedRateLimitNames(t *testing.T) {
	t.Run("nil policies returns nil", func(t *testing.T) {
		if got := sortedRateLimitNames(nil); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty map returns empty", func(t *testing.T) {
		got := sortedRateLimitNames(&ir.Policies{})

		if len(got) != 0 {
			t.Fatalf("expected empty result, got %v", got)
		}
	})

	t.Run("names are lexicographically sorted", func(t *testing.T) {
		policies := &ir.Policies{
			RateLimits: map[string]ir.RateLimit{
				"z-limit": {},
				"a-limit": {},
				"m-limit": {},
			},
		}

		want := []string{"a-limit", "m-limit", "z-limit"}

		if got := sortedRateLimitNames(policies); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
