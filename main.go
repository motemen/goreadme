// TODO: Custom templates

// goreadme generates an (opinionated) READMEs for your Go packages.
// it extracts informatino from the source code and tests, then generates
// a Markdown content suitable as a README boilerplate.
//
//   goreadme [.] > README.md
//
// For default template, run `go doc github.com/motemen/goreadme.DefaultTemplate`.
package main

import (
	"bytes"
	"flag"
	"io/ioutil"
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

type Readme struct {
	fset     *token.FileSet
	Pkg      *doc.Package
	Examples []*doc.Example
	Exports  []string
	Author   Author
}

func (r Readme) IsCommand() bool {
	return r.Pkg.Name == "main"
}

func (r Readme) Name() string {
	if r.Pkg.Name == "main" {
		// this package should be a command
		parts := strings.Split(r.Pkg.ImportPath, "/")
		return parts[len(parts)-1]
	}

	return r.Pkg.Name
}

type Author struct {
	Name string
}

var predefCodePatterns = []string{
	"interface",
	`[a-z]+\.[A-Z]\w*`, // e.g. "http.Client"
}

// The default README template.
var DefaultTemplate = `# {{.Name}}

{{if (not .IsCommand)}}
[![GoDoc](https://godoc.org/{{.Pkg.ImportPath}}?status.svg)](https://godoc.org/{{.Pkg.ImportPath}})
{{end}}

{{.Pkg.Doc|markdown}}

{{if .IsCommand}}
## Installation

    go get -u {{.Pkg.ImportPath}}

{{end}}

{{if (len .Examples)}}
## Examples
{{  range .Examples}}
### {{.Name}}

{{.Code|code|fence "go"}}
{{    if .Output}}
Output:

{{.Output|fence ""}}
{{    end}}
{{  end}}
{{end}}

## Author

{{.Author.Name}} <https://github.com/{{.Author.Name}}>
`

func main() {
	tmplFile := flag.String("f", "", "template file")
	flag.Parse()

	dir := "."

	args := flag.Args()
	if len(args) >= 2 {
		dir = args[1]
	}

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

	r := Readme{}

	for name, pkg := range pkgs {
		log.Println(name, pkg)

		r.Examples = append(r.Examples, doc.Examples(pkgFiles(pkg)...)...)

		if strings.HasSuffix(name, "_test") {
			continue
		}

		if r.Pkg == nil {
			r.Pkg = doc.New(pkg, bpkg.ImportPath, doc.Mode(0))
		}
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

	r.Author = Author{
		Name: regexp.MustCompile(`^github\.com/([^/]+)`).FindStringSubmatch(r.Pkg.ImportPath)[1],
	}

	var (
		rxCode = regexp.MustCompile(
			`(\b(?:` + strings.Join(append(r.Exports, predefCodePatterns...), "|") + `)\b` +
				`(?:\{.*?\}|\[.*?\]|\(.*?\))?)`,
		)
		rxIndent     = regexp.MustCompile(`^\s+\S`)
		rxEmptyLines = regexp.MustCompile(`\n{3,}`)
	)

	tmpl := template.New("readme").Funcs(template.FuncMap{
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
			inCodeBlock := false
			for i, line := range lines {
				if rxIndent.MatchString(line) {
					line = "    " + strings.TrimSpace(line)
					if !inCodeBlock {
						line = "\n" + line
					}
					inCodeBlock = true
				} else {
					inCodeBlock = false
				}
				line = rxCode.ReplaceAllString(line, "`$1`")
				lines[i] = line
			}
			return strings.Join(lines, "\n")
		},
		"fence": func(ft, s string) string {
			if !strings.HasSuffix(s, "\n") {
				s = s + "\n"
			}
			return "```" + ft + "\n" + s + "```\n"
		},
	})

	tmplContent := DefaultTemplate
	if *tmplFile != "" {
		b, err := ioutil.ReadFile(*tmplFile)
		if err != nil {
			log.Fatal(err)
		}
		tmplContent = string(b)
	}

	template.Must(tmpl.Parse(tmplContent))

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, r)

	if err != nil {
		log.Fatal(err)
	}

	// drop successive empty lines
	os.Stdout.WriteString(rxEmptyLines.ReplaceAllString(buf.String(), "\n\n"))
}

func pkgFiles(pkg *ast.Package) []*ast.File {
	ff := make([]*ast.File, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		ff = append(ff, f)
	}
	return ff
}
