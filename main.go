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
	"fmt"
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

{{.|code|fence "go"}}
{{    if .Output}}
Output:

{{.Output|fence ""}}
{{    end}}
{{  end}}
{{end}}

{{if .Pkg.Notes.TODO}}
## TODO

{{range .Pkg.Notes.TODO}}- {{.Body}}{{end}}
{{end}}

## Author

{{.Author.Name}} <https://github.com/{{.Author.Name}}>
`

var outputPrefix = regexp.MustCompile(`(?i)^[[:space:]]*output:`)

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
		r.Examples = append(r.Examples, doc.Examples(pkgFiles(pkg)...)...)

		if strings.HasSuffix(name, "_test") {
			continue
		}

		if r.Pkg == nil {
			r.Pkg = doc.New(pkg, bpkg.ImportPath, doc.Mode(0))
		}
	}

	if r.Pkg == nil {
		log.Fatal("no source found")
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

	printerConfig := printer.Config{
		Tabwidth: 4,
		Mode:     printer.UseSpaces,
	}

	tmpl := template.New("readme").Funcs(template.FuncMap{
		"code": func(v interface{}) string {
			var (
				buf bytes.Buffer
				err error
			)
			if node, ok := v.(ast.Node); ok {
				if block, ok := node.(*ast.BlockStmt); ok {
					err = printerConfig.Fprint(&buf, fset, block.List)
				} else {
					err = printerConfig.Fprint(&buf, fset, node)
				}
			} else if ex, ok := v.(*doc.Example); ok {
				// Try to remove "Output:" comments
				comments := make([]*ast.CommentGroup, 0, len(ex.Comments))
				var outputComment *ast.CommentGroup
				for _, c := range ex.Comments {
					if outputPrefix.MatchString(c.Text()) {
						outputComment = c
						continue
					}
					comments = append(comments, c)
				}

				if f := ex.Play; f != nil {
					for _, d := range f.Decls {
						if fun, ok := d.(*ast.FuncDecl); ok && fun.Name.Name == "main" {
							if fun.Pos() <= outputComment.Pos() && outputComment.Pos() <= fun.End() {
								fun.Body.Rbrace = fun.Body.List[len(fun.Body.List)-1].End()
							}
						}
					}

					node := printer.CommentedNode{
						Node:     f,
						Comments: comments,
					}
					err = printerConfig.Fprint(&buf, fset, &node)
				} else if block, ok := ex.Code.(*ast.BlockStmt); ok {
					// XXX dirty hack: we need BlockStmt code without indentation;
					// so here we make a fake "switch" statement and remove the
					// outermost braces.
					node := printer.CommentedNode{
						Node:     &ast.SwitchStmt{Body: block},
						Comments: comments,
					}

					var b bytes.Buffer

					printerConfig.Fprint(&b, fset, &node)

					s := b.String()
					if strings.HasPrefix(s, "switch {\n") && strings.HasSuffix(s, "\n}") {
						s = s[len("switch {\n") : len(s)-len("\n}")]
						buf.WriteString(s)
					}
				}
				if buf.Len() == 0 {
					node := printer.CommentedNode{
						Node:     ex.Code,
						Comments: comments,
					}
					err = printerConfig.Fprint(&buf, fset, &node)
				}
			} else {
				err = fmt.Errorf("cannot handle %T", v)
			}

			if err != nil {
				panic(err)
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
