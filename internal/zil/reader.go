// Package zil reads ZIL (MDL-syntax) source into S-expression nodes. It is a
// reader, not an evaluator: forms are returned structurally, with byte spans
// into the original source so callers can capture routine bodies verbatim.
// Scope is exactly what the historicalsource Zork I/II/III files need — see
// docs/action-waits.md §5.1.
package zil

import (
	"fmt"
	"strings"
)

// Kind discriminates Node variants.
type Kind int

const (
	KAtom   Kind = iota // FOO, ,GLOBAL, .LOCAL, escaped chars resolved
	KString             // "…" with \-escapes resolved
	KChar               // !\c character literal
	KForm               // <…>
	KList               // (…) and ![…]
	KMacro              // %elem or %%elem — single child
	KHash               // #TYPE elem — two children
)

// Node is one parsed element. Start/End are byte offsets into the source
// passed to Parse, spanning the whole element including its brackets.
type Node struct {
	Kind       Kind
	Text       string // atom name, string value, or char literal
	Kids       []*Node
	File       string
	Line       int
	Start, End int
}

// Atom returns the atom text, or "" when the node is not an atom.
func (n *Node) Atom() string {
	if n == nil || n.Kind != KAtom {
		return ""
	}
	return n.Text
}

// IsAtom reports whether the node is the atom name.
func (n *Node) IsAtom(name string) bool {
	return n != nil && n.Kind == KAtom && n.Text == name
}

// Head returns the leading atom of a form or list, or "".
func (n *Node) Head() string {
	if n == nil || (n.Kind != KForm && n.Kind != KList) || len(n.Kids) == 0 {
		return ""
	}
	return n.Kids[0].Atom()
}

type parser struct {
	src  []byte
	file string
	pos  int
	line int
}

// Parse reads every top-level element of one ZIL source file.
func Parse(src []byte, file string) ([]*Node, error) {
	p := &parser{src: src, file: file, line: 1}
	var nodes []*Node
	for {
		n, err := p.next()
		if err != nil {
			return nil, err
		}
		if n == nil {
			return nodes, nil
		}
		nodes = append(nodes, n)
	}
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s", p.file, p.line, fmt.Sprintf(format, args...))
}

func (p *parser) advance() byte {
	c := p.src[p.pos]
	if c == '\n' {
		p.line++
	}
	p.pos++
	return c
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			p.advance()
		default:
			return
		}
	}
}

// next returns the next element, or nil at end of input. Comments (;elem) are
// consumed and skipped here so callers never see them.
func (p *parser) next() (*Node, error) {
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, nil
		}
		if p.src[p.pos] == ';' {
			p.advance()
			if _, err := p.element(); err != nil {
				return nil, err
			}
			continue
		}
		return p.element()
	}
}

// element reads one element at the current position (no leading whitespace).
func (p *parser) element() (*Node, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return nil, p.errorf("unexpected end of input")
	}
	start, line := p.pos, p.line
	switch c := p.src[p.pos]; c {
	case '<':
		return p.sequence('>', KForm, start, line)
	case '(':
		return p.sequence(')', KList, start, line)
	case '[':
		return p.sequence(']', KList, start, line)
	case '"':
		return p.string_(start, line)
	case '\'':
		// Quote prefix ('elem): transparent for reading purposes.
		p.advance()
		return p.element()
	case '!':
		p.advance()
		if p.pos < len(p.src) && p.src[p.pos] == '\\' {
			p.advance()
			if p.pos >= len(p.src) {
				return nil, p.errorf("dangling character literal")
			}
			ch := p.advance()
			return &Node{Kind: KChar, Text: string(ch), File: p.file, Line: line, Start: start, End: p.pos}, nil
		}
		// !, !. ![ etc.: exotic-type marker, transparent.
		return p.element()
	case '%':
		p.advance()
		if p.pos < len(p.src) && p.src[p.pos] == '%' {
			p.advance()
		}
		kid, err := p.element()
		if err != nil {
			return nil, err
		}
		return &Node{Kind: KMacro, Kids: []*Node{kid}, File: p.file, Line: line, Start: start, End: kid.End}, nil
	case '#':
		p.advance()
		typ, err := p.element()
		if err != nil {
			return nil, err
		}
		val, err := p.element()
		if err != nil {
			return nil, err
		}
		return &Node{Kind: KHash, Kids: []*Node{typ, val}, File: p.file, Line: line, Start: start, End: val.End}, nil
	case '>', ')', ']':
		return nil, p.errorf("unexpected %q", c)
	default:
		return p.atom(start, line)
	}
}

func (p *parser) sequence(close byte, kind Kind, start, line int) (*Node, error) {
	p.advance() // opening bracket
	n := &Node{Kind: kind, File: p.file, Line: line, Start: start}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("%s:%d: unclosed %q", p.file, line, p.src[start])
		}
		switch p.src[p.pos] {
		case close:
			p.advance()
			n.End = p.pos
			return n, nil
		case '>', ')', ']':
			return nil, p.errorf("mismatched %q inside element opened at line %d", p.src[p.pos], line)
		case ';':
			p.advance()
			if _, err := p.element(); err != nil {
				return nil, err
			}
		default:
			kid, err := p.element()
			if err != nil {
				return nil, err
			}
			n.Kids = append(n.Kids, kid)
		}
	}
}

func (p *parser) string_(start, line int) (*Node, error) {
	p.advance() // opening quote
	var sb strings.Builder
	for {
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("%s:%d: unclosed string", p.file, line)
		}
		c := p.advance()
		switch c {
		case '"':
			return &Node{Kind: KString, Text: sb.String(), File: p.file, Line: line, Start: start, End: p.pos}, nil
		case '\\':
			if p.pos >= len(p.src) {
				return nil, fmt.Errorf("%s:%d: unclosed string", p.file, line)
			}
			sb.WriteByte(p.advance())
		default:
			sb.WriteByte(c)
		}
	}
}

func isDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v', '<', '>', '(', ')', '[', ']', '"', ';':
		return true
	}
	return false
}

func (p *parser) atom(start, line int) (*Node, error) {
	var sb strings.Builder
	for p.pos < len(p.src) && !isDelimiter(p.src[p.pos]) {
		c := p.advance()
		if c == '\\' {
			if p.pos >= len(p.src) {
				return nil, p.errorf("dangling escape in atom")
			}
			c = p.advance()
		}
		sb.WriteByte(c)
	}
	if sb.Len() == 0 {
		return nil, p.errorf("empty atom")
	}
	return &Node{Kind: KAtom, Text: sb.String(), File: p.file, Line: line, Start: start, End: p.pos}, nil
}
