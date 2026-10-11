package main

import (
	"regexp"
	"strings"
)

// A deliberately small JavaScript/TypeScript reader: enough to recover the
// URL paths the API clients build, which are written as string literals,
// template literals, `+` concatenations and small path helpers such as
//
//	const base = (id: string) => `/inside/businesses/${id}`;
//	axiosInstance.get(base(id) + "/spaces/" + spaceId);
//
// Unknown expressions become the placeholder segment.

type jsTokKind int

const (
	tokString jsTokKind = iota
	tokTemplate
	tokIdent
	tokPunct
)

type jsTok struct {
	kind jsTokKind
	text string // string body (unquoted), identifier, or punctuation
}

func jsTokenize(src string) []jsTok {
	var toks []jsTok
	i := 0
	n := len(src)
	for i < n {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '/' && i+1 < n && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return toks
			}
			i += end + 4
		case c == '"' || c == '\'':
			j := i + 1
			for j < n && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= n {
				return toks
			}
			toks = append(toks, jsTok{tokString, src[i+1 : j]})
			i = j + 1
		case c == '`':
			j := i + 1
			depth := 0
			for j < n {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if depth == 0 && src[j] == '`' {
					break
				}
				if src[j] == '$' && j+1 < n && src[j+1] == '{' {
					depth++
					j += 2
					continue
				}
				if depth > 0 && src[j] == '}' {
					depth--
				}
				j++
			}
			if j >= n {
				return toks
			}
			toks = append(toks, jsTok{tokTemplate, src[i+1 : j]})
			i = j + 1
		case isIdentStart(c):
			j := i
			for j < n && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, jsTok{tokIdent, src[i:j]})
			i = j
		default:
			toks = append(toks, jsTok{tokPunct, string(c)})
			i++
		}
	}
	return toks
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

type jsDef struct {
	params []string
	values []string // may carry param markers \x01<i>\x02
}

type jsEval struct {
	toks   []jsTok
	defs   map[string]jsDef
	params []string // parameters of the definition being collected
}

const maxAlternatives = 16

// jsPathValues returns every string value a file builds that may be a path.
func jsPathValues(src string) []string {
	e := &jsEval{toks: jsTokenize(src), defs: map[string]jsDef{}}
	e.collectDefs()
	var out []string
	for i := 0; i < len(e.toks); i++ {
		t := e.toks[i]
		if t.kind != tokString && t.kind != tokTemplate && t.kind != tokIdent {
			continue
		}
		if i > 0 && e.toks[i-1].kind == tokPunct && (e.toks[i-1].text == "+" || e.toks[i-1].text == ".") {
			continue // inside a chain that starts earlier
		}
		vs, _, lit := e.chain(i)
		if !lit {
			continue
		}
		for _, v := range vs {
			v = stripMarkers(v)
			if strings.Contains(v, "/") {
				out = append(out, v)
			}
		}
	}
	return out
}

func stripMarkers(v string) string {
	for {
		i := strings.IndexByte(v, '\x01')
		if i < 0 {
			return v
		}
		j := strings.IndexByte(v[i:], '\x02')
		if j < 0 {
			return v[:i] + placeholder
		}
		v = v[:i] + placeholder + v[i+j+1:]
	}
}

// collectDefs records `const X = <chain>`, `const X = (...) => <chain>` and
// `function X(...) { return <chain>` whose value contains a string.
func (e *jsEval) collectDefs() {
	t := e.toks
	for i := 0; i+2 < len(t); i++ {
		if t[i].kind == tokIdent && (t[i].text == "const" || t[i].text == "let" || t[i].text == "var") &&
			t[i+1].kind == tokIdent {
			name := t[i+1].text
			j := i + 2
			if j < len(t) && t[j].text == ":" { // type annotation: skip to '='
				for j < len(t) && t[j].text != "=" && t[j].text != ";" {
					j++
				}
			}
			if j >= len(t) || t[j].text != "=" {
				continue
			}
			j++
			body, params := e.skipArrowHead(j)
			e.params = params
			if vs, _, lit := e.chain(body); lit {
				e.defs[name] = jsDef{params: params, values: vs}
			}
			e.params = nil
		}
		if t[i].kind == tokIdent && t[i].text == "function" && t[i+1].kind == tokIdent {
			name := t[i+1].text
			j := i + 2
			var params []string
			if j < len(t) && t[j].text == "(" {
				params = e.paramNames(j)
				j = e.skipBalanced(j)
			}
			// optional return type, then `{ return`
			for k := j; k < len(t) && k < j+12; k++ {
				if t[k].text == "{" {
					if k+1 < len(t) && t[k+1].text == "return" {
						e.params = params
						if vs, _, lit := e.chain(k + 2); lit {
							e.defs[name] = jsDef{params: params, values: vs}
						}
						e.params = nil
					}
					break
				}
			}
		}
	}
}

// paramNames lists the parameter names of the group opening at j.
func (e *jsEval) paramNames(j int) []string {
	t := e.toks
	end := e.skipBalanced(j)
	var names []string
	depth := 0
	expect := true
	for k := j; k < end; k++ {
		if t[k].kind == tokPunct {
			switch t[k].text {
			case "(", "[", "{", "<":
				depth++
			case ")", "]", "}", ">":
				depth--
			case ",":
				if depth == 1 {
					expect = true
				}
			}
			continue
		}
		if depth == 1 && expect && t[k].kind == tokIdent {
			names = append(names, t[k].text)
			expect = false
		}
	}
	return names
}

// skipArrowHead skips `(params): T =>` or `x =>` and returns the body start
// and the parameter names (nil when this is not an arrow function).
func (e *jsEval) skipArrowHead(j int) (int, []string) {
	t := e.toks
	k := j
	if k < len(t) && t[k].text == "async" {
		k++
	}
	var params []string
	if k < len(t) && t[k].text == "(" {
		params = e.paramNames(k)
		k = e.skipBalanced(k)
	} else if k < len(t) && t[k].kind == tokIdent {
		params = []string{t[k].text}
		k++
	} else {
		return j, nil
	}
	// optional return type annotation
	if k < len(t) && t[k].text == ":" {
		for k < len(t) && !(t[k].text == "=" && k+1 < len(t) && t[k+1].text == ">") && t[k].text != ";" {
			k++
		}
	}
	if k+1 < len(t) && t[k].text == "=" && t[k+1].text == ">" {
		return k + 2, params
	}
	return j, nil
}

// skipBalanced returns the index just past the bracket group opening at j.
func (e *jsEval) skipBalanced(j int) int {
	t := e.toks
	open := t[j].text
	closeCh := map[string]string{"(": ")", "[": "]", "{": "}"}[open]
	depth := 0
	for k := j; k < len(t); k++ {
		if t[k].kind != tokPunct {
			continue
		}
		switch t[k].text {
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return k + 1
			}
		}
	}
	return len(t)
}

func product(a, b []string) []string {
	var out []string
	for _, x := range a {
		for _, y := range b {
			if len(out) >= maxAlternatives {
				return out
			}
			out = append(out, x+y)
		}
	}
	return out
}

// chain evaluates `term (+ term)*` from j. It returns the possible values,
// the index after the chain, and whether any string literal took part.
func (e *jsEval) chain(j int) ([]string, int, bool) {
	acc := []string{""}
	lit := false
	for {
		vs, next, isLit, ok := e.term(j)
		if !ok {
			return acc, j, lit
		}
		acc = product(acc, vs)
		lit = lit || isLit
		j = next
		if j < len(e.toks) && e.toks[j].kind == tokPunct && e.toks[j].text == "+" &&
			!(j+1 < len(e.toks) && e.toks[j+1].text == "+") {
			j++
			continue
		}
		return acc, j, lit
	}
}

func (e *jsEval) paramMarker(name string) (string, bool) {
	for i, p := range e.params {
		if p == name {
			return "\x01" + string(rune('0'+i)) + "\x02", true
		}
	}
	return "", false
}

// callArgs evaluates the comma-separated arguments of the group at j.
func (e *jsEval) callArgs(j int) [][]string {
	t := e.toks
	end := e.skipBalanced(j)
	var args [][]string
	k := j + 1
	for k < end-1 {
		vs, next, _ := e.chain(k)
		if next == k {
			vs = []string{placeholder}
		}
		args = append(args, vs)
		// skip to the next top-level comma
		depth := 0
		for k = next; k < end-1; k++ {
			if t[k].kind == tokPunct {
				switch t[k].text {
				case "(", "[", "{":
					depth++
				case ")", "]", "}":
					depth--
				case ",":
					if depth == 0 {
						goto nextArg
					}
				}
			}
		}
		break
	nextArg:
		k++
	}
	return args
}

func substitute(values []string, args [][]string) []string {
	out := values
	for i := 0; i < 10; i++ {
		marker := "\x01" + string(rune('0'+i)) + "\x02"
		var next []string
		for _, v := range out {
			if !strings.Contains(v, marker) {
				next = append(next, v)
				continue
			}
			repl := []string{placeholder}
			if i < len(args) {
				repl = args[i]
			}
			for _, r := range repl {
				if len(next) < maxAlternatives {
					next = append(next, strings.ReplaceAll(v, marker, r))
				}
			}
		}
		out = next
	}
	return out
}

func (e *jsEval) term(j int) ([]string, int, bool, bool) {
	t := e.toks
	if j >= len(t) {
		return nil, j, false, false
	}
	switch t[j].kind {
	case tokString:
		return []string{t[j].text}, j + 1, true, true
	case tokTemplate:
		return e.template(t[j].text), j + 1, true, true
	case tokIdent:
		name := t[j].text
		if m, ok := e.paramMarker(name); ok && !(j+1 < len(t) && t[j+1].text == ".") {
			return []string{m}, j + 1, false, true
		}
		k := j + 1
		member := false
		var args [][]string
		called := false
		for k < len(t) && t[k].kind == tokPunct {
			switch t[k].text {
			case "(", "[":
				if t[k].text == "(" && !member && !called {
					args = e.callArgs(k)
					called = true
				}
				k = e.skipBalanced(k)
				continue
			case ".", "?", "!":
				if k+1 < len(t) && t[k+1].kind == tokIdent {
					member = true
					k += 2
					continue
				}
				if t[k].text == "?" && k+1 < len(t) && t[k+1].text == "." {
					k++
					continue
				}
				if t[k].text == "!" {
					k++
					continue
				}
			}
			break
		}
		if d, ok := e.defs[name]; ok && !member {
			return substitute(d.values, args), k, true, true
		}
		return []string{placeholder}, k, false, true
	case tokPunct:
		if t[j].text == "(" {
			k := e.skipBalanced(j)
			return []string{placeholder}, k, false, true
		}
	}
	return nil, j, false, false
}

var ternary = regexp.MustCompile(`^[^?]+\?\s*(?:"([^"]*)"|'([^']*)')\s*:\s*(?:"([^"]*)"|'([^']*)')$`)

// template resolves `${name}` / `${name(...)}` against known definitions,
// expands `${c ? "a" : "b"}` into both values and turns every other
// interpolation into the placeholder.
func (e *jsEval) template(body string) []string {
	acc := []string{""}
	for {
		i := strings.Index(body, "${")
		if i < 0 {
			return product(acc, []string{body})
		}
		acc = product(acc, []string{body[:i]})
		depth := 0
		end := -1
		for k := i + 2; k < len(body); k++ {
			if body[k] == '{' {
				depth++
			} else if body[k] == '}' {
				if depth == 0 {
					end = k
					break
				}
				depth--
			}
		}
		if end < 0 {
			return product(acc, []string{placeholder})
		}
		expr := strings.TrimSpace(body[i+2 : end])
		acc = product(acc, e.interpolation(expr))
		body = body[end+1:]
	}
}

func (e *jsEval) interpolation(expr string) []string {
	if m := ternary.FindStringSubmatch(expr); m != nil {
		return []string{m[1] + m[2], m[3] + m[4]}
	}
	if marker, ok := e.paramMarker(expr); ok {
		return []string{marker}
	}
	// Re-read the expression as code so helpers and literals resolve.
	sub := &jsEval{toks: jsTokenize(expr), defs: e.defs, params: e.params}
	if vs, next, lit := sub.chain(0); lit && next == len(sub.toks) {
		return vs
	}
	return []string{placeholder}
}
