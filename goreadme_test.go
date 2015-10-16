package main

import (
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	cases := []struct {
		from string
		to   string
	}{
		{
			from: "Package loghttp provides automatic logging functionalities to http.Client.",
			to:   "Package loghttp provides automatic logging functionalities to `http.Client`.\n",
		},
		{
			from: `goreadme generates an (opinionated) READMEs for your Go packages.
it extracts informatino from the source code and tests, then generates
a Markdown content suitable as a README boilerplate.

  goreadme [.] > README.md

` + "For the default template, run `go doc github.com/motemen/goreadme.DefaultTemplate`.",
			to: `goreadme generates an (opinionated) READMEs for your Go packages.
it extracts informatino from the source code and tests, then generates
a Markdown content suitable as a README boilerplate.

    goreadme [.] > README.md

` + "For the default template, run `go doc github.com/motemen/goreadme.DefaultTemplate`.\n",
		},
		{
			from: `
    foo bar
  x   1   2
  y   3   4
`,
			to: `      foo bar
    x   1   2
    y   3   4

`,
		},
	}

	for _, c := range cases {
		rendered := squeezeEmptyLines(renderMarkdown(c.from, []string{}))
		if rendered != c.to {
			t.Errorf("renderMarkdown mismatch:\nGot ---\n%q\nExpected ---\n%q\n", rendered, c.to)
		}
	}
}
