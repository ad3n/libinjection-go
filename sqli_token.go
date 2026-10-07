package libinjection

import "strings"

type sqliToken struct {
	pos int
	len int

	count int

	category byte
	strOpen  byte
	strClose byte
	val      string
}

const (
	maxTokens = 5
	tokenSize = 32
)

func (t *sqliToken) parseStringCore(s string, length, pos, offset int, delimiter byte) int {
	str := s[pos+offset:]
	switch {
	case offset > 0:
		t.strOpen = delimiter
	default:
		t.strOpen = byteNull
	}

	for {
		index := strings.IndexByte(str, delimiter)
		if index != -1 {
			str = str[index:]
		}

		switch {
		case index == -1:
			t.assign(sqliTokenTypeString, pos+offset, length-pos-offset, s[pos+offset:])
			t.strClose = byteNull
			return length
		case isBackslashEscaped(s[pos+offset : len(s)-len(str)]):
			str = str[1:]
			continue
		case isDoubleDelimiterEscaped(str):
			str = str[2:]
			continue
		default:
			t.assign(sqliTokenTypeString, pos+offset, len(s[pos+offset:])-len(str), s[pos+offset:])
			t.strClose = delimiter
			return len(s) - len(str) + 1
		}
	}
}

func (t *sqliToken) assign(tokenType byte, pos, length int, value string) {
	var last int
	switch {
	case length < tokenSize:
		last = length
	default:
		last = tokenSize - 1
	}

	t.category = tokenType
	t.pos = pos
	t.len = last
	t.val = value[:last]
}

//nolint:gocyclo // complexity 9, reduction tracked in #122
func (t *sqliToken) isUnaryOp() bool {
	if t.category != sqliTokenTypeOperator {
		return false
	}

	switch t.len {
	case 1:
		return t.val[0] == '+' || t.val[0] == '-' || t.val[0] == '!' || t.val[0] == '~'
	case 2:
		return t.val[0] == '!' && t.val[1] == '!'
	case 3:
		return toUpperCmp("NOT", t.val[:3])
	default:
		return false
	}
}

func (t *sqliToken) isArithmeticOp() bool {
	return t.category == sqliTokenTypeOperator && t.len == 1 &&
		(t.val[0] == '*' || t.val[0] == '/' || t.val[0] == '+' || t.val[0] == '-' || t.val[0] == '%')
}
