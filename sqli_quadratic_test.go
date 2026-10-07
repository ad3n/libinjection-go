package libinjection

import (
	"strings"
	"testing"
	"time"
)

func TestParseStringLinear(t *testing.T) {
	const budget = 10 * time.Second
	n := 500_000
	payloads := map[string]string{
		"backslash-escaped quote": "'" + strings.Repeat(`\'`, n),
		"doubled quote":           "'" + strings.Repeat(`''`, n),
	}

	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})

			go func() {
				IsSQLi(payload)
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(budget):
				t.Fatalf("IsSQLi on a %d-byte %s payload did not finish within %s; "+
					"parseStringCore is scaling super-linearly", len(payload), name, budget)
			}
		})
	}
}
