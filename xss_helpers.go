package libinjection

import (
	"slices"
	"strings"
)

const maxNormalizedTokenLen = 64

func isH5White(ch byte) bool {
	return ch == '\n' || ch == '\t' || ch == '\v' || ch == '\f' || ch == '\r' || ch == ' '
}

func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 0x20
		}

		if cb >= 'A' && cb <= 'Z' {
			cb += 0x20
		}

		if ca != cb {
			return false
		}
	}

	return true
}

func upperRemoveNulls(buf []byte, s string) (n int, truncated bool) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0 {
			continue
		}

		if n == len(buf) {
			truncated = true
			break
		}

		if c >= 'a' && c <= 'z' {
			c -= 0x20
		}

		buf[n] = c
		n++
	}

	return n, truncated
}

//nolint:gocyclo // complexity 12, reduction tracked in #122
func isBlackTag(s string) bool {
	if len(s) < 3 {
		return false
	}

	var buf [maxNormalizedTokenLen]byte
	n, truncated := upperRemoveNulls(buf[:], s)
	if truncated {
		return false
	}

	normalized := buf[:n]

	if slices.Contains(blackTags, string(normalized)) {
		return true
	}

	if n >= 3 && ((normalized[0] == 'S' && normalized[1] == 'V' && normalized[2] == 'G') ||
		(normalized[0] == 'X' && normalized[1] == 'S' && normalized[2] == 'L')) {
		return true
	}

	return false
}

//nolint:gocyclo // complexity 12, reduction tracked in #122
func isBlackAttr(s string) int {
	var buf [maxNormalizedTokenLen]byte
	n, truncated := upperRemoveNulls(buf[:], s)
	if truncated {
		return attributeTypeNone
	}

	if n < 2 {
		return attributeTypeNone
	}

	normalized := buf[:n]

	if n >= 5 {
		if string(normalized) == "XMLNS" || string(normalized) == "XLINK" {
			return attributeTypeBlack
		}

		if buf[0] == 'O' && buf[1] == 'N' {
			if typ, ok := blackEventsMap[string(buf[2:n])]; ok {
				return typ
			}
		}
	}

	if typ, ok := blacksMap[string(normalized)]; ok {
		return typ
	}

	return attributeTypeNone
}

//nolint:gocyclo // complexity 21, reduction tracked in #125
func htmlDecodeByteAt(s string) (int, int) {
	length := len(s)
	val := 0

	if length == 0 {
		return byteEOF, 0
	}

	if s[0] != '&' || length < 2 {
		return int(s[0]), 1
	}

	if s[1] != '#' || len(s) < 3 {
		return '&', 1
	}

	if s[2] == 'x' || s[2] == 'X' {
		if len(s) < 4 {
			return '&', 1
		}

		ch := int(s[3])
		ch = gsHexDecodeMap[ch]
		if ch == 256 {
			return '&', 1
		}

		val = ch
		i := 4

		for i < length {
			ch = int(s[i])
			if ch == ';' {
				return val, i + 1
			}

			ch = gsHexDecodeMap[ch]
			if ch == 256 {
				return val, i
			}

			val = val*16 + ch
			if val > 0x1000FF {
				return '&', 1
			}

			i++
		}

		return val, i
	}

	i := 2
	ch := int(s[i])
	if ch < '0' || ch > '9' {
		return '&', 1
	}

	val = ch - '0'
	i++
	for i < length {
		ch = int(s[i])
		if ch == ';' {
			return val, i + 1
		}

		if ch < '0' || ch > '9' {
			return val, i
		}

		val = val*10 + (ch - '0')
		if val > 0x1000FF {
			return '&', 1
		}

		i++
	}

	return val, i
}

//nolint:gocyclo // complexity 10, reduction tracked in #122
func htmlEncodeStartsWith(a, b string) bool {
	var (
		first  = true
		pos    = 0
		length = len(b)
		ai     = 0
	)

	for length > 0 {
		cb, consumed := htmlDecodeByteAt(b[pos:])
		pos += consumed
		length -= consumed

		if first && cb <= 32 {
			continue
		}

		first = false

		if cb == 0 || cb == 10 {
			continue
		}

		if cb >= 'a' && cb <= 'z' {
			cb -= 0x20
		}

		ch := byte(cb & 0xFF)

		if ai >= len(a) {
			return true
		}

		if ch != a[ai] {
			return false
		}

		ai++
	}

	return ai >= len(a)
}

func isBlackURL(s string) bool {
	urls := []string{
		"DATA",
		"VIEW-SOURCE",
		"VBSCRIPT",
		"JAVA",
	}

	str := strings.TrimLeftFunc(s, func(r rune) bool {
		return r <= 32 || r >= 127
	})

	for _, url := range urls {
		if htmlEncodeStartsWith(url, str) {
			return true
		}
	}

	return false
}
