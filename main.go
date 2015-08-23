// TODO: README for cmds
// TODO: Custom templates
// TODO: Godoc format to Markdown

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
	Author   author
}

type author struct {
	Name string
}

var tmpl = template.Must(template.New("readme").Funcs(template.FuncMap{
	"code": func(node *ast.Node) string {
		return ""
	},
}).Parse(
	`# {{.Pkg.Name}}

[![GoDoc](https://godoc.org/{{.Pkg.ImportPath}}?status.svg)](https://godoc.org/{{.Pkg.ImportPath}})

{{.Pkg.Doc}}
{{if (len .Examples)}}## Examples
{{range .Examples}}
### {{.Name}}

` + "```" + `go
{{.Code|code}}
` + "```" + `
{{if .Output}}
Output:
` + "```" + `
{{.Output}}
` + "```" + `
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

	r.Author = author{
		Name: regexp.MustCompile(`^github\.com/([^/]+)`).FindStringSubmatch(r.Pkg.ImportPath)[1],
	}

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
