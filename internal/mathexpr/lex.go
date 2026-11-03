package mathexpr

import (
	"fmt"
	"strconv"
	"strings"
)

type tokKind int

const (
	tkEOF tokKind = iota
	tkNumber
	tkIdent
	tkOp
	tkLParen
	tkRParen
	tkComma
	tkQuestion
	tkColon
)

type token struct {
	kind tokKind
	text string
	num  float64
	pos  int
}

func (t token) describe() string {
	if t.kind == tkEOF {
		return "end of expression"
	}
	return strconv.Quote(t.text)
}

// startsOperand reports whether the token can begin a primary expression,
// which is what makes implicit multiplication such as "2p" detectable.
func (t token) startsOperand() bool {
	return t.kind == tkNumber || t.kind == tkIdent || t.kind == tkLParen
}

type lexer struct {
	src  string
	pos  int
	toks []token
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

func tokenize(src string) ([]token, error) {
	l := &lexer{src: src}
	for {
		l.skipSpace()
		if l.pos >= len(l.src) {
			l.toks = append(l.toks, token{kind: tkEOF, pos: l.pos})
			return l.toks, nil
		}

		start := l.pos
		c := l.src[l.pos]

		switch {
		case isDigit(c) || c == '.':
			if err := l.lexNumber(); err != nil {
				return nil, err
			}

		case isIdentStart(c):
			for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
				l.pos++
			}
			l.toks = append(l.toks, token{
				kind: tkIdent,
				text: strings.ToLower(l.src[start:l.pos]),
				pos:  start,
			})

		case c == '(':
			l.pos++
			l.emit(tkLParen, start)
		case c == ')':
			l.pos++
			l.emit(tkRParen, start)
		case c == ',':
			l.pos++
			l.emit(tkComma, start)
		case c == '?':
			l.pos++
			l.emit(tkQuestion, start)
		case c == ':':
			l.pos++
			l.emit(tkColon, start)

		case strings.ContainsRune("+-*/%^<>=!&|", rune(c)):
			l.lexOperator()

		default:
			return nil, &Error{Pos: start, Msg: fmt.Sprintf("unexpected character %q", string(c))}
		}
	}
}

func (l *lexer) emit(k tokKind, start int) {
	l.toks = append(l.toks, token{kind: k, text: l.src[start:l.pos], pos: start})
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.src) {
		switch l.src[l.pos] {
		case ' ', '\t', '\n', '\r':
			l.pos++
		default:
			return
		}
	}
}

func (l *lexer) lexNumber() error {
	start := l.pos
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	// Exponent, but only when followed by digits so that "2e" lexes as 2*e.
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		save := l.pos
		l.pos++
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.pos++
		}
		if l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
				l.pos++
			}
		} else {
			l.pos = save
		}
	}

	text := l.src[start:l.pos]
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return &Error{Pos: start, Msg: fmt.Sprintf("invalid number %q", text)}
	}
	l.toks = append(l.toks, token{kind: tkNumber, text: text, num: v, pos: start})
	return nil
}

var multiCharOps = []string{"<=", ">=", "==", "!=", "&&", "||"}

func (l *lexer) lexOperator() {
	start := l.pos
	for _, op := range multiCharOps {
		if strings.HasPrefix(l.src[l.pos:], op) {
			l.pos += len(op)
			l.emit(tkOp, start)
			return
		}
	}
	l.pos++
	l.emit(tkOp, start)
}
