package libinjection

import "testing"

type h5TokenInfo struct {
	typ int
	len int
}

func nextTokenInfos(input string, flags int) []h5TokenInfo {
	h := new(h5State)
	h.init(input, flags)
	var toks []h5TokenInfo
	for h.next() {
		toks = append(toks, h5TokenInfo{typ: h.tokenType, len: h.tokenLen})
	}

	return toks
}

func checkTokens(t *testing.T, got []h5TokenInfo, want ...h5TokenInfo) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d tokens %v", len(got), got, len(want), want)
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("token[%d]: got {typ=%d, len=%d}, want {typ=%d, len=%d}",
				i, got[i].typ, got[i].len, w.typ, w.len)
		}
	}
}

func TestSkipWhiteEOF(t *testing.T) {
	got := nextTokenInfos("<div   ", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagNameOpen, 3})
}

func TestStateBogusComment2Continue(t *testing.T) {
	got := nextTokenInfos("<%a%b>", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 4})
}

func TestStateBogusComment2PctGT(t *testing.T) {
	got := nextTokenInfos("<%foo%>", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 3})
}

func TestStateCommentNullBytes(t *testing.T) {
	got := nextTokenInfos("<!---\x00->", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 0})
}

func TestStateCommentNonDashBangContinue(t *testing.T) {
	got := nextTokenInfos("<!--foo-bar-->", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 7})
}

func TestStateCommentNullBytesEOF(t *testing.T) {
	got := nextTokenInfos("<!---\x00\x00", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 3})
}

func TestStateCommentDashBangNoGT(t *testing.T) {
	got := nextTokenInfos("<!--foo-!bar-->", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 8})
}

func TestStateCommentEOFAfterDashNull(t *testing.T) {
	got := nextTokenInfos("<!---\x00-", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 3})
}

func TestStateEndTagOpenEOF(t *testing.T) {
	got := nextTokenInfos("</", html5FlagsDataState)
	if len(got) != 0 {
		t.Fatalf("expected no tokens for EOF after '</', got %v", got)
	}
}

func TestStateEndTagOpenGT(t *testing.T) {
	got := nextTokenInfos("</>", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeDataText, 1})
}

func TestStateEndTagOpenBogus(t *testing.T) {
	got := nextTokenInfos("</0abc>", html5FlagsDataState)
	checkTokens(t, got, h5TokenInfo{html5TypeTagComment, 4})
}

func TestStateTagOpenNullChar(t *testing.T) {
	got := nextTokenInfos("<\x00div>", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeTagNameOpen, 4},
		h5TokenInfo{html5TypeTagNameClose, 1},
	)
}

func TestStateTagOpenDefaultNonZeroPos(t *testing.T) {
	got := nextTokenInfos("<1foo>", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeDataText, 1},
		h5TokenInfo{html5TypeDataText, 5},
	)
}

func TestStateTagOpenDefaultZeroPos(t *testing.T) {
	h := &h5State{}

	h.s = "1foo"
	h.len = 4
	h.pos = 0
	h.state = (*h5State).stateTagOpen

	result := h.next()
	if !result {
		t.Fatal("expected h.next() to return true")
	}

	if h.tokenType != html5TypeDataText {
		t.Fatalf("expected html5TypeDataText token, got %d", h.tokenType)
	}

	if h.tokenLen == 0 {
		t.Fatal("expected non-empty token")
	}
}

func TestStateAfterAttributeNameEOF(t *testing.T) {
	got := nextTokenInfos("<div foo   ", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeTagNameOpen, 3},
		h5TokenInfo{html5TypeAttrName, 3},
	)
}

func TestStateAfterAttributeNameSlash(t *testing.T) {
	got := nextTokenInfos("<div foo />", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeTagNameOpen, 3},
		h5TokenInfo{html5TypeAttrName, 3},
		h5TokenInfo{html5TypeTagNameSelfClose, 2},
	)
}

func TestStateAfterAttributeNameGT(t *testing.T) {
	got := nextTokenInfos("<div foo >", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeTagNameOpen, 3},
		h5TokenInfo{html5TypeAttrName, 3},
		h5TokenInfo{html5TypeTagNameClose, 1},
	)
}

func TestStateBeforeAttributeValueEOF(t *testing.T) {
	got := nextTokenInfos("<div href=   ", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeTagNameOpen, 3},
		h5TokenInfo{html5TypeAttrName, 4},
	)
}

func TestStateBeforeAttributeNameSlashContinue(t *testing.T) {
	got := nextTokenInfos("<div / foo>", html5FlagsDataState)
	checkTokens(
		t, got,
		h5TokenInfo{html5TypeTagNameOpen, 3},
		h5TokenInfo{html5TypeAttrName, 3},
		h5TokenInfo{html5TypeTagNameClose, 1},
	)
}
