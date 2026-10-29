package fault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEveryReasonIsDefined(t *testing.T) {
	codes := make(map[string]Reason)

	for reason := NotReady; reason < numReasons; reason++ {
		if reason.Status() < 400 || reason.Status() > 599 {
			t.Errorf("reason %d: status %d is not an error status", reason, reason.Status())
		}
		if reason.String() == "" {
			t.Errorf("reason %d has no code", reason)
		}
		if other, ok := codes[reason.String()]; ok {
			t.Errorf("reasons %d and %d share code %q", other, reason, reason.String())
		}
		codes[reason.String()] = reason
	}
}

func TestWrite(t *testing.T) {
	for reason := NotReady; reason < numReasons; reason++ {
		t.Run(reason.String(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			Write(rec, reason)

			if rec.Code != reason.Status() {
				t.Errorf("status = %d, want %d", rec.Code, reason.Status())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}

			var doc struct {
				Type   string `json:"type"`
				Title  string `json:"title"`
				Status int    `json:"status"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
				t.Fatalf("body %q is not JSON: %v", rec.Body, err)
			}
			if want := "urn:aegis:problem:" + reason.String(); doc.Type != want {
				t.Errorf("type = %q, want %q", doc.Type, want)
			}
			if doc.Title == "" {
				t.Error("title is empty")
			}
			if doc.Status != reason.Status() {
				t.Errorf("body status = %d, want %d", doc.Status, reason.Status())
			}
		})
	}
}

func TestWriteDoesNotAllocate(t *testing.T) {
	w := discardWriter{header: make(http.Header)}

	allocs := testing.AllocsPerRun(100, func() { Write(w, NoRoute) })
	if allocs != 0 {
		t.Errorf("Write allocates %v times per call, want 0", allocs)
	}
}

type discardWriter struct{ header http.Header }

func (w discardWriter) Header() http.Header         { return w.header }
func (w discardWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w discardWriter) WriteHeader(int)             {}
