package mathexpr

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Input variables available to every expression. Slot order matters: it is the
// layout of Vars.
const (
	VarPhase = iota // p: phase in radians, 0..2pi
	VarX            // x: normalized phase, 0..1
	VarT            // t: seconds since the note started
	VarFreq         // f: note frequency in Hz
	VarEnv          // env: current envelope value, 0..1
	VarVel          // vel: note velocity, 0..1
	VarSR           // sr: sample rate
	VarA            // a..d: user parameters
	VarB
	VarC
	VarD
	numSlots
)

// slotNames maps each reserved identifier to its slot. Several names are
// accepted for the same slot so expressions can read naturally.
var slotNames = map[string]int{
	"p": VarPhase, "phase": VarPhase,
	"x": VarX,
	"t": VarT, "time": VarT,
	"f": VarFreq, "freq": VarFreq,
	"env": VarEnv,
	"vel": VarVel,
	"sr":  VarSR,
	"a":   VarA, "b": VarB, "c": VarC, "d": VarD,
}

var reservedVars = func() map[string]bool {
	m := make(map[string]bool, len(slotNames)+2)
	for k := range slotNames {
		m[k] = true
	}
	m["noise"] = true
	m["rand"] = true
	return m
}()

// VariableNames lists the reserved variables with descriptions, for help text.
func VariableNames() []struct{ Name, Doc string } {
	return []struct{ Name, Doc string }{
		{"p", "phase in radians, 0..2pi (alias: phase)"},
		{"x", "phase normalized to 0..1"},
		{"t", "seconds since the note began (alias: time)"},
		{"f", "note frequency in Hz (alias: freq)"},
		{"env", "envelope level, 0..1"},
		{"vel", "note velocity, 0..1"},
		{"sr", "sample rate in Hz"},
		{"a, b, c, d", "your own parameters"},
		{"noise", "fresh white noise in -1..1 (alias: rand)"},
	}
}

type opcode uint8

const (
	opConst opcode = iota
	opLoadSlot
	opLoadLocal
	opStoreLocal
	opNoise
	opNeg
	opNot
	opAdd
	opSub
	opMul
	opDiv
	opMod
	opPow
	opLess
	opLessEq
	opGreater
	opGreaterEq
	opEqual
	opNotEqual
	opAnd
	opOr
	opCall1
	opCall2
	opCall3
	opJump
	opJumpIfFalse
	opLoopInit
	opLoopTest
	opLoopAccum
)

type instr struct {
	op   opcode
	arg  int     // slot index or function index
	arg2 int     // loop limit slot
	jump int     // branch target
	val  float64 // immediate for opConst
}

// Program is a compiled expression, ready to evaluate. It is immutable and
// safe to share across goroutines; evaluation state lives in Vars.
type Program struct {
	code    []instr
	source  string
	nLocals int
	stackSz int

	usesTime  bool
	usesEnv   bool
	usesNoise bool
	usesVel   bool
	usesPhase bool
	usesParam [4]bool
}

// Source returns the original expression text.
func (p *Program) Source() string { return p.source }

// PhaseOnly reports whether the expression depends only on phase, meaning its
// output is a fixed periodic waveform. Such programs can be safely rendered
// into a wavetable once instead of evaluated per sample.
func (p *Program) PhaseOnly() bool {
	return !p.usesTime && !p.usesEnv && !p.usesNoise && !p.usesVel
}

// UsesTime reports whether the expression reads t.
func (p *Program) UsesTime() bool { return p.usesTime }

// UsesNoise reports whether the expression reads noise.
func (p *Program) UsesNoise() bool { return p.usesNoise }

// UsesPhase reports whether the expression reads p or x. An oscillator that
// ignores phase produces no pitch, which is worth warning about.
func (p *Program) UsesPhase() bool { return p.usesPhase }

// UsedParams reports which of a, b, c, d the expression actually reads.
func (p *Program) UsedParams() [4]bool { return p.usesParam }

// Vars holds the evaluation inputs plus scratch space for loop variables.
// Each concurrent evaluator needs its own Vars; get one from Program.NewVars.
type Vars struct {
	slots  [numSlots]float64
	locals []float64
	stack  []float64
}

// NewVars allocates evaluation state sized for this program.
func (p *Program) NewVars() *Vars {
	return &Vars{
		locals: make([]float64, p.nLocals),
		stack:  make([]float64, p.stackSz+8),
	}
}

// Set assigns an input slot, e.g. v.Set(mathexpr.VarPhase, 1.5).
func (v *Vars) Set(slot int, val float64) { v.slots[slot] = val }

// Get reads an input slot.
func (v *Vars) Get(slot int) float64 { return v.slots[slot] }

// SetParams assigns the a, b, c, d parameter slots at once.
func (v *Vars) SetParams(a, b, c, d float64) {
	v.slots[VarA] = a
	v.slots[VarB] = b
	v.slots[VarC] = c
	v.slots[VarD] = d
}

type compiler struct {
	code   []instr
	locals map[string]int
	prog   *Program
	depth  int
	maxDep int
}

// Compile parses and compiles an expression. The returned error is a *Error
// with a column offset when the source is malformed.
func Compile(src string) (*Program, error) {
	ast, err := parse(src)
	if err != nil {
		return nil, err
	}

	prog := &Program{source: src}
	c := &compiler{locals: map[string]int{}, prog: prog}
	if err := c.emitNode(ast); err != nil {
		return nil, err
	}

	prog.code = c.code
	prog.nLocals = len(c.locals)
	prog.stackSz = c.maxDep
	return prog, nil
}

// MustCompile is Compile but panics on error. Intended for built-in
// expressions that are known good at build time.
func MustCompile(src string) *Program {
	p, err := Compile(src)
	if err != nil {
		panic(fmt.Sprintf("mathexpr: compiling %q: %v", src, err))
	}
	return p
}

func (c *compiler) push(n int) {
	c.depth += n
	if c.depth > c.maxDep {
		c.maxDep = c.depth
	}
}

func (c *compiler) emit(in instr) int {
	c.code = append(c.code, in)
	return len(c.code) - 1
}

func (c *compiler) emitNode(n node) error {
	switch v := n.(type) {
	case *numNode:
		c.emit(instr{op: opConst, val: v.val})
		c.push(1)
		return nil

	case *varNode:
		return c.emitVar(v)

	case *unaryNode:
		if err := c.emitNode(v.child); err != nil {
			return err
		}
		if v.op == "-" {
			c.emit(instr{op: opNeg})
		} else {
			c.emit(instr{op: opNot})
		}
		return nil

	case *binNode:
		return c.emitBinary(v)

	case *callNode:
		return c.emitCall(v)

	case *condNode:
		return c.emitCond(v)

	case *sumNode:
		return c.emitSum(v)
	}
	return fmt.Errorf("mathexpr: unhandled node %T", n)
}

func (c *compiler) emitVar(v *varNode) error {
	if slot, ok := slotNames[v.name]; ok {
		c.emit(instr{op: opLoadSlot, arg: slot})
		c.push(1)
		c.markUse(slot)
		return nil
	}
	if v.name == "noise" || v.name == "rand" {
		c.emit(instr{op: opNoise})
		c.push(1)
		c.prog.usesNoise = true
		return nil
	}
	if idx, ok := c.locals[v.name]; ok {
		c.emit(instr{op: opLoadLocal, arg: idx})
		c.push(1)
		return nil
	}
	return &Error{Pos: v.pos, Msg: fmt.Sprintf("unknown variable %q (try %s)", v.name, hint(v.name))}
}

func (c *compiler) markUse(slot int) {
	switch slot {
	case VarT:
		c.prog.usesTime = true
	case VarEnv:
		c.prog.usesEnv = true
	case VarVel:
		c.prog.usesVel = true
	case VarPhase, VarX:
		c.prog.usesPhase = true
	case VarA, VarB, VarC, VarD:
		c.prog.usesParam[slot-VarA] = true
	}
}

var binOps = map[string]opcode{
	"+": opAdd, "-": opSub, "*": opMul, "/": opDiv, "%": opMod, "^": opPow,
	"<": opLess, "<=": opLessEq, ">": opGreater, ">=": opGreaterEq,
	"==": opEqual, "!=": opNotEqual, "&&": opAnd, "||": opOr,
}

func (c *compiler) emitBinary(v *binNode) error {
	if err := c.emitNode(v.l); err != nil {
		return err
	}
	if err := c.emitNode(v.r); err != nil {
		return err
	}
	op, ok := binOps[v.op]
	if !ok {
		return &Error{Pos: v.pos, Msg: fmt.Sprintf("unsupported operator %q", v.op)}
	}
	c.emit(instr{op: op})
	c.push(-1)
	return nil
}

func (c *compiler) emitCall(v *callNode) error {
	for _, a := range v.args {
		if err := c.emitNode(a); err != nil {
			return err
		}
	}
	idx := builtinIndex[v.name]
	switch builtins[idx].arity {
	case 1:
		c.emit(instr{op: opCall1, arg: idx})
	case 2:
		c.emit(instr{op: opCall2, arg: idx})
		c.push(-1)
	case 3:
		c.emit(instr{op: opCall3, arg: idx})
		c.push(-2)
	}
	return nil
}

func (c *compiler) emitCond(v *condNode) error {
	if err := c.emitNode(v.cond); err != nil {
		return err
	}
	jf := c.emit(instr{op: opJumpIfFalse})
	c.push(-1)

	if err := c.emitNode(v.then); err != nil {
		return err
	}
	jEnd := c.emit(instr{op: opJump})
	c.code[jf].jump = len(c.code)

	// Both branches leave one value, so undo the double count.
	c.depth--
	if err := c.emitNode(v.els); err != nil {
		return err
	}
	c.code[jEnd].jump = len(c.code)
	return nil
}

// emitSum compiles an aggregate into a counted loop. Layout:
//
//	<accumulator const>          ; 0 for sum, 1 for prod
//	<from> <to>  -> opLoopInit   ; stores counter+limit in locals
//	test: opLoopTest             ; jumps past body when counter > limit
//	<body> opLoopAccum           ; folds body into accumulator, bumps counter
//	opJump test
func (c *compiler) emitSum(v *sumNode) error {
	if _, exists := c.locals[v.loopVar]; exists {
		return &Error{Pos: v.pos, Msg: fmt.Sprintf("loop variable %q is already in use", v.loopVar)}
	}

	init := 0.0
	if v.product {
		init = 1.0
	}
	c.emit(instr{op: opConst, val: init})
	c.push(1)

	limitSlot := c.newLocal("")
	if err := c.emitNode(v.to); err != nil {
		return err
	}
	c.emit(instr{op: opStoreLocal, arg: limitSlot})
	c.push(-1)

	counterSlot := c.newLocal(v.loopVar)
	if err := c.emitNode(v.from); err != nil {
		return err
	}
	c.emit(instr{op: opLoopInit, arg: counterSlot})
	c.push(-1)

	testPC := c.emit(instr{op: opLoopTest, arg: counterSlot, arg2: limitSlot})
	if err := c.emitNode(v.body); err != nil {
		return err
	}
	accum := opAdd
	if v.product {
		accum = opMul
	}
	c.emit(instr{op: opLoopAccum, arg: counterSlot, val: float64(accum)})
	c.push(-1)
	c.emit(instr{op: opJump, jump: testPC})
	c.code[testPC].jump = len(c.code)

	// The loop variable leaves scope; the limit slot stays reserved.
	delete(c.locals, v.loopVar)
	c.locals[fmt.Sprintf("\x00retired%d", counterSlot)] = counterSlot
	return nil
}

func (c *compiler) newLocal(name string) int {
	idx := len(c.locals)
	if name == "" {
		name = fmt.Sprintf("\x00tmp%d", idx)
	}
	c.locals[name] = idx
	return idx
}

func hint(name string) string {
	var names []string
	for k := range slotNames {
		names = append(names, k)
	}
	sort.Strings(names)
	best, bestScore := "", 1<<30
	for _, cand := range append(names, "noise") {
		if d := editDistance(name, cand); d < bestScore {
			best, bestScore = cand, d
		}
	}
	if bestScore <= 2 {
		return strconv.Quote(best)
	}
	return "p, x, t, f, env or a-d"
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// maxLoopIterations caps aggregates so a typo like sum(k,1,1e9,...) cannot
// stall the audio thread.
const maxLoopIterations = 4096

// Eval runs the program. It never returns NaN or Inf; those collapse to 0 so
// a bad expression cannot damage speakers.
func (p *Program) Eval(v *Vars) float64 {
	st := v.stack
	sp := 0
	pc := 0
	code := p.code
	iterBudget := maxLoopIterations

	for pc < len(code) {
		in := code[pc]
		pc++

		switch in.op {
		case opConst:
			st[sp] = in.val
			sp++
		case opLoadSlot:
			st[sp] = v.slots[in.arg]
			sp++
		case opLoadLocal:
			st[sp] = v.locals[in.arg]
			sp++
		case opStoreLocal:
			sp--
			v.locals[in.arg] = st[sp]
		case opNoise:
			st[sp] = whiteNoise()
			sp++

		case opNeg:
			st[sp-1] = -st[sp-1]
		case opNot:
			if st[sp-1] == 0 {
				st[sp-1] = 1
			} else {
				st[sp-1] = 0
			}

		case opAdd:
			sp--
			st[sp-1] += st[sp]
		case opSub:
			sp--
			st[sp-1] -= st[sp]
		case opMul:
			sp--
			st[sp-1] *= st[sp]
		case opDiv:
			sp--
			st[sp-1] = safeDiv(st[sp-1], st[sp])
		case opMod:
			sp--
			if st[sp] == 0 {
				st[sp-1] = 0
			} else {
				st[sp-1] = math.Mod(st[sp-1], st[sp])
			}
		case opPow:
			sp--
			st[sp-1] = math.Pow(st[sp-1], st[sp])

		case opLess:
			sp--
			st[sp-1] = boolVal(st[sp-1] < st[sp])
		case opLessEq:
			sp--
			st[sp-1] = boolVal(st[sp-1] <= st[sp])
		case opGreater:
			sp--
			st[sp-1] = boolVal(st[sp-1] > st[sp])
		case opGreaterEq:
			sp--
			st[sp-1] = boolVal(st[sp-1] >= st[sp])
		case opEqual:
			sp--
			st[sp-1] = boolVal(st[sp-1] == st[sp])
		case opNotEqual:
			sp--
			st[sp-1] = boolVal(st[sp-1] != st[sp])
		case opAnd:
			sp--
			st[sp-1] = boolVal(st[sp-1] != 0 && st[sp] != 0)
		case opOr:
			sp--
			st[sp-1] = boolVal(st[sp-1] != 0 || st[sp] != 0)

		case opCall1:
			st[sp-1] = builtins[in.arg].f1(st[sp-1])
		case opCall2:
			sp--
			st[sp-1] = builtins[in.arg].f2(st[sp-1], st[sp])
		case opCall3:
			sp -= 2
			st[sp-1] = builtins[in.arg].f3(st[sp-1], st[sp], st[sp+1])

		case opJump:
			pc = in.jump
		case opJumpIfFalse:
			sp--
			if st[sp] == 0 {
				pc = in.jump
			}

		case opLoopInit:
			sp--
			v.locals[in.arg] = math.Round(st[sp])

		case opLoopTest:
			if v.locals[in.arg] > math.Round(v.locals[in.arg2]) || iterBudget <= 0 {
				pc = in.jump
			}

		case opLoopAccum:
			sp--
			if opcode(in.val) == opMul {
				st[sp-1] *= st[sp]
			} else {
				st[sp-1] += st[sp]
			}
			v.locals[in.arg]++
			iterBudget--
		}
	}

	if sp == 0 {
		return 0
	}
	out := st[sp-1]
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return 0
	}
	return out
}

func boolVal(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
