package libinjection

import (
	"strings"
	"sync"
	"testing"
)

func TestSQLiPoolReuse(t *testing.T) {
	cases := []struct {
		input  string
		isSQLi bool
	}{
		{`1 UNION SELECT username, password FROM users--`, true},
		{`hello world`, false},
		{`1' AND 1=1--`, true},
		{`user@example.com`, false},
		{`1'; DROP TABLE users--`, true},
		{`2024-01-15`, false},
		{`1/**/UNION/**/SELECT/**/1,2,3--`, true},
		{`The quick brown fox jumps over the lazy dog`, false},

		{`1 UNION SELECT username, password FROM users--`, true},
		{`1 UNION SELECT username, password FROM users--`, true},

		{`hello world`, false},
		{`hello world`, false},
	}

	retained := make([]struct{ got, want string }, 0, len(cases))
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, fingerprint := IsSQLi(tc.input)
			if got != tc.isSQLi {
				t.Errorf("IsSQLi(%q) = %v, want %v", tc.input, got, tc.isSQLi)
			}

			retained = append(retained, struct{ got, want string }{fingerprint, strings.Clone(fingerprint)})
		})
	}

	for _, result := range retained {
		if result.got != result.want {
			t.Errorf("fingerprint changed after pool reuse: got %q, want %q", result.got, result.want)
		}
	}
}

func TestXSSPoolReuse(t *testing.T) {
	cases := []struct {
		input string
		isXSS bool
	}{
		{`<script>alert(1)</script>`, true},
		{`<p>Hello world</p>`, false},
		{`<img src=x onerror=alert(1)>`, true},
		{`normal text without any html`, false},
		{`<svg onload=alert(1)>`, true},
		{`john.doe@example.com`, false},

		{`<script>alert(1)</script>`, true},
		{`<script>alert(1)</script>`, true},

		{`<p>Hello world</p>`, false},
		{`<p>Hello world</p>`, false},
	}

	for _, tc := range cases {
		got := IsXSS(tc.input)
		if got != tc.isXSS {
			t.Errorf("IsXSS(%q) = %v, want %v", tc.input, got, tc.isXSS)
		}
	}
}

func TestXSSDataStatePrefilter(t *testing.T) {
	noAngleAttacks := []string{
		`onerror=alert(1)`,
		`onerror=alert(1)>`,
		`x onerror=alert(1);>`,
		`x' onerror=alert(1);>`,
		`x" onerror=alert(1);>`,
		`onload=alert(1)`,
		`onclick=alert(1)`,
	}

	for _, input := range noAngleAttacks {
		if !IsXSS(input) {
			t.Errorf("IsXSS(%q) = false, want true (attribute-value context)", input)
		}
	}

	noAngleClean := []string{
		`hello world`,
		`john.doe@example.com`,
		`onY29va2llcw==`,
		`myvar=onfoobar==`,
		`2024-01-15`,
	}

	for _, input := range noAngleClean {
		if IsXSS(input) {
			t.Errorf("IsXSS(%q) = true, want false (clean input, no '<')", input)
		}
	}
}

const (
	concurrencyGoroutines = 50
	concurrencyIterations = 200
)

func runConcurrent(t *testing.T, name string, check func(t *testing.T)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		var wg sync.WaitGroup
		wg.Add(concurrencyGoroutines)
		for range concurrencyGoroutines {
			go func() {
				defer wg.Done()
				for range concurrencyIterations {
					check(t)
				}
			}()
		}

		wg.Wait()
	})
}

func TestPoolConcurrency(t *testing.T) {
	t.Parallel()

	runConcurrent(t, "SQLi", func(t *testing.T) {
		if got, _ := IsSQLi(`1 UNION SELECT 1,2--`); !got {
			t.Error("IsSQLi: expected true for attack input")
		}

		if got, _ := IsSQLi(`hello world`); got {
			t.Error("IsSQLi: expected false for clean input")
		}
	})

	runConcurrent(t, "XSS", func(t *testing.T) {
		if !IsXSS(`<script>alert(1)</script>`) {
			t.Error("IsXSS: expected true for attack input")
		}

		if IsXSS(`hello world`) {
			t.Error("IsXSS: expected false for clean input")
		}
	})
}
