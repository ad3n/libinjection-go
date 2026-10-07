package libinjection

import (
	"bytes"
	"strings"
)

var (
	wordAcceptTable = buildAcceptTable(" []{}<>:\\?=@!#~+-*/&|^%(),';\t\n\v\f\r\"\240\000")
	varAcceptTable  = buildAcceptTable(" <>:\\?=@!#~+-*/&|^%(),';\t\n\v\f\r'`\"")
)

func parseEolComment(s *sqliState) int {
	index := strings.IndexByte(s.input[s.pos:], '\n')

	if index == -1 {
		s.current.assign(sqliTokenTypeComment, s.pos, s.length-s.pos, s.input[s.pos:])
		return s.length
	}

	s.current.assign(sqliTokenTypeComment, s.pos, index, s.input[s.pos:])
	return s.pos + index + 1
}

//nolint:gocyclo // complexity 11, reduction tracked in #122
func parseMoney(s *sqliState) int {
	if s.pos+1 == s.length {
		s.current.assign(sqliTokenTypeBareWord, s.pos, 1, "$")
		return s.length
	}

	length := strLenSpn(s.input[s.pos+1:], s.length-s.pos-1, "0123456789.,")
	switch {
	case length == 0:
		if s.input[s.pos+1] == '$' {
			index := strings.Index(s.input[s.pos+2:], "$$")
			if index == -1 {
				s.current.assign(sqliTokenTypeString, s.pos+2, s.length-(s.pos+2), s.input[s.pos+2:])
				s.current.strOpen = '$'
				s.current.strClose = byteNull
				return s.length
			}

			s.current.assign(sqliTokenTypeString, s.pos+2, index, s.input[s.pos+2:])
			s.current.strOpen = '$'
			s.current.strClose = '$'
			return s.pos + 2 + index + 2
		}

		xlen := strLenSpn(s.input[s.pos+1:], s.length-s.pos-1, "abcdefghjiklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
		if xlen == 0 {
			s.current.assign(sqliTokenTypeBareWord, s.pos, 1, "$")
			return s.pos + 1
		}

		if s.pos+xlen+1 == s.length || s.input[s.pos+xlen+1] != '$' {
			s.current.assign(sqliTokenTypeBareWord, s.pos, 1, "$")
			return s.pos + 1
		}

		index := strings.Index(s.input[s.pos+xlen+2:], s.input[s.pos:s.pos+xlen+2])
		if index == -1 {
			s.current.assign(sqliTokenTypeString, s.pos+xlen+2, s.length-s.pos-xlen-2, s.input[s.pos+xlen+2:])
			s.current.strOpen = '$'
			s.current.strClose = byteNull
			return s.length
		}

		s.current.assign(sqliTokenTypeString, s.pos+xlen+2, index, s.input[s.pos+xlen+2:])
		s.current.strOpen = '$'
		s.current.strClose = '$'
		return s.pos + xlen + 2 + index + xlen + 2
	case length == 1 && s.input[s.pos+1] == '.':
		return parseWord(s)
	default:
		s.current.assign(sqliTokenTypeNumber, s.pos, length+1, s.input[s.pos:])
		return s.pos + length + 1
	}
}

func parseOther(s *sqliState) int {
	s.current.assign(sqliTokenTypeUnknown, s.pos, 1, s.input[s.pos:])
	return s.pos + 1
}

func parseWhite(s *sqliState) int {
	return s.pos + 1
}

func parseOperator1(s *sqliState) int {
	s.current.assign(sqliTokenTypeOperator, s.pos, 1, s.input[s.pos:])
	return s.pos + 1
}

func parseByte(s *sqliState) int {
	s.current.assign(s.input[s.pos], s.pos, 1, s.input[s.pos:])
	return s.pos + 1
}

func parseHash(s *sqliState) int {
	s.statsCommentHash++
	if (s.flags & sqliFlagSQLMysql) != 0 {
		s.statsCommentHash++
		return parseEolComment(s)
	}

	s.current.assign(sqliTokenTypeOperator, s.pos, 1, "#")
	return s.pos + 1
}

//nolint:gocyclo // complexity 9, reduction tracked in #122
func parseDash(s *sqliState) int {
	switch {
	case s.pos+2 < s.length && s.input[s.pos+1] == '-' && isByteWhite(s.input[s.pos+2]):
		return parseEolComment(s)
	case s.pos+2 == s.length && s.input[s.pos+1] == '-':
		return parseEolComment(s)
	case s.pos+1 < s.length && s.input[s.pos+1] == '-' && (s.flags&sqliFlagSQLAnsi) != 0:
		s.statsCommentDDX++
		return parseEolComment(s)
	default:
		s.current.assign(sqliTokenTypeOperator, s.pos, 1, "-")
		return s.pos + 1
	}
}

func parseSlash(s *sqliState) int {
	var (
		length int
		ctype  = sqliTokenTypeComment
	)
	if s.pos+1 == s.length || s.input[s.pos+1] != '*' {
		return parseOperator1(s)
	}

	index := strings.Index(s.input[s.pos+2:], "*/")
	switch {
	case index == -1:
		length = s.length - s.pos
	default:
		length = 2 + index + 2
	}

	switch {
	case index != -1 &&
		strings.Contains(s.input[s.pos+2:s.pos+2+index+1], "/*"):
		ctype = sqliTokenTypeEvil
	default:
		if isMysqlComment(s.input, s.pos) {
			ctype = sqliTokenTypeEvil
		}
	}

	s.current.assign(ctype, s.pos, length, s.input[s.pos:])
	return s.pos + length
}

func parseBackSlash(s *sqliState) int {
	if s.pos+1 < s.length && s.input[s.pos+1] == 'N' {
		s.current.assign(sqliTokenTypeNumber, s.pos, 2, s.input[s.pos:])
		return s.pos + 2
	}

	s.current.assign(sqliTokenTypeBackslash, s.pos, 1, s.input[s.pos:])
	return s.pos + 1
}

func parseOperator2(s *sqliState) int {
	if s.pos+1 >= s.length {
		return parseOperator1(s)
	}

	if s.pos+2 < s.length && s.input[s.pos] == '<' && s.input[s.pos+1] == '=' && s.input[s.pos+2] == '>' {
		s.current.assign(sqliTokenTypeOperator, s.pos, 3, s.input[s.pos:])
		return s.pos + 3
	}

	ch := s.lookupWord(sqliLookupOperator, s.input[s.pos:s.pos+2])
	if ch != byteNull {
		s.current.assign(ch, s.pos, 2, s.input[s.pos:])
		return s.pos + 2
	}

	if s.input[s.pos] == ':' {
		s.current.assign(sqliTokenTypeColon, s.pos, 1, s.input[s.pos:])
		return s.pos + 1
	}

	return parseOperator1(s)
}

func parseString(s *sqliState) int {
	return s.current.parseStringCore(s.input, s.length, s.pos, 1, s.input[s.pos])
}

func parseWord(s *sqliState) int {
	length := strLenCSpn(s.input[s.pos:], s.length-s.pos, wordAcceptTable)
	s.current.assign(sqliTokenTypeBareWord, s.pos, length, s.input[s.pos:])

	for i := 0; i < s.current.len; i++ {
		delimiter := s.current.val[i]
		if delimiter == '.' || delimiter == '`' {
			ch := s.lookupWord(sqliLookupWord, s.current.val[:i])
			if ch != sqliTokenTypeNone && ch != sqliTokenTypeBareWord {
				*s.current = sqliToken{}

				s.current.assign(ch, s.pos, i, s.input[s.pos:])
				return s.pos + i
			}
		}
	}

	if length < tokenSize {
		ch := s.lookupWord(sqliLookupWord, s.current.val[:length])
		if ch == byteNull {
			ch = sqliTokenTypeBareWord
		}

		s.current.category = ch
	}

	return s.pos + length
}

func parseVar(s *sqliState) int {
	pos := s.pos + 1
	switch {
	case pos < s.length && s.input[pos] == '@':
		pos++
		s.current.count = 2
	default:
		s.current.count = 1
	}

	if pos < s.length {
		if s.input[pos] == '`' {
			s.pos = pos
			pos = parseTick(s)
			s.current.category = sqliTokenTypeVariable
			return pos
		}

		if s.input[pos] == byteSingle || s.input[pos] == byteDouble {
			s.pos = pos
			pos = parseString(s)
			s.current.category = sqliTokenTypeVariable
			return pos
		}
	}

	length := strLenCSpn(s.input[pos:], s.length-pos, varAcceptTable)
	if length == 0 {
		s.current.assign(sqliTokenTypeVariable, pos, 0, s.input[pos:])
		return pos
	}

	s.current.assign(sqliTokenTypeVariable, pos, length, s.input[pos:])
	return pos + length
}

//nolint:gocyclo // complexity 36, reduction tracked in #126
func parseNumber(s *sqliState) int {
	var (
		digits  string
		haveE   int
		haveExp int
	)

	if s.input[s.pos] == '0' && s.pos+1 < s.length {
		switch {
		case s.input[s.pos+1] == 'X' || s.input[s.pos+1] == 'x':
			digits = "0123456789ABCDEFabcdef"
		default:
			if s.input[s.pos+1] == 'B' || s.input[s.pos+1] == 'b' {
				digits = "01"
			}
		}

		if digits != "" {
			length := strLenSpn(s.input[s.pos+2:], s.length-s.pos-2, digits)
			if length == 0 {
				s.current.assign(sqliTokenTypeBareWord, s.pos, 2, s.input[s.pos:])
				return s.pos + 2
			}

			s.current.assign(sqliTokenTypeNumber, s.pos, 2+length, s.input[s.pos:])
			return s.pos + 2 + length
		}
	}

	pos := s.pos
	start := s.pos
	for pos < s.length && s.input[pos]-'0' <= 9 {
		pos++
	}

	if pos < s.length && s.input[pos] == '.' {
		pos++
		for pos < s.length && s.input[pos]-'0' <= 9 {
			pos++
		}

		if pos-start == 1 {
			s.current.assign(sqliTokenTypeDot, start, 1, ".")
			return pos
		}
	}

	if pos < s.length {
		if s.input[pos] == 'E' || s.input[pos] == 'e' {
			haveE = 1
			pos++

			if pos < s.length && (s.input[pos] == '+' || s.input[pos] == '-') {
				pos++
			}

			for pos < s.length && s.input[pos]-'0' <= 9 {
				haveExp = 1
				pos++
			}
		}
	}

	if pos < s.length && (s.input[pos] == 'd' || s.input[pos] == 'D' || s.input[pos] == 'f' || s.input[pos] == 'F') {
		switch {
		case pos+1 == s.length:
			pos++
		case isByteWhite(s.input[pos+1]) || s.input[pos+1] == ';':
			pos++
		case s.input[pos+1] == 'u' || s.input[pos+1] == 'U':
			pos++
		default:
		}
	}

	if !(haveE == 1 && haveExp == 0) {
		s.current.assign(sqliTokenTypeNumber, start, pos-start, s.input[start:])
	}

	return pos
}

func parseTick(s *sqliState) int {
	pos := s.current.parseStringCore(s.input, s.length, s.pos, 1, byteTick)

	ch := s.lookupWord(sqliLookupWord, s.current.val[:s.current.len])
	switch {
	case ch == sqliTokenTypeFunction:
		s.current.category = sqliTokenTypeFunction
	default:
		s.current.category = sqliTokenTypeBareWord
	}

	return pos
}

func parseUString(s *sqliState) int {
	pos := s.pos
	if pos+2 < s.length && s.input[pos+1] == '&' && s.input[pos+2] == byteSingle {
		s.pos += 2
		pos = parseString(s)
		s.current.strOpen = 'u'
		if s.current.strClose == byteSingle {
			s.current.strClose = 'u'
		}

		return pos
	}

	return parseWord(s)
}

func parseQString(s *sqliState) int {
	return parseQStringCore(s, 0)
}

func parseNqString(s *sqliState) int {
	if s.pos+2 < s.length && s.input[s.pos+1] == byteSingle {
		return parseEString(s)
	}

	return parseQStringCore(s, 1)
}

func parseXString(s *sqliState) int {
	if s.pos+2 >= s.length || s.input[s.pos+1] != byteSingle {
		return parseWord(s)
	}

	length := strLenSpn(s.input[s.pos+2:], s.length-s.pos-2, "0123456789abcdefABCDEF")
	if s.pos+2+length >= s.length || s.input[s.pos+2+length] != byteSingle {
		return parseWord(s)
	}

	s.current.assign(sqliTokenTypeNumber, s.pos, length+3, s.input[s.pos:])
	return s.pos + 2 + length + 1
}

func parseBString(s *sqliState) int {
	if s.pos+2 >= s.length || s.input[s.pos+1] != byteSingle {
		return parseWord(s)
	}

	length := strLenSpn(s.input[s.pos+2:], s.length-s.pos-2, "01")
	if s.pos+2+length >= s.length || s.input[s.pos+2+length] != byteSingle {
		return parseWord(s)
	}

	s.current.assign(sqliTokenTypeNumber, s.pos, length+3, s.input[s.pos:])
	return s.pos + 2 + length + 1
}

func parseEString(s *sqliState) int {
	if s.pos+2 >= s.length || s.input[s.pos+1] != byteSingle {
		return parseWord(s)
	}

	return s.current.parseStringCore(s.input, s.length, s.pos, 2, byteSingle)
}

func parseBWord(s *sqliState) int {
	end := strings.IndexByte(s.input[s.pos:], ']')
	if end == -1 {
		s.current.assign(sqliTokenTypeBareWord, s.pos, s.length-s.pos, s.input[s.pos:])
		return s.length
	}

	s.current.assign(sqliTokenTypeBareWord, s.pos, end+1, s.input[s.pos:])
	return s.pos + end + 1
}

func buildAcceptTable(acceptStr string) []byte {
	accept := []byte(acceptStr)
	acceptTable := make([]byte, 256)
	for i := range acceptTable {
		if i < len(acceptTable) && bytes.IndexByte(accept, byte(i)) != -1 {
			acceptTable[i] = 1
		}
	}

	return acceptTable
}
