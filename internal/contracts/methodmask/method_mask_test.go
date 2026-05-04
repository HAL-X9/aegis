package methodmask

import "testing"

func TestMethodBit(t *testing.T) {
	tests := []struct {
		name   string
		method string
		wantOk bool
	}{
		{"GET", "GET", true},
		{"POST", "POST", true},
		{"PUT", "PUT", true},
		{"DELETE", "DELETE", true},
		{"PATCH", "PATCH", true},
		{"OPTIONS", "OPTIONS", true},
		{"HEAD", "HEAD", true},

		{"lowercase", "get", false},
		{"invalid", "TRACE", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MethodBit(tt.method)
			if ok != tt.wantOk {
				t.Fatalf("expected ok=%v, got %v", tt.wantOk, ok)
			}
			if tt.wantOk && got == 0 {
				t.Fatalf("expected non-zero bit for valid method %s", tt.method)
			}
		})
	}
}

func TestBuildMethodMask(t *testing.T) {
	t.Run("empty input returns MethodAll", func(t *testing.T) {
		got, err := BuildMethodMask([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != MethodAll {
			t.Fatalf("expected MethodAll, got %v", got)
		}
	})

	t.Run("single method GET", func(t *testing.T) {
		got, err := BuildMethodMask([]string{"GET"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected, _ := MethodBit("GET")
		if got != expected {
			t.Fatalf("expected %v, got %v", expected, got)
		}
	})

	t.Run("multiple methods OR combination", func(t *testing.T) {
		got, err := BuildMethodMask([]string{"GET", "POST"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		getBit, _ := MethodBit("GET")
		postBit, _ := MethodBit("POST")

		expected := getBit | postBit

		if got != expected {
			t.Fatalf("expected %v, got %v", expected, got)
		}
	})

	t.Run("invalid method returns error", func(t *testing.T) {
		_, err := BuildMethodMask([]string{"GET", "INVALID"})
		if err == nil {
			t.Fatalf("expected error for invalid method")
		}
	})

	t.Run("all methods supported", func(t *testing.T) {
		methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}

		got, err := BuildMethodMask(methods)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != MethodAll {
			t.Fatalf("expected MethodAll, got %v", got)
		}
	})
}
