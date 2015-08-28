// TODO: README for cmds
// TODO: Custom templates
// TODO: Godoc format to Markdown

// goreadme generates an (opinionated) READMEs for your Go packages.
// it extracts informatino from the source code and tests, then generates
// a Markdown content suitable as a README boilerplate.
//
//   goreadme [.] > README.md
package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
)

type readme struct {
	Fset     *token.FileSet
	Pkg      *doc.Package
	Examples []*doc.Example
	Exports  []string
	Author   author
}

func (r readme) IsCommand() bool {
	return r.Pkg.Name == "main"
}

func (r readme) Name() string {
	if r.Pkg.Name == "main" {
		// this package should be a command
		parts := strings.Split(r.Pkg.ImportPath, "/")
		return parts[len(parts)-1]
	}

	return r.Pkg.Name
}

type author struct {
	Name string
}

var predefCodePatterns = []string{
	"interface",
	`[a-z]+\.[A-Z]\w*`, // e.g. "http.Client"
}

var tmpl = template.Must(template.New("readme").Funcs(template.FuncMap{
	"code":     func() string { return "" },
	"markdown": func() string { return "" },
	"fence": func(ft, s string) string {
		if !strings.HasSuffix(s, "\n") {
			s = s + "\n"
		}
		return "```" + ft + "\n" + s + "```\n"
	},
}).Parse(
	`# {{.Name}}

{{if (not .IsCommand)}}[![GoDoc](https://godoc.org/{{.Pkg.ImportPath}}?status.svg)](https://godoc.org/{{.Pkg.ImportPath}})
{{end}}
{{.Pkg.Doc|markdown}}
{{if (len .Examples)}}## Examples
{{range .Examples}}
### {{.Name}}

` + `{{.Code|code|fence "go"}}{{if .Output}}
Output:

{{.Output|fence ""}}
{{end}}{{end}}{{end}}
## Author

{{.Author.Name}} <https://github.com/{{.Author.Name}}>
`))

func main() {
	dir := "."

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	bpkg, err := build.ImportDir(filepath.Join(wd, dir), build.FindOnly)
	if err != nil {
		log.Fatal(err)
	}

	r := readme{}

	for name, pkg := range pkgs {
		r.Examples = append(r.Examples, doc.Examples(pkgFiles(pkg)...)...)

		if strings.HasSuffix(name, "_test") {
			continue
		}

		r.Pkg = doc.New(pkg, bpkg.ImportPath, doc.Mode(0))

		break
	}

	for _, v := range append(r.Pkg.Consts, r.Pkg.Vars...) {
		r.Exports = append(r.Exports, v.Names...)
	}

	for _, f := range r.Pkg.Funcs {
		r.Exports = append(r.Exports, f.Name)
	}

	for _, t := range r.Pkg.Funcs {
		r.Exports = append(r.Exports, t.Name)
	}

	r.Author = author{
		Name: regexp.MustCompile(`^github\.com/([^/]+)`).FindStringSubmatch(r.Pkg.ImportPath)[1],
	}

	var (
		rxCode = regexp.MustCompile(
			`(\b(?:` + strings.Join(append(r.Exports, predefCodePatterns...), "|") + `)\b` +
				`(?:\{.*?\}|\[.*?\]|\(.*?\))?)`,
		)
		rxIndent = regexp.MustCompile(`^ {1,3}(\S)`)
	)

	err = tmpl.Funcs(template.FuncMap{
		"code": func(node ast.Node) string {
			var buf bytes.Buffer
			if block, ok := node.(*ast.BlockStmt); ok {
				printer.Fprint(&buf, fset, block.List)
			} else {
				printer.Fprint(&buf, fset, node)
			}
			return buf.String()
		},
		"markdown": func(doc string) string {
			lines := strings.Split(doc, "\n")
			for i, line := range lines {
				line = rxIndent.ReplaceAllString(line, "    $1")
				line = rxCode.ReplaceAllString(line, "`$1`")
				lines[i] = line
			}
			return strings.Join(lines, "\n")
		},
	}).Execute(os.Stdout, r)

	if err != nil {
		log.Fatal(err)
	}
}

func pkgFiles(pkg *ast.Package) []*ast.File {
	ff := make([]*ast.File, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		ff = append(ff, f)
	}
	return ff
}
