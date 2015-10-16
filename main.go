// goreadme generates an (opinionated) READMEs for your Go packages.
// it extracts informatino from the source code and tests, then generates
// a Markdown content suitable as a README boilerplate.
//
//   goreadme [.] > README.md
//
// For the default template, run `go doc github.com/motemen/goreadme.DefaultTemplate`.
package main

// TODO(motemen): Make author information correct
// TODO(motemen): Show only toplevel todos?

import (
	"bytes"
	"flag"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
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
	Badges   []string
}

func (r Readme) IsCommand() bool {
	return r.Pkg.Name == "main"
}

func (r Readme) Name() string {
	if r.IsCommand() {
		// this package should be a command
		parts := strings.Split(r.Pkg.ImportPath, "/")
		return parts[len(parts)-1]
	}

	return r.Pkg.Name
}

type Author struct {
	Name string
}

var (
	patExportedIdent = `\p{Lu}[\pL_0-9]*`
	patPkgPath       = `(?:[-a-z0-9.:]+/)*[-a-z0-9]+`
)

var predefCodePatterns = []string{
	"interface",
	"struct",
	`(?:` + patPkgPath + `\.)?` + patExportedIdent + `\.` + patExportedIdent,
	patPkgPath + `\.` + `(?:` + patExportedIdent + `\.)?` + patExportedIdent,
	`\(\*` + `(?:` + patPkgPath + `\.)?` + patExportedIdent + `\)\.` + patExportedIdent,
}

func mkCodeRegexp(idents []string) *regexp.Regexp {
	return regexp.MustCompile(
		`(^|\s)((?:` + strings.Join(predefCodePatterns, "|") + `)` +
			`(?:\{.*?\}|\[.*?\]|\(.*?\))?)([.,]|\s|$)`,
	)
}

// The default README template.
var DefaultTemplate = `# {{.Name}}

{{if (not .IsCommand)}}
[![GoDoc](https://godoc.org/{{.Pkg.ImportPath}}?status.svg)](https://godoc.org/{{.Pkg.ImportPath}}){{end}}
{{range .Badges}}{{.}}
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

func main() {
	tmplFile := flag.String("f", "", "template file")
	flag.Parse()

	dir := "."

	args := flag.Args()
	if len(args) >= 1 {
		dir = args[0]
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

	// Collect badges
	if _, err := os.Stat(filepath.Join(bpkg.Dir, ".travis.yml")); err == nil {
		if strings.HasPrefix(bpkg.ImportPath, "github.com/") {
			// [![Build Status](https://travis-ci.org/motemen/go-sqlf.svg?branch=master)](https://travis-ci.org/motemen/go-sqlf)
			branch := "master"

			path := bpkg.ImportPath[len("github.com/"):]
			cmd := exec.Command("git", "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
			cmd.Dir = bpkg.Dir
			if out, err := cmd.CombinedOutput(); err != nil {
				b := strings.TrimSpace(string(out))
				if strings.HasPrefix(b, "origin/") {
					branch = b[len("origin/"):]
				}
			}

			r.Badges = append(r.Badges, fmt.Sprintf(
				"[![Build Status](https://travis-ci.org/%s.svg?branch=%s)](https://travis-ci.org/%s)",
				path, branch, path,
			))
		}
	}

	r.Author = Author{
		Name: regexp.MustCompile(`^github\.com/([^/]+)`).FindStringSubmatch(r.Pkg.ImportPath)[1],
	}

	tmpl := template.New("readme").Funcs(template.FuncMap{
		"code": func(v interface{}) string {
			s, err := renderCode(fset, v)
			if err != nil {
				panic(err)
			}
			return s
		},
		"markdown": func(d string) string {
			return renderMarkdown(d, r.Exports)
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
	os.Stdout.WriteString(squeezeEmptyLines(buf.String()))
}

func pkgFiles(pkg *ast.Package) []*ast.File {
	ff := make([]*ast.File, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		ff = append(ff, f)
	}
	return ff
}

var (
	rxParseHTML = regexp.MustCompile(
		`<pre\b[^>]*>(?P<pre>[^<]*)</pre>` +
			`|<h3\b[^>]*>(?P<heading>[^<]*)</h3>` +
			`|<p\b[^>]*>\n?(?P<para>(?s:.)*?)</p>`,
	)
	rxStripTag = regexp.MustCompile(`<[^>]*>`)
)

func renderMarkdown(docString string, idents []string) string {
	var out bytes.Buffer

	rxCode := mkCodeRegexp(idents)

	var docHTML string
	{
		var bufHTML bytes.Buffer
		doc.ToHTML(&bufHTML, docString, nil)
		docHTML = bufHTML.String()
	}

	for {
		m := rxParseHTML.FindStringSubmatchIndex(docHTML)
		if m == nil {
			break
		}

		for i, name := range rxParseHTML.SubexpNames() {
			if i == 0 {
				continue
			}
			if m[i*2] == -1 {
				continue
			}

			s := html.UnescapeString(docHTML[m[i*2]:m[i*2+1]])

			if name == "pre" {
				lines := strings.SplitAfter(s, "\n")
				for i, line := range lines {
					if i == len(lines)-1 && line == "" {
						// nop
					} else {
						out.WriteString("    ")
					}
					out.WriteString(line)
				}
				out.WriteString("\n")
			} else if name == "heading" {
				out.WriteString("## ")
				out.WriteString(s)
				out.WriteString("\n\n")
			} else {
				s = rxStripTag.ReplaceAllString(s, "")
				s = rxCode.ReplaceAllString(s, "$1`$2`$3")
				s = regexp.MustCompile(`[_]`).ReplaceAllString(s, `\_`)
				out.WriteString(s)
				out.WriteString("\n")
			}
		}

		docHTML = docHTML[m[1]:]
	}

	return out.String()
}

var rxOutputPrefix = regexp.MustCompile(`(?i)^[[:space:]]*output:`)

func renderCode(fset *token.FileSet, v interface{}) (string, error) {
	printerConfig := printer.Config{
		Tabwidth: 4,
		Mode:     printer.UseSpaces,
	}

	var buf bytes.Buffer

	if node, ok := v.(ast.Node); ok {
		var err error
		if block, ok := node.(*ast.BlockStmt); ok {
			err = printerConfig.Fprint(&buf, fset, block.List)
		} else {
			err = printerConfig.Fprint(&buf, fset, node)
		}
		return buf.String(), err
	}

	if ex, ok := v.(*doc.Example); ok {
		// Try to remove "Output:" comments
		comments := make([]*ast.CommentGroup, 0, len(ex.Comments))
		var outputComment *ast.CommentGroup
		for _, c := range ex.Comments {
			if rxOutputPrefix.MatchString(c.Text()) {
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
			err := printerConfig.Fprint(&buf, fset, &node)
			return buf.String(), err
		} else if block, ok := ex.Code.(*ast.BlockStmt); ok {
			// XXX dirty hack: we need BlockStmt code without indentation;
			// so here we make a fake "switch" statement and remove the
			// outermost braces.
			node := printer.CommentedNode{
				Node:     &ast.SwitchStmt{Body: block},
				Comments: comments,
			}

			var b bytes.Buffer

			err := printerConfig.Fprint(&b, fset, &node)
			if err != nil {
				return "", err
			}

			s := b.String()
			if strings.HasPrefix(s, "switch {\n") && strings.HasSuffix(s, "\n}") {
				s = s[len("switch {\n") : len(s)-len("\n}")]
				return s, nil
			}
		}

		node := printer.CommentedNode{
			Node:     ex.Code,
			Comments: comments,
		}
		err := printerConfig.Fprint(&buf, fset, &node)
		return buf.String(), err
	}

	return "", fmt.Errorf("cannot handle %T", v)
}

var rxEmptyLines = regexp.MustCompile(`\n{3,}`)

func squeezeEmptyLines(s string) string {
	return rxEmptyLines.ReplaceAllString(s, "\n\n")
}
