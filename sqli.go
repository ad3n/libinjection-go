package libinjection

import (
	"strings"
	"sync"
)

type sqliState struct {
	input string

	length int

	flags int

	pos int

	tokenVec [8]sqliToken

	current *sqliToken

	fingerprint    [maxTokens]byte
	fingerprintLen int

	statsCommentDDX int

	statsCommentHash int

	statsFolds int

	statsTokens int
}

func sqliInit(s *sqliState, input string, flags int) {
	if flags == 0 {
		flags = sqliFlagQuoteNone | sqliFlagSQLAnsi
	}

	*s = sqliState{}

	s.input = input
	s.length = len(input)
	s.flags = flags
	s.current = &s.tokenVec[0]
}

func (s *sqliState) computeFingerprint(flags int) {
	s.reset(flags)
	length := s.fold()

	if length > 2 &&
		s.tokenVec[length-1].category == sqliTokenTypeBareWord &&
		s.tokenVec[length-1].strOpen == byteTick &&
		s.tokenVec[length-1].len == 0 &&
		s.tokenVec[length-1].strClose == byteNull {
		s.tokenVec[length-1].category = sqliTokenTypeComment
	}

	for i := range length {
		c := s.tokenVec[i].category

		if c == sqliTokenTypeEvil {
			s.fingerprint[0] = sqliTokenTypeEvil
			s.fingerprintLen = 1
			s.tokenVec[0].category = sqliTokenTypeEvil
			s.tokenVec[0].val = string(sqliTokenTypeEvil)
			return
		}

		s.fingerprint[i] = c
	}

	s.fingerprintLen = length
}

//nolint:gocyclo // complexity 20, reduction tracked in #124
func (s *sqliState) merge(tokenA, tokenB *sqliToken) bool {
	if !(tokenA.category == sqliTokenTypeKeyword ||
		tokenA.category == sqliTokenTypeBareWord ||
		tokenA.category == sqliTokenTypeOperator ||
		tokenA.category == sqliTokenTypeUnion ||
		tokenA.category == sqliTokenTypeFunction ||
		tokenA.category == sqliTokenTypeExpression ||
		tokenA.category == sqliTokenTypeTSQL ||
		tokenA.category == sqliTokenTypeSQLType) {
		return false
	}

	if !(tokenB.category == sqliTokenTypeKeyword ||
		tokenB.category == sqliTokenTypeBareWord ||
		tokenB.category == sqliTokenTypeOperator ||
		tokenB.category == sqliTokenTypeUnion ||
		tokenB.category == sqliTokenTypeFunction ||
		tokenB.category == sqliTokenTypeExpression ||
		tokenB.category == sqliTokenTypeTSQL ||
		tokenB.category == sqliTokenTypeSQLType ||
		tokenB.category == sqliTokenTypeLogicOperator) {
		return false
	}

	if tokenA.len+tokenB.len+1 > tokenSize {
		return false
	}

	var buf [tokenSize]byte
	length := copy(buf[:], tokenA.val[:tokenA.len])
	buf[length] = ' '
	length++
	length += copy(buf[length:], tokenB.val[:tokenB.len])
	ch := s.lookupWord(sqliLookupWord, string(buf[:length]))
	if ch == byteNull {
		return false
	}

	tokenA.assign(ch, tokenA.pos, length, string(buf[:length]))
	return true
}

//nolint:gocyclo // complexity 178, reduction tracked in #129
func (s *sqliState) fold() int {
	var (
		pos         = 0
		left        = 0
		more        = true
		lastComment = sqliToken{}
	)

	s.current = &s.tokenVec[0]
	for more {
		more = s.tokenize()
		if !(s.current.category == sqliTokenTypeComment ||
			s.current.category == sqliTokenTypeLeftParenthesis ||
			s.current.category == sqliTokenTypeSQLType ||
			s.current.isUnaryOp()) {
			break
		}
	}

	if !more {
		return 0
	}

	pos++

	for {
		if pos >= maxTokens {
			if (s.tokenVec[0].category == sqliTokenTypeNumber &&
				(s.tokenVec[1].category == sqliTokenTypeOperator || s.tokenVec[1].category == sqliTokenTypeComma) &&
				s.tokenVec[2].category == sqliTokenTypeLeftParenthesis &&
				s.tokenVec[3].category == sqliTokenTypeNumber &&
				s.tokenVec[4].category == sqliTokenTypeRightParenthesis) ||
				(s.tokenVec[0].category == sqliTokenTypeBareWord &&
					s.tokenVec[1].category == sqliTokenTypeOperator &&
					s.tokenVec[2].category == sqliTokenTypeLeftParenthesis &&
					(s.tokenVec[3].category == sqliTokenTypeBareWord || s.tokenVec[3].category == sqliTokenTypeNumber) &&
					s.tokenVec[4].category == sqliTokenTypeRightParenthesis) ||
				(s.tokenVec[0].category == sqliTokenTypeNumber &&
					s.tokenVec[1].category == sqliTokenTypeRightParenthesis &&
					s.tokenVec[2].category == sqliTokenTypeComma &&
					s.tokenVec[3].category == sqliTokenTypeLeftParenthesis &&
					s.tokenVec[4].category == sqliTokenTypeNumber) ||
				(s.tokenVec[0].category == sqliTokenTypeBareWord &&
					s.tokenVec[1].category == sqliTokenTypeRightParenthesis &&
					s.tokenVec[2].category == sqliTokenTypeOperator &&
					s.tokenVec[3].category == sqliTokenTypeLeftParenthesis &&
					s.tokenVec[4].category == sqliTokenTypeBareWord) {
				switch {
				case pos > maxTokens:
					s.tokenVec[1] = s.tokenVec[maxTokens]
					left = 1
					pos = 2
				default:
					pos = 1
					left = 0
				}
			}
		}

		if !more || left >= maxTokens {
			left = pos
			break
		}

		for more && pos <= maxTokens && pos-left < 2 {
			s.current = &s.tokenVec[pos]
			more = s.tokenize()
			if more {
				switch {
				case s.current.category == sqliTokenTypeComment:
					lastComment = *s.current
				default:
					lastComment.category = byteNull
					pos++
				}
			}
		}

		if pos-left < 2 {
			left = pos
			continue
		}

		switch {
		case s.tokenVec[left].category == sqliTokenTypeString && s.tokenVec[left+1].category == sqliTokenTypeString:
			pos--
			s.statsFolds++
			continue
		case s.tokenVec[left].category == sqliTokenTypeSemiColon && s.tokenVec[left+1].category == sqliTokenTypeSemiColon:
			pos--
			s.statsFolds++
			continue
		case (s.tokenVec[left].category == sqliTokenTypeOperator || s.tokenVec[left].category == sqliTokenTypeLogicOperator) &&
			(s.tokenVec[left+1].isUnaryOp() || s.tokenVec[left+1].category == sqliTokenTypeSQLType):
			pos--
			s.statsFolds++
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeLeftParenthesis &&
			s.tokenVec[left+1].isUnaryOp():
			pos--
			s.statsFolds++
			if left > 0 {
				left--
			}

			continue
		case s.merge(&s.tokenVec[left], &s.tokenVec[left+1]):
			pos--
			s.statsFolds++
			if left > 0 {
				left--
			}

			continue
		case s.tokenVec[left].category == sqliTokenTypeSemiColon &&
			s.tokenVec[left+1].category == sqliTokenTypeFunction &&
			(s.tokenVec[left+1].val[0] == 'I' || s.tokenVec[left+1].val[0] == 'i') &&
			(s.tokenVec[left+1].val[1] == 'F' || s.tokenVec[left+1].val[1] == 'f'):
			s.tokenVec[left+1].category = sqliTokenTypeTSQL

			continue
		case (s.tokenVec[left].category == sqliTokenTypeBareWord || s.tokenVec[left].category == sqliTokenTypeVariable) &&
			s.tokenVec[left+1].category == sqliTokenTypeLeftParenthesis &&
			(toUpperCmp("USER_ID", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("USER_NAME", s.tokenVec[left].val[:s.tokenVec[left].len]) ||

				toUpperCmp("DATABASE", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("PASSWORD", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("USER", s.tokenVec[left].val[:s.tokenVec[left].len]) ||

				toUpperCmp("CURRENT_USER", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("CURRENT_DATE", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("CURRENT_TIME", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("CURRENT_TIMESTAMP", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("LOCALTIME", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("LOCALTIMESTAMP", s.tokenVec[left].val[:s.tokenVec[left].len])):
			s.tokenVec[left].category = sqliTokenTypeFunction
			continue
		case s.tokenVec[left].category == sqliTokenTypeKeyword &&
			(toUpperCmp("IN", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("NOT IN", s.tokenVec[left].val[:s.tokenVec[left].len])):
			switch {
			case s.tokenVec[left+1].category == sqliTokenTypeLeftParenthesis:
				s.tokenVec[left].category = sqliTokenTypeOperator
			default:
				s.tokenVec[left].category = sqliTokenTypeBareWord
			}

			continue
		case s.tokenVec[left].category == sqliTokenTypeOperator &&
			(toUpperCmp("LIKE", s.tokenVec[left].val[:s.tokenVec[left].len]) ||
				toUpperCmp("NOT LIKE", s.tokenVec[left].val[:s.tokenVec[left].len])):
			if s.tokenVec[left+1].category == sqliTokenTypeLeftParenthesis {
				s.tokenVec[left].category = sqliTokenTypeFunction
			}
		case s.tokenVec[left].category == sqliTokenTypeSQLType &&
			(s.tokenVec[left+1].category == sqliTokenTypeBareWord ||
				s.tokenVec[left+1].category == sqliTokenTypeNumber ||
				s.tokenVec[left+1].category == sqliTokenTypeSQLType ||
				s.tokenVec[left+1].category == sqliTokenTypeLeftParenthesis ||
				s.tokenVec[left+1].category == sqliTokenTypeFunction ||
				s.tokenVec[left+1].category == sqliTokenTypeVariable ||
				s.tokenVec[left+1].category == sqliTokenTypeString):
			s.tokenVec[left] = s.tokenVec[left+1]
			pos--
			s.statsFolds++
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeCollate && s.tokenVec[left+1].category == sqliTokenTypeBareWord:
			if strings.IndexByte(s.tokenVec[left+1].val[:s.tokenVec[left+1].len], '_') != -1 {
				s.tokenVec[left+1].category = sqliTokenTypeSQLType
				left = 0
			}
		case s.tokenVec[left].category == sqliTokenTypeBackslash:
			switch {
			case s.tokenVec[left+1].isArithmeticOp():
				s.tokenVec[left].category = sqliTokenTypeNumber
			default:
				s.tokenVec[left] = s.tokenVec[left+1]
				pos--
				s.statsFolds++
			}

			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeLeftParenthesis &&
			s.tokenVec[left+1].category == sqliTokenTypeLeftParenthesis:
			pos--
			left = 0
			s.statsFolds++
			continue
		case s.tokenVec[left].category == sqliTokenTypeRightParenthesis &&
			s.tokenVec[left+1].category == sqliTokenTypeRightParenthesis:
			pos--
			left = 0
			s.statsFolds++
			continue
		case s.tokenVec[left].category == sqliTokenTypeLeftBrace &&
			s.tokenVec[left+1].category == sqliTokenTypeBareWord:
			if s.tokenVec[left+1].len == 0 {
				s.tokenVec[left+1].category = sqliTokenTypeEvil
				return left + 2
			}

			left = 0
			pos -= 2
			s.statsFolds += 2
			continue
		case s.tokenVec[left+1].category == sqliTokenTypeRightBrace:
			pos--
			left = 0
			s.statsFolds++
			continue
		}

		for more && pos <= maxTokens && pos-left < 3 {
			s.current = &s.tokenVec[pos]
			more = s.tokenize()
			if more {
				switch {
				case s.current.category == sqliTokenTypeComment:
					lastComment = *s.current
				default:
					lastComment.category = byteNull
					pos++
				}
			}
		}

		if pos-left < 3 {
			left = pos
			continue
		}

		switch {
		case s.tokenVec[left].category == sqliTokenTypeNumber &&
			s.tokenVec[left+1].category == sqliTokenTypeOperator &&
			s.tokenVec[left+2].category == sqliTokenTypeNumber:
			pos -= 2
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeOperator &&
			s.tokenVec[left+1].category != sqliTokenTypeLeftParenthesis &&
			s.tokenVec[left+2].category == sqliTokenTypeOperator:
			pos -= 2
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeLogicOperator &&
			s.tokenVec[left+2].category == sqliTokenTypeLogicOperator:
			pos -= 2
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeVariable &&
			s.tokenVec[left+1].category == sqliTokenTypeOperator &&
			(s.tokenVec[left+2].category == sqliTokenTypeVariable ||
				s.tokenVec[left+2].category == sqliTokenTypeNumber ||
				s.tokenVec[left+2].category == sqliTokenTypeBareWord):
			pos -= 2
			left = 0
			continue
		case (s.tokenVec[left].category == sqliTokenTypeBareWord ||
			s.tokenVec[left].category == sqliTokenTypeNumber) &&
			s.tokenVec[left+1].category == sqliTokenTypeOperator &&
			(s.tokenVec[left+2].category == sqliTokenTypeNumber ||
				s.tokenVec[left+2].category == sqliTokenTypeBareWord):
			pos -= 2
			left = 0
			continue
		case (s.tokenVec[left].category == sqliTokenTypeBareWord ||
			s.tokenVec[left].category == sqliTokenTypeNumber ||
			s.tokenVec[left].category == sqliTokenTypeVariable ||
			s.tokenVec[left].category == sqliTokenTypeString) &&
			s.tokenVec[left+1].category == sqliTokenTypeOperator &&
			s.tokenVec[left+1].val[:s.tokenVec[left+1].len] == "::" &&
			s.tokenVec[left+2].category == sqliTokenTypeSQLType:
			pos -= 2
			left = 0
			s.statsFolds += 2
			continue
		case (s.tokenVec[left].category == sqliTokenTypeBareWord ||
			s.tokenVec[left].category == sqliTokenTypeNumber ||
			s.tokenVec[left].category == sqliTokenTypeString ||
			s.tokenVec[left].category == sqliTokenTypeVariable) &&
			s.tokenVec[left+1].category == sqliTokenTypeComma &&
			(s.tokenVec[left+2].category == sqliTokenTypeNumber ||
				s.tokenVec[left+2].category == sqliTokenTypeBareWord ||
				s.tokenVec[left+2].category == sqliTokenTypeString ||
				s.tokenVec[left+2].category == sqliTokenTypeVariable):
			pos -= 2
			left = 0
			continue
		case (s.tokenVec[left].category == sqliTokenTypeExpression ||
			s.tokenVec[left].category == sqliTokenTypeGroup ||
			s.tokenVec[left].category == sqliTokenTypeComma) &&
			s.tokenVec[left+1].isUnaryOp() &&
			s.tokenVec[left+2].category == sqliTokenTypeLeftParenthesis:
			s.tokenVec[left+1] = s.tokenVec[left+2]
			pos--
			left = 0
			continue
		case (s.tokenVec[left].category == sqliTokenTypeKeyword ||
			s.tokenVec[left].category == sqliTokenTypeExpression ||
			s.tokenVec[left].category == sqliTokenTypeGroup) &&
			s.tokenVec[left+1].isUnaryOp() &&
			(s.tokenVec[left+2].category == sqliTokenTypeNumber ||
				s.tokenVec[left+2].category == sqliTokenTypeBareWord ||
				s.tokenVec[left+2].category == sqliTokenTypeVariable ||
				s.tokenVec[left+2].category == sqliTokenTypeString ||
				s.tokenVec[left+2].category == sqliTokenTypeFunction):
			s.tokenVec[left+1] = s.tokenVec[left+2]
			pos--
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeComma &&
			s.tokenVec[left+1].isUnaryOp() &&
			(s.tokenVec[left+2].category == sqliTokenTypeNumber ||
				s.tokenVec[left+2].category == sqliTokenTypeBareWord ||
				s.tokenVec[left+2].category == sqliTokenTypeVariable ||
				s.tokenVec[left+2].category == sqliTokenTypeString):
			s.tokenVec[left+1] = s.tokenVec[left+2]
			left = 0
			pos -= 3
			continue
		case s.tokenVec[left].category == sqliTokenTypeComma &&
			s.tokenVec[left+1].isUnaryOp() &&
			s.tokenVec[left+2].category == sqliTokenTypeFunction:
			s.tokenVec[left+1] = s.tokenVec[left+2]
			pos--
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeBareWord &&
			s.tokenVec[left+1].category == sqliTokenTypeDot &&
			s.tokenVec[left+2].category == sqliTokenTypeBareWord:
			pos -= 2
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeExpression &&
			s.tokenVec[left+1].category == sqliTokenTypeDot &&
			s.tokenVec[left+2].category == sqliTokenTypeBareWord:
			s.tokenVec[left+1] = s.tokenVec[left+2]
			pos--
			left = 0
			continue
		case s.tokenVec[left].category == sqliTokenTypeFunction &&
			s.tokenVec[left+1].category == sqliTokenTypeLeftParenthesis &&
			s.tokenVec[left+2].category != sqliTokenTypeRightParenthesis:
			if toUpperCmp("USER", s.tokenVec[left].val[:s.tokenVec[left].len]) {
				s.tokenVec[left].category = sqliTokenTypeBareWord
			}
		}

		left++
	}

	if left < maxTokens && lastComment.category == sqliTokenTypeComment {
		s.tokenVec[left] = lastComment
		left++
	}

	if left > maxTokens {
		left = maxTokens
	}

	return left
}

func (s *sqliState) tokenize() bool {
	if s.length == 0 {
		return false
	}

	*s.current = sqliToken{}

	if s.pos == 0 && (s.flags&(sqliFlagQuoteSingle|sqliFlagQuoteDouble)) != 0 {
		s.pos = s.current.parseStringCore(s.input, s.length, 0, 0, flag2Delimiter(s.flags))
		s.statsTokens++
		return true
	}

	for s.pos < s.length {
		ch := s.input[s.pos]

		s.pos = parseByteFunctions(s, ch)

		if s.current.category != byteNull {
			s.statsTokens++
			return true
		}
	}

	return false
}

func (s *sqliState) blacklist() bool {
	length := s.fingerprintLen
	if length < 1 {
		return false
	}

	var buf [maxTokens + 1]byte
	buf[0] = '0'
	upper := buf[1 : length+1]
	for i := range length {
		ch := s.fingerprint[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 0x20
		}

		upper[i] = ch
	}

	fp := buf[:length+1]
	if val, ok := sqlKeywords[string(fp)]; ok {
		return val == sqliTokenTypeFingerprint
	}

	return false
}

//nolint:gocyclo // complexity 34, reduction tracked in #127
func (s *sqliState) notWhitelist() bool {
	length := s.fingerprintLen

	if length > 1 && s.fingerprint[length-1] == sqliTokenTypeComment {
		if strings.Contains(s.input, "sp_password") {
			return true
		}
	}

	switch length {
	case 2:
		if s.fingerprint[1] == sqliTokenTypeUnion {
			return s.statsTokens != 2
		}

		if s.tokenVec[1].val[0] == '#' {
			return false
		}

		if s.tokenVec[0].category == sqliTokenTypeBareWord &&
			s.tokenVec[1].category == sqliTokenTypeComment &&
			s.tokenVec[1].val[0] != '/' {
			return false
		}

		if s.tokenVec[0].category == sqliTokenTypeNumber &&
			s.tokenVec[1].category == sqliTokenTypeComment &&
			s.tokenVec[1].val[0] != '/' {
			return true
		}

		if s.tokenVec[0].category == sqliTokenTypeNumber &&
			s.tokenVec[1].category == sqliTokenTypeComment {
			if s.statsTokens > 2 {
				return true
			}

			ch := s.input[s.tokenVec[0].len]
			if ch <= 32 {
				return true
			}

			if ch == '/' && s.input[s.tokenVec[0].len+1] == '*' {
				return true
			}

			if ch == '-' && s.input[s.tokenVec[0].len+1] == '-' {
				return true
			}

			return false
		}

		if s.tokenVec[1].len > 2 && s.tokenVec[1].val[0] == '-' {
			return false
		}

	case 3:
		switch string(s.fingerprint[:length]) {
		case "sos", "s&s":
			if s.tokenVec[0].strOpen == byteNull &&
				s.tokenVec[2].strClose == byteNull &&
				s.tokenVec[0].strClose == s.tokenVec[2].strOpen {
				return true
			}

			if s.statsTokens == 3 {
				return false
			}

			return false
		case "s&n", "n&1", "1&1", "1&v", "1&s":
			if s.statsTokens == 3 {
				return false
			}
		}

		if s.tokenVec[1].category == sqliTokenTypeKeyword && (s.tokenVec[1].len < 5 || !toUpperCmp("INTO", s.tokenVec[1].val[:4])) {
			return false
		}
	}

	return true
}

func (s *sqliState) checkFingerprint() bool {
	return s.blacklist() && s.notWhitelist()
}

func (s *sqliState) lookupWord(lookupType int, word string) byte {
	if lookupType == sqliLookupFingerprint {
		if s.checkFingerprint() {
			return 'X'
		}

		return byteNull
	}

	return searchKeyword(word, sqlKeywords)
}

func (s *sqliState) reset(flags int) {
	if flags == 0 {
		flags = sqliFlagQuoteNone | sqliFlagSQLAnsi
	}

	sqliInit(s, s.input, flags)
}

func (s *sqliState) reparseAsMySQL() bool {
	return s.statsCommentDDX != 0 || s.statsCommentHash != 0
}

//nolint:gocyclo // complexity 11, reduction tracked in #122
func (s *sqliState) check() bool {
	if s.length == 0 {
		return false
	}

	s.computeFingerprint(sqliFlagQuoteNone | sqliFlagSQLAnsi)
	if s.checkFingerprint() {
		return true
	}

	if s.reparseAsMySQL() {
		s.computeFingerprint(sqliFlagQuoteNone | sqliFlagSQLMysql)
		if s.checkFingerprint() {
			return true
		}
	}

	if strings.IndexByte(s.input, byteSingle) != -1 {
		s.computeFingerprint(sqliFlagQuoteSingle | sqliFlagSQLAnsi)
		if s.checkFingerprint() {
			return true
		}

		if s.reparseAsMySQL() {
			s.computeFingerprint(sqliFlagQuoteSingle | sqliFlagSQLMysql)
			if s.checkFingerprint() {
				return true
			}
		}
	}

	if strings.IndexByte(s.input, byteDouble) != -1 {
		s.computeFingerprint(sqliFlagQuoteDouble | sqliFlagSQLMysql)
		if s.checkFingerprint() {
			return true
		}
	}

	return false
}

var sqliStatePool = sync.Pool{New: func() any { return new(sqliState) }}

func IsSQLi(input string) (bool, string) {
	state := sqliStatePool.Get().(*sqliState)
	defer func() {
		*state = sqliState{}

		sqliStatePool.Put(state)
	}()

	sqliInit(state, input, 0)
	if state.check() {
		return true, string(state.fingerprint[:state.fingerprintLen])
	}

	return false, ""
}
