package libinjection

import (
	"strings"
	"sync"
)

var h5StatePool = sync.Pool{New: func() any { return new(h5State) }}

func runXSS(h5 *h5State) bool {
	attr := attributeTypeNone
	for h5.next() {
		if h5.tokenType != html5TypeAttrValue {
			attr = attributeTypeNone
		}

		detected, nextAttr := checkToken(h5, attr)
		if detected {
			return true
		}

		attr = nextAttr
	}

	return false
}

func checkToken(h5 *h5State, attr int) (detected bool, nextAttr int) {
	switch h5.tokenType {
	case html5TypeDocType:
		return true, attr
	case html5TypeTagNameOpen:
		return isBlackTag(h5.tokenStart[:h5.tokenLen]), attr
	case html5TypeAttrName:
		return false, isBlackAttr(h5.tokenStart[:h5.tokenLen])
	case html5TypeAttrValue:
		return handleAttrValue(h5.tokenStart, h5.tokenLen, attr), attributeTypeNone
	case html5TypeTagComment:
		return handleTagComment(h5.tokenStart, h5.tokenLen), attr
	}

	return false, attr
}

func handleAttrValue(tokenStart string, tokenLen int, attr int) bool {
	switch attr {
	case attributeTypeNone:
		return false
	case attributeTypeBlack:
		return true
	case attributeTypeAttrURL:
		return isBlackURL(tokenStart[:tokenLen])
	case attributeTypeStyle:
		return true
	case attributeTypeAttrIndirect:
		return isBlackAttr(tokenStart[:tokenLen]) == attributeTypeBlack
	}

	return false
}

func hasCommentPrefix(token string) bool {
	if len(token) < 4 {
		return false
	}

	lower := [3]byte{token[0] | 0x20, token[1] | 0x20, token[2] | 0x20}

	return (token[0] == '[' && lower[1] == 'i' && lower[2] == 'f') ||
		lower == [3]byte{'x', 'm', 'l'}
}

func handleTagComment(tokenStart string, tokenLen int) bool {
	if strings.IndexByte(tokenStart[:tokenLen], '`') != -1 {
		return true
	}

	if hasCommentPrefix(tokenStart[:tokenLen]) {
		return true
	}

	if tokenLen > 5 {
		var buf [6]byte
		n, _ := upperRemoveNulls(buf[:], tokenStart[:tokenLen])
		if n == 6 && (string(buf[:6]) == "IMPORT" || string(buf[:6]) == "ENTITY") {
			return true
		}
	}

	return false
}

func IsXSS(input string) bool {
	h5 := h5StatePool.Get().(*h5State)
	defer func() {
		*h5 = h5State{}

		h5StatePool.Put(h5)
	}()

	if strings.IndexByte(input, '<') != -1 {
		h5.init(input, html5FlagsDataState)
		if runXSS(h5) {
			return true
		}
	}

	for _, flags := range [...]int{
		html5FlagsValueNoQuote,
		html5FlagsValueSingleQuote,
		html5FlagsValueDoubleQuote,
		html5FlagsValueBackQuote,
	} {
		h5.init(input, flags)
		if runXSS(h5) {
			return true
		}
	}

	return false
}
