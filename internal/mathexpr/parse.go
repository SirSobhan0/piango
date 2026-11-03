package mathexpr

import "fmt"

// Error is a parse or compile failure carrying the byte offset in the source,
// so the editor can point at the offending character.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("col %d: %s", e.Pos+1, e.Msg) }

type node interface{}

type numNode struct{ val float64 }

type varNode struct {
	name string
	pos  int
}

type callNode struct {
	name string
	args []node
	pos  int
}

type unaryNode struct {
	op    string
	child node
}

type binNode struct {
	op   string
	l, r node
	pos  int
}

type condNode struct {
	cond, then, els node
}

// sumNode is the summation form sum(var, from, to, body), evaluated by
// iterating the loop variable over an inclusive integer range.
type sumNode struct {
	loopVar  string
	from, to node
	body     node
	product  bool
	pos      int
}

type parser struct {
	toks []token
	pos  int
	src  string
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }
func (p *parser) atEOF() bool { return p.peek().kind == tkEOF }
func (p *parser) errAt(t token, msg string) error {
	return &Error{Pos: t.pos, Msg: msg}
}

func (p *parser) expect(k tokKind, what string) (token, error) {
	t := p.peek()
	if t.kind != k {
		return t, p.errAt(t, fmt.Sprintf("expected %s but found %s", what, t.describe()))
	}
	return p.next(), nil
}

// Binary operator precedence. Higher binds tighter.
var precedence = map[string]int{
	"||": 1,
	"&&": 2,
	"==": 3, "!=": 3,
	"<": 4, "<=": 4, ">": 4, ">=": 4,
	"+": 5, "-": 5,
	"*": 6, "/": 6, "%": 6,
	"^": 8,
}

// rightAssoc marks operators that group to the right, so 2^3^2 is 2^(3^2).
var rightAssoc = map[string]bool{"^": true}

// implicitMulPrec is the binding strength of juxtaposition ("2p", "3sin(x)").
// It matches "*" so that 2p/3q parses like (2*p)/(3*q).
const implicitMulPrec = 6

func (p *parser) parseExpr(minPrec int) (node, error) {
	lhs, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for {
		t := p.peek()

		// Ternary has the loosest binding of all.
		if t.kind == tkQuestion && minPrec <= 0 {
			p.next()
			then, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tkColon, "\":\" to complete the ternary"); err != nil {
				return nil, err
			}
			els, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			lhs = &condNode{cond: lhs, then: then, els: els}
			continue
		}

		if t.kind == tkOp {
			prec, ok := precedence[t.text]
			if !ok {
				return nil, p.errAt(t, fmt.Sprintf("%s is not a binary operator", t.describe()))
			}
			if prec < minPrec {
				return lhs, nil
			}
			p.next()
			nextMin := prec + 1
			if rightAssoc[t.text] {
				nextMin = prec
			}
			rhs, err := p.parseExpr(nextMin)
			if err != nil {
				return nil, err
			}
			lhs = &binNode{op: t.text, l: lhs, r: rhs, pos: t.pos}
			continue
		}

		// Implicit multiplication: an operand directly following an operand.
		if t.startsOperand() && implicitMulPrec >= minPrec {
			rhs, err := p.parseExpr(implicitMulPrec + 1)
			if err != nil {
				return nil, err
			}
			lhs = &binNode{op: "*", l: lhs, r: rhs, pos: t.pos}
			continue
		}

		return lhs, nil
	}
}

func (p *parser) parseUnary() (node, error) {
	t := p.peek()
	if t.kind == tkOp {
		switch t.text {
		case "-":
			p.next()
			child, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			if n, ok := child.(*numNode); ok {
				return &numNode{val: -n.val}, nil
			}
			return &unaryNode{op: "-", child: child}, nil
		case "+":
			p.next()
			return p.parseUnary()
		case "!":
			p.next()
			child, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &unaryNode{op: "!", child: child}, nil
		}
	}
	return p.parsePower()
}

// parsePower lets "-2^2" mean -(2^2) while "2^-1" still parses, by allowing a
// unary expression on the right of "^".
func (p *parser) parsePower() (node, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tkOp && t.text == "^" {
		p.next()
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &binNode{op: "^", l: base, r: exp, pos: t.pos}, nil
	}
	return base, nil
}

func (p *parser) parsePrimary() (node, error) {
	t := p.next()

	switch t.kind {
	case tkNumber:
		return &numNode{val: t.num}, nil

	case tkLParen:
		inner, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tkRParen, "a closing \")\""); err != nil {
			return nil, err
		}
		return inner, nil

	case tkIdent:
		if p.peek().kind == tkLParen {
			return p.parseCall(t)
		}
		if v, ok := namedConstants[t.text]; ok {
			return &numNode{val: v}, nil
		}
		return &varNode{name: t.text, pos: t.pos}, nil
	}

	return nil, p.errAt(t, fmt.Sprintf("unexpected %s", t.describe()))
}

func (p *parser) parseCall(name token) (node, error) {
	p.next() // consume "("

	if name.text == "sum" || name.text == "prod" {
		return p.parseAggregate(name)
	}

	var args []node
	if p.peek().kind != tkRParen {
		for {
			a, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			if p.peek().kind != tkComma {
				break
			}
			p.next()
		}
	}
	if _, err := p.expect(tkRParen, "a closing \")\""); err != nil {
		return nil, err
	}

	idx, ok := builtinIndex[name.text]
	if !ok {
		return nil, p.errAt(name, fmt.Sprintf("unknown function %q", name.text))
	}
	if got, want := len(args), builtins[idx].arity; got != want {
		return nil, p.errAt(name, fmt.Sprintf("%s takes %d argument(s) but got %d", name.text, want, got))
	}
	return &callNode{name: name.text, args: args, pos: name.pos}, nil
}

// parseAggregate handles sum(k, 1, 9, body) and prod(k, 1, 4, body).
func (p *parser) parseAggregate(name token) (node, error) {
	loopTok, err := p.expect(tkIdent, "a loop variable name, as in sum(k, 1, 9, ...)")
	if err != nil {
		return nil, err
	}
	if _, ok := namedConstants[loopTok.text]; ok {
		return nil, p.errAt(loopTok, fmt.Sprintf("%q is a constant and cannot be a loop variable", loopTok.text))
	}
	if reservedVars[loopTok.text] {
		return nil, p.errAt(loopTok, fmt.Sprintf("%q is a built-in variable and cannot be a loop variable", loopTok.text))
	}

	parts := make([]node, 0, 3)
	for range 3 {
		if _, err := p.expect(tkComma, "a \",\""); err != nil {
			return nil, err
		}
		n, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		parts = append(parts, n)
	}
	if _, err := p.expect(tkRParen, "a closing \")\""); err != nil {
		return nil, err
	}

	return &sumNode{
		loopVar: loopTok.text,
		from:    parts[0],
		to:      parts[1],
		body:    parts[2],
		product: name.text == "prod",
		pos:     name.pos,
	}, nil
}

func parse(src string) (node, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, src: src}
	n, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if !p.atEOF() {
		return nil, p.errAt(p.peek(), fmt.Sprintf("unexpected trailing %s", p.peek().describe()))
	}
	return n, nil
}
