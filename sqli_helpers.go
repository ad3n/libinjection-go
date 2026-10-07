package libinjection

import (
	"strings"
)

func flag2Delimiter(flag int) byte {
	switch {
	case (flag & sqliFlagQuoteSingle) != 0:
		return byteSingle
	case (flag & sqliFlagQuoteDouble) != 0:
		return byteDouble
	default:
		return byteNull
	}
}

func isBackslashEscaped(str string) bool {
	count := 0
	for i := len(str) - 1; i >= 0; i-- {
		if str[i] != '\\' {
			break
		}

		count++
	}

	return count%2 != 0
}

func isDoubleDelimiterEscaped(str string) bool {
	return len(str) >= 2 && str[0] == str[1]
}

func isByteWhite(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\v' || ch == '\f' || ch == '\r' || ch == '\240' || ch == '\000'
}

func strLenSpn(s string, length int, accept string) int {
	for i := range length {
		if strings.IndexByte(accept, s[i]) == -1 {
			return i
		}
	}

	return length
}

func strLenCSpn(s string, length int, accept []byte) int {
	for i := range length {
		if accept[s[i]] == 1 {
			return i
		}
	}

	return length
}

func isMysqlComment(s string, pos int) bool {
	if pos+2 >= len(s) {
		return false
	}

	if s[pos+2] != '!' {
		return false
	}

	return true
}

func toUpperCmp(expectedUpper, s string) bool {
	if len(expectedUpper) != len(s) {
		return false
	}

	for i := 0; i < len(expectedUpper); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 0x20
		}

		if expectedUpper[i] != c {
			return false
		}
	}

	return true
}

func searchKeyword(key string, keywords map[string]byte) byte {
	if len(key) > tokenSize {
		return keywords[strings.ToUpper(key)]
	}

	var upper [tokenSize]byte
	for i := range len(key) {
		c := key[i]
		if c >= 0x80 {
			return keywords[strings.ToUpper(key)]
		}

		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}

		upper[i] = c
	}

	return keywords[string(upper[:len(key)])]
}
