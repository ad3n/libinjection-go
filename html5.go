package libinjection

import (
	"strings"
)

func (h *h5State) skipWhite() int {
	for h.pos < h.len {
		ch := h.s[h.pos]
		switch ch {
		case 0x00, 0x20, 0x09, 0x0A, 0x0B, 0x0C, 0x0D:
			h.pos++
		default:
			return int(ch)
		}
	}

	return byteEOF
}

func (h *h5State) stateEOF() bool {
	return false
}

func (h *h5State) stateBogusComment() bool {
	index := strings.IndexByte(h.s[h.pos:], byteGT)
	h.tokenStart = h.s[h.pos:]
	h.tokenType = html5TypeTagComment
	if index == -1 {
		h.tokenLen = h.len - h.pos
		h.pos = h.len
		h.state = (*h5State).stateEOF
		return true
	}

	h.tokenLen = index
	h.pos += index + 1
	h.state = (*h5State).stateData
	return true
}

func (h *h5State) stateBogusComment2() bool {
	pos := h.pos
	for {
		index := strings.IndexByte(h.s[pos:], bytePercent)
		if index == -1 || pos+index+1 >= h.len {
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = h.len - h.pos
			h.pos = h.len
			h.tokenType = html5TypeTagComment
			h.state = (*h5State).stateEOF
			return true
		}

		if h.s[h.pos+index+1] != byteGT {
			pos = pos + index + 1
			continue
		}

		h.tokenStart = h.s[h.pos:]
		h.tokenLen = index
		h.pos = pos + index + 2
		h.state = (*h5State).stateData
		h.tokenType = html5TypeTagComment
		return true
	}
}

//nolint:gocyclo // complexity 11, reduction tracked in #122
func (h *h5State) stateComment() bool {
	pos := h.pos

	for {
		index := strings.IndexByte(h.s[pos:], byteDash)

		if index == -1 || pos+index+3 > h.len {
			h.state = (*h5State).stateEOF
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = h.len - h.pos
			h.tokenType = html5TypeTagComment
			return true
		}

		offset := 1

		for pos+index+offset < h.len && h.s[pos+index+offset] == 0x00 {
			offset++
		}

		if pos+index+offset == h.len {
			h.state = (*h5State).stateEOF
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = h.len - h.pos
			h.tokenType = html5TypeTagComment
			return true
		}

		ch := h.s[pos+index+offset]
		if ch != byteDash && ch != byteBang {
			pos = pos + index + 1
			continue
		}

		offset++

		if pos+index+offset == h.len {
			h.state = (*h5State).stateEOF
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = h.len - h.pos
			h.tokenType = html5TypeTagComment
			return true
		}

		if h.s[pos+index+offset] != byteGT {
			pos = pos + index + 1
			continue
		}

		offset++

		h.tokenStart = h.s[h.pos:]
		h.tokenLen = index + pos - h.pos
		h.pos = pos + index + offset
		h.state = (*h5State).stateData
		h.tokenType = html5TypeTagComment
		return true
	}
}

func (h *h5State) stateCData() bool {
	pos := h.pos

	for {
		index := strings.IndexByte(h.s[pos:], byteRightB)

		switch {
		case index == -1 || pos+index+3 > h.len:
			h.state = (*h5State).stateEOF
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = h.len - h.pos
			h.tokenType = html5TypeDataText
			return true
		case h.s[pos+index+1] == byteRightB && h.s[pos+index+2] == byteGT:
			h.state = (*h5State).stateData
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos + index - h.pos
			h.pos = pos + index + 3
			h.tokenType = html5TypeDataText
			return true
		default:
			pos = pos + index + 1
		}
	}
}

func (h *h5State) stateDoctype() bool {
	h.tokenStart = h.s[h.pos:]
	h.tokenType = html5TypeDocType
	index := strings.IndexByte(h.s[h.pos:], byteGT)
	if index == -1 {
		h.state = (*h5State).stateEOF
		h.tokenLen = h.len - h.pos
		return true
	}

	h.state = (*h5State).stateData
	h.tokenLen = index
	h.pos += index + 1
	return true
}

func (h *h5State) stateMarkupDeclarationOpen() bool {
	remaining := h.len - h.pos
	switch {
	case remaining >= 7 &&
		asciiEqualFold(h.s[h.pos:h.pos+7], "doctype"):
		return h.stateDoctype()
	case remaining >= 7 &&
		h.s[h.pos:h.pos+7] == "[CDATA[":
		h.pos += 7
		return h.stateCData()
	case remaining >= 2 &&
		h.s[h.pos:h.pos+2] == "--":
		h.pos += 2
		return h.stateComment()
	}

	return h.stateBogusComment()
}

func (h *h5State) stateSelfClosingStartTag() bool {
	if h.pos >= h.len {
		return false
	}

	ch := h.s[h.pos]
	if ch == byteGT {
		h.tokenStart = h.s[h.pos-1:]
		h.tokenLen = 2
		h.tokenType = html5TypeTagNameSelfClose
		h.state = (*h5State).stateData
		h.pos++
		return true
	}

	return h.stateBeforeAttributeName()
}

func (h *h5State) stateTagNameClose() bool {
	h.isClose = false
	h.tokenStart = h.s[h.pos:]
	h.tokenLen = 1
	h.tokenType = html5TypeTagNameClose
	h.pos++
	h.state = (*h5State).stateEOF
	if h.pos < h.len {
		h.state = (*h5State).stateData
	}

	return true
}

func (h *h5State) stateTagName() bool {
	pos := h.pos

	for pos < h.len {
		ch := h.s[pos]
		switch {
		case ch == 0:
			pos++
		case isH5White(ch):
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.tokenType = html5TypeTagNameOpen
			h.pos = pos + 1
			h.state = (*h5State).stateBeforeAttributeName
			return true
		case ch == byteSlash:
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.tokenType = html5TypeTagNameOpen
			h.pos = pos + 1
			h.state = (*h5State).stateSelfClosingStartTag
			return true
		case ch == byteGT:
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			if h.isClose {
				h.pos = pos + 1
				h.isClose = false
				h.tokenType = html5TypeTagClose
				h.state = (*h5State).stateData
				return true
			}

			h.pos = pos
			h.tokenType = html5TypeTagNameOpen
			h.state = (*h5State).stateTagNameClose
			return true
		default:
			pos++
		}
	}

	h.tokenStart = h.s[h.pos:]
	h.tokenLen = h.len - h.pos
	h.tokenType = html5TypeTagNameOpen
	h.state = (*h5State).stateEOF
	return true
}

func (h *h5State) stateEndTagOpen() bool {
	if h.pos >= h.len {
		return false
	}

	ch := h.s[h.pos]
	if ch == byteGT {
		return h.stateData()
	}

	if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
		return h.stateTagName()
	}

	h.isClose = false
	return h.stateBogusComment()
}

//nolint:gocyclo // complexity 12, reduction tracked in #122
func (h *h5State) stateTagOpen() bool {
	if h.pos >= h.len {
		return false
	}

	ch := h.s[h.pos]
	switch {
	case ch == byteBang:
		h.pos++
		return h.stateMarkupDeclarationOpen()
	case ch == byteSlash:
		h.pos++
		h.isClose = true
		return h.stateEndTagOpen()
	case ch == byteQuestion:
		h.pos++
		return h.stateBogusComment()
	case ch == bytePercent:
		h.pos++
		return h.stateBogusComment2()
	case (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z'):
		return h.stateTagName()
	case ch == byteNull:
		return h.stateTagName()
	default:
		if h.pos == 0 {
			return h.stateData()
		}

		h.tokenStart = h.s[h.pos-1:]
		h.tokenLen = 1
		h.tokenType = html5TypeDataText
		h.state = (*h5State).stateData
		return true
	}
}

func (h *h5State) stateData() bool {
	index := strings.IndexByte(h.s[h.pos:], byteLT)
	h.tokenStart = h.s[h.pos:]
	h.tokenType = html5TypeDataText
	if index == -1 {
		h.tokenLen = h.len - h.pos
		h.state = (*h5State).stateEOF
		return h.tokenLen != 0
	}

	h.tokenLen = index
	h.pos += index + 1
	h.state = (*h5State).stateTagOpen
	if h.tokenLen == 0 {
		return h.stateTagOpen()
	}

	return true
}

func (h *h5State) stateAttributeValueNoQuote() bool {
	pos := h.pos

	for pos < h.len {
		ch := h.s[pos]
		if isH5White(ch) {
			h.tokenType = html5TypeAttrValue
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.pos = pos + 1
			h.state = (*h5State).stateBeforeAttributeName
			return true
		}

		if ch == byteGT {
			h.tokenType = html5TypeAttrValue
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.pos = pos
			h.state = (*h5State).stateTagNameClose
			return true
		}

		pos++
	}

	h.state = (*h5State).stateEOF
	h.tokenStart = h.s[h.pos:]
	h.tokenLen = h.len - h.pos
	h.tokenType = html5TypeAttrValue
	return true
}

func (h *h5State) stateBeforeAttributeValue() bool {
	ch := h.skipWhite()

	if ch == byteEOF {
		h.state = (*h5State).stateEOF
		return false
	}

	chByte := byte(ch & 0xFF)
	switch chByte {
	case byteDouble:
		return h.stateAttributeValueDoubleQuote()
	case byteSingle:
		return h.stateAttributeValueSingleQuote()
	case byteTick:
		return h.stateAttributeValueBackQuote()
	default:
		return h.stateAttributeValueNoQuote()
	}
}

func (h *h5State) stateAfterAttributeName() bool {
	ch := h.skipWhite()

	switch ch {
	case byteEOF:
		return false

	case byteSlash:
		h.pos++
		return h.stateSelfClosingStartTag()

	case byteEquals:
		h.pos++
		return h.stateBeforeAttributeValue()

	case byteGT:
		return h.stateTagNameClose()

	default:
		return h.stateAttributeName()
	}
}

func (h *h5State) stateAttributeName() bool {
	pos := h.pos + 1

	for pos < h.len {
		ch := h.s[pos]
		switch {
		case isH5White(ch):
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.tokenType = html5TypeAttrName
			h.state = (*h5State).stateAfterAttributeName
			h.pos = pos + 1
			return true
		case ch == byteSlash:
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.tokenType = html5TypeAttrName
			h.state = (*h5State).stateSelfClosingStartTag
			h.pos = pos + 1
			return true
		case ch == byteEquals:
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.tokenType = html5TypeAttrName
			h.state = (*h5State).stateBeforeAttributeValue
			h.pos = pos + 1
			return true
		case ch == byteGT:
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = pos - h.pos
			h.tokenType = html5TypeAttrName
			h.state = (*h5State).stateTagNameClose
			h.pos = pos
			return true
		default:
			pos++
		}
	}

	h.tokenStart = h.s[h.pos:]
	h.tokenLen = h.len - h.pos
	h.tokenType = html5TypeAttrName
	h.state = (*h5State).stateEOF
	h.pos = h.len
	return true
}

func (h *h5State) stateBeforeAttributeName() bool {
	for {
		ch := h.skipWhite()
		switch ch {
		case byteEOF:
			return false

		case byteSlash:
			h.pos++

			if h.pos < h.len && h.s[h.pos] != byteGT {
				continue
			}

			return h.stateSelfClosingStartTag()

		case byteGT:
			h.state = (*h5State).stateData
			h.tokenStart = h.s[h.pos:]
			h.tokenLen = 1
			h.tokenType = html5TypeTagNameClose
			h.pos++
			return true

		default:
			return h.stateAttributeName()
		}
	}
}

func (h *h5State) stateAfterAttributeValueQuotedState() bool {
	if h.pos >= h.len {
		return false
	}

	ch := h.s[h.pos]
	switch {
	case isH5White(ch):
		h.pos++
		return h.stateBeforeAttributeName()
	case ch == byteSlash:
		h.pos++
		return h.stateSelfClosingStartTag()
	case ch == byteGT:
		h.tokenStart = h.s[h.pos:]
		h.tokenLen = 1
		h.tokenType = html5TypeTagNameClose
		h.pos++
		h.state = (*h5State).stateData
		return true
	default:
		return h.stateBeforeAttributeName()
	}
}

func (h *h5State) stateAttributeValueQuote(ch byte) bool {
	if h.pos > 0 {
		h.pos++
	}

	index := strings.IndexByte(h.s[h.pos:], ch)
	h.tokenStart = h.s[h.pos:]
	h.tokenType = html5TypeAttrValue
	if index == -1 {
		h.tokenLen = h.len - h.pos
		h.state = (*h5State).stateEOF
		return true
	}

	h.tokenLen = index
	h.state = (*h5State).stateAfterAttributeValueQuotedState
	h.pos += h.tokenLen + 1
	return true
}

func (h *h5State) stateAttributeValueSingleQuote() bool {
	return h.stateAttributeValueQuote(byteSingle)
}

func (h *h5State) stateAttributeValueDoubleQuote() bool {
	return h.stateAttributeValueQuote(byteDouble)
}

func (h *h5State) stateAttributeValueBackQuote() bool {
	return h.stateAttributeValueQuote(byteTick)
}

func (h *h5State) init(input string, flags int) {
	*h = h5State{}

	h.s = input
	h.len = len(input)

	switch flags {
	case html5FlagsDataState:
		h.state = (*h5State).stateData

	case html5FlagsValueNoQuote:
		h.state = (*h5State).stateBeforeAttributeName

	case html5FlagsValueSingleQuote:
		h.state = (*h5State).stateAttributeValueSingleQuote

	case html5FlagsValueDoubleQuote:
		h.state = (*h5State).stateAttributeValueDoubleQuote

	case html5FlagsValueBackQuote:
		h.state = (*h5State).stateAttributeValueBackQuote
	}
}

func (h *h5State) next() bool {
	return h.state(h)
}
