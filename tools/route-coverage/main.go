// Command route-coverage lists every backend HTTP route that no first-party
// client calls.
//
// It reads the gin route table straight from the backend source (every
// .GET/.POST/... call under backend/cmd/app and backend/internal, with
// router-group prefixes resolved, including groups passed into helper
// functions), then looks for a caller of each route in:
//
//   - frontend/src                     (the web app)
//   - tools/payverge-admin-mcp/src     (the operator MCP server)
//   - docs/api                         (the public API reference)
//
// A caller is any string or template literal whose path matches the route:
// `${...}` in a template literal matches any one path segment, and a route
// parameter (:id, *key) matches any segment. A leading `${base}` and the
// `/api/v1` prefix are ignored, as are query strings.
//
// Routes that legitimately have no first-party caller (webhooks, OAuth
// callbacks, probes, scrapers, links in emails) are listed in allowlist.txt
// with a reason. The command fails when a route is uncovered and not
// allowlisted, and when an allowlist entry no longer names an uncovered
// route, so the list cannot rot.
//
// Matching is by path, not by method: a client that calls GET /x also covers
// DELETE /x. That keeps the evaluator simple and errs towards "covered".
//
// Usage (the tool is its own Go module; the repository root is not one):
//
//	cd tools/route-coverage && go run . -repo ../..         # check against allowlist.txt
//	cd tools/route-coverage && go run . -repo ../.. -list   # print every uncovered route
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type route struct {
	method string
	path   string // full path, e.g. /api/v1/inside/businesses/:id
	pos    string
}

func (r route) key() string { return r.method + " " + r.path }

func main() {
	repo := flag.String("repo", ".", "repository root")
	list := flag.Bool("list", false, "print every uncovered route and exit 0")
	allowPath := flag.String("allowlist", "", "allowlist file (default tools/route-coverage/allowlist.txt)")
	flag.Parse()
	if *allowPath == "" {
		*allowPath = filepath.Join(*repo, "tools", "route-coverage", "allowlist.txt")
	}

	routes, err := collectRoutes(filepath.Join(*repo, "backend"))
	if err != nil {
		fail(err)
	}
	callers, err := collectCallers(*repo)
	if err != nil {
		fail(err)
	}

	var uncovered []route
	for _, r := range routes {
		if !covered(r, callers) {
			uncovered = append(uncovered, r)
		}
	}
	sort.Slice(uncovered, func(i, j int) bool { return uncovered[i].key() < uncovered[j].key() })

	if *list {
		for _, r := range uncovered {
			fmt.Printf("%s\t%s\n", r.key(), r.pos)
		}
		fmt.Fprintf(os.Stderr, "%d routes, %d uncovered\n", len(routes), len(uncovered))
		return
	}

	allow, err := readAllowlist(*allowPath)
	if err != nil {
		fail(err)
	}
	problems := 0
	seen := map[string]bool{}
	for _, r := range uncovered {
		seen[r.key()] = true
		if _, ok := allow[r.key()]; !ok {
			fmt.Printf("uncovered route with no first-party caller: %s (%s)\n", r.key(), r.pos)
			problems++
		}
	}
	for k := range allow {
		if !seen[k] {
			fmt.Printf("stale allowlist entry (route removed or now called): %s\n", k)
			problems++
		}
	}
	fmt.Printf("route-coverage: %d routes, %d uncovered, %d allowlisted\n", len(routes), len(uncovered), len(allow))
	if problems > 0 {
		fmt.Println("Remove the route and its handler, add a client, or allowlist it with a reason in tools/route-coverage/allowlist.txt.")
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "route-coverage:", err)
	os.Exit(2)
}

// ---------------------------------------------------------------- routes

var routeMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "Any": true}

type funcInfo struct {
	decl     *ast.FuncDecl
	file     string
	fset     *token.FileSet
	ginParam map[string]int // param name -> index, for gin router params
}

func collectRoutes(backend string) ([]route, error) {
	fset := token.NewFileSet()
	funcs := map[string][]*funcInfo{}
	var all []*funcInfo
	for _, dir := range []string{"cmd/app", "internal"} {
		err := filepath.WalkDir(filepath.Join(backend, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// Test-only routers (integration build tag) are not the product.
			if strings.Contains(string(src), "//go:build integration") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				fi := &funcInfo{decl: fd, file: path, fset: fset, ginParam: map[string]int{}}
				idx := 0
				for _, field := range fd.Type.Params.List {
					isGin := isGinRouterType(field.Type)
					n := len(field.Names)
					if n == 0 {
						n = 1
					}
					for i := 0; i < n; i++ {
						if isGin && len(field.Names) > 0 {
							fi.ginParam[field.Names[i].Name] = idx
						}
						idx++
					}
				}
				funcs[fd.Name.Name] = append(funcs[fd.Name.Name], fi)
				all = append(all, fi)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// paramPrefixes[func][param] = set of prefixes callers pass in.
	paramPrefixes := map[*funcInfo]map[string]map[string]bool{}
	routes := map[string]route{}

	// Iterate to a fixpoint: a pass may discover new prefixes for a helper's
	// router parameter, which a later pass then uses.
	for pass := 0; pass < 6; pass++ {
		changed := false
		for _, fi := range all {
			scope := map[string]map[string]bool{}
			for p := range fi.ginParam {
				scope[p] = copySet(paramPrefixes[fi][p])
			}
			ast.Inspect(fi.decl.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
						return true
					}
					lhs, ok := n.Lhs[0].(*ast.Ident)
					if !ok {
						return true
					}
					call, ok := n.Rhs[0].(*ast.CallExpr)
					if !ok {
						return true
					}
					if isGinNew(call) {
						scope[lhs.Name] = map[string]bool{"": true}
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Group" || len(call.Args) == 0 {
						return true
					}
					parent, ok := sel.X.(*ast.Ident)
					if !ok {
						return true
					}
					prefix, ok := stringLit(call.Args[0])
					if !ok {
						return true
					}
					set := map[string]bool{}
					for p := range scope[parent.Name] {
						set[joinPath(p, prefix)] = true
					}
					scope[lhs.Name] = set
				case *ast.CallExpr:
					// Route registration.
					if sel, ok := n.Fun.(*ast.SelectorExpr); ok && routeMethods[sel.Sel.Name] && len(n.Args) >= 2 {
						if recv, ok := sel.X.(*ast.Ident); ok {
							if p, ok := stringLit(n.Args[0]); ok {
								for prefix := range scope[recv.Name] {
									full := joinPath(prefix, p)
									methods := []string{sel.Sel.Name}
									if sel.Sel.Name == "Any" {
										methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
									}
									for _, m := range methods {
										r := route{method: m, path: full, pos: relPos(fi.fset.Position(n.Pos()), backend)}
										routes[r.key()] = r
									}
								}
							}
						}
					}
					// Router passed into a helper.
					name := calleeName(n)
					for _, target := range funcs[name] {
						for pname, idx := range target.ginParam {
							if idx >= len(n.Args) {
								continue
							}
							arg, ok := n.Args[idx].(*ast.Ident)
							if !ok {
								continue
							}
							for p := range scope[arg.Name] {
								if paramPrefixes[target] == nil {
									paramPrefixes[target] = map[string]map[string]bool{}
								}
								if paramPrefixes[target][pname] == nil {
									paramPrefixes[target][pname] = map[string]bool{}
								}
								if !paramPrefixes[target][pname][p] {
									paramPrefixes[target][pname][p] = true
									changed = true
								}
							}
						}
					}
				}
				return true
			})
		}
		if !changed {
			break
		}
	}

	out := make([]route, 0, len(routes))
	for _, r := range routes {
		// HEAD mirrors a GET for the same path; coverage follows the GET.
		if r.method == "HEAD" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no routes found under %s", backend)
	}
	return out, nil
}

func isGinRouterType(e ast.Expr) bool {
	if st, ok := e.(*ast.StarExpr); ok {
		e = st.X
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "gin" {
		return false
	}
	switch sel.Sel.Name {
	case "Engine", "RouterGroup", "IRoutes", "IRouter":
		return true
	}
	return false
}

func isGinNew(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "gin" && (sel.Sel.Name == "New" || sel.Sel.Name == "Default")
}

func calleeName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func joinPath(prefix, p string) string {
	if p == "" {
		return strings.TrimSuffix(prefix, "/")
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(p, "/")
}

func copySet(s map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range s {
		out[k] = true
	}
	return out
}

func relPos(p token.Position, backend string) string {
	rel, err := filepath.Rel(filepath.Dir(backend), p.Filename)
	if err != nil {
		rel = p.Filename
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), p.Line)
}

// ---------------------------------------------------------------- callers

// placeholder stands for a `${...}` template expression: any one segment.
const placeholder = "\x00"

var (
	tmplExpr = regexp.MustCompile(`\$\{[^}]*\}`)
	docPath  = regexp.MustCompile("/api/v1/[A-Za-z0-9_\\-./:{}*]+")
)

func collectCallers(repo string) ([][]string, error) {
	seen := map[string]bool{}
	var out [][]string
	add := func(raw string) {
		segs := normalise(raw)
		if segs == nil {
			return
		}
		k := strings.Join(segs, "/")
		if !seen[k] {
			seen[k] = true
			out = append(out, segs)
		}
	}

	codeExt := map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true}
	for _, dir := range []string{"frontend/src", "tools/payverge-admin-mcp/src"} {
		err := filepath.WalkDir(filepath.Join(repo, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "__tests__" || d.Name() == "__mocks__" {
					return filepath.SkipDir
				}
				return nil
			}
			base := d.Name()
			if !codeExt[filepath.Ext(base)] || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, v := range jsPathValues(string(src)) {
				add(v)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	err := filepath.WalkDir(filepath.Join(repo, "docs", "api"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range docPath.FindAllString(string(src), -1) {
			add(strings.TrimRight(m, ".:"))
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}

// normalise turns a client string into path segments, or nil when the string
// is not a path. `${...}` becomes the placeholder segment.
func normalise(s string) []string {
	s = tmplExpr.ReplaceAllString(s, placeholder)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	// Drop an absolute origin or a leading `${base}`.
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		j := strings.Index(rest, "/")
		if j < 0 {
			return nil
		}
		s = rest[j:]
	}
	s = strings.TrimPrefix(s, placeholder)
	if !strings.HasPrefix(s, "/") || strings.ContainsAny(s, " \t\n<>") {
		return nil
	}
	s = strings.TrimPrefix(s, "/api/v1")
	s = strings.Trim(s, "/")
	if s == "" {
		return nil
	}
	segs := strings.Split(s, "/")
	for i, seg := range segs {
		if strings.Contains(seg, placeholder) {
			segs[i] = placeholder
		} else if strings.HasPrefix(seg, ":") || (strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")) {
			segs[i] = placeholder
		}
	}
	return segs
}

func covered(r route, callers [][]string) bool {
	p := strings.Trim(strings.TrimPrefix(r.path, "/api/v1"), "/")
	rsegs := strings.Split(p, "/")
	for _, c := range callers {
		if matchSegs(rsegs, c) {
			return true
		}
	}
	return false
}

func matchSegs(route, caller []string) bool {
	if len(route) > 0 && strings.HasPrefix(route[len(route)-1], "*") {
		// Catch-all: caller must cover the prefix and add at least one segment.
		fixed := route[:len(route)-1]
		if len(caller) <= len(fixed) {
			return false
		}
		return matchSegs(fixed, caller[:len(fixed)])
	}
	if len(route) != len(caller) {
		return false
	}
	for i := range route {
		if strings.HasPrefix(route[i], ":") || caller[i] == placeholder {
			continue
		}
		if route[i] != caller[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- allowlist

// readAllowlist parses lines of `METHOD /path  # reason`. A reason is required.
func readAllowlist(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		entry, reason, _ := strings.Cut(text, "#")
		fields := strings.Fields(entry)
		if len(fields) != 2 || strings.TrimSpace(reason) == "" {
			return nil, fmt.Errorf("%s:%d: want `METHOD /path  # reason`", path, line)
		}
		k := fields[0] + " " + fields[1]
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("%s:%d: duplicate entry %s", path, line, k)
		}
		out[k] = strings.TrimSpace(reason)
	}
	return out, sc.Err()
}
