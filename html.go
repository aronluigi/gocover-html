package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/tools/cover"
)

type templateData struct {
	Files []*templateFile
	Set   bool
}

type templateFile struct {
	Name     string
	Body     template.HTML
	Coverage float64
	ID       int
}

const (
	// HTMLTemplateFile - raw html template
	HTMLTemplateFile = "./res/index.html"
	// PrismCSS file
	PrismCSS = "./res/prism.css"
	// PrismJS file
	PrismJS = "./res/prism.js"
	// BootstrapCSS file
	BootstrapCSS = "./res/bootstrap.min.css"
	// BootstrapJS file
	BootstrapJS = "./res/bootstrap.min.js"
	// JQuery file
	JQuery = "./res/jquery-3.2.1.slim.min.js"
	// Popper file
	Popper = "./res/popper.min.js"
)

func removeArrayDuplicates(e []string) []string {
	enc := map[string]bool{}
	for v := range e {
		enc[e[v]] = true
	}

	res := []string{}

	for k := range enc {
		res = append(res, k)
	}

	return res
}

func getTemplate(buf *os.File, data *templateData) error {
	it := template.Must(template.ParseFiles(HTMLTemplateFile))

	prismCSS, err := ioutil.ReadFile(PrismCSS)
	if err != nil {
		return err
	}

	prismJS, err := ioutil.ReadFile(PrismJS)
	if err != nil {
		return err
	}

	bsCSS, err := ioutil.ReadFile(BootstrapCSS)
	if err != nil {
		return err
	}

	jq, err := ioutil.ReadFile(JQuery)
	if err != nil {
		return err
	}

	bsJS, err := ioutil.ReadFile(BootstrapJS)
	if err != nil {
		return err
	}

	popper, err := ioutil.ReadFile(Popper)
	if err != nil {
		return err
	}

	tplVals := map[string]interface{}{
		"prismCSS":     template.CSS(prismCSS),
		"bootstrapCSS": template.CSS(bsCSS),
		"prismJS":      template.JS(prismJS),
		"popper":       template.JS(popper),
		"jq":           template.JS(jq),
		"bootstrapJS":  template.JS(bsJS),
		"data":         data,
		"totalCov":     totalCoverage(data),
	}

	err = it.Execute(buf, tplVals)
	return err
}

// findFile finds the location of the named file in GOROOT, GOPATH etc.
func findFile(file string) (string, error) {
	dir, file := filepath.Split(file)
	pkg, err := build.Import(dir, ".", build.FindOnly)

	if err != nil {
		return "", fmt.Errorf("can't find %q: %v", file, err)
	}

	return filepath.Join(pkg.Dir, file), nil
}

// htmlGen generates an HTML coverage report with the provided filename,
// source code, and tokens, and writes it to the given Writer.
func htmlGen(w io.Writer, src []byte, profile *cover.Profile) error {
	dst := bufio.NewWriter(w)
	uncoverdLines := []string{}

	for _, block := range profile.Blocks {
		if block.Count != 0 {
			continue
		}

		l := fmt.Sprintf("%d-%d", block.StartLine, block.EndLine)
		uncoverdLines = append(uncoverdLines, l)
	}

	html := `<pre class=" line-numbers" data-line="%s"><code class="language-go">%s</code></pre>`
	uncoverdLines = removeArrayDuplicates(uncoverdLines)

	fmt.Fprintf(dst, html, strings.Join(uncoverdLines, ","), string(src))
	return dst.Flush()
}

// percentCovered returns, as a percentage, the fraction of the statements in
// the profile covered by the test run.
// In effect, it reports the coverage of a given source file.
func percentCovered(p *cover.Profile) float64 {
	var total, covered int64

	for _, b := range p.Blocks {
		total += int64(b.NumStmt)
		if b.Count > 0 {
			covered += int64(b.NumStmt)
		}
	}

	if total == 0 {
		return 0
	}

	return float64(covered) / float64(total) * 100
}

func totalCoverage(p *templateData) float64 {
	x := float64(0)

	for _, v := range p.Files {
		x += v.Coverage
	}

	return x / float64(len(p.Files))
}

func getTemplateData(profile string) (templateData, error) {
	var d templateData

	profiles, err := cover.ParseProfiles(profile)
	if err != nil {
		return d, err
	}

	for k, profile := range profiles {
		fn := profile.FileName

		if profile.Mode == "set" {
			d.Set = true
		}

		file, err := findFile(fn)
		if err != nil {
			return d, err
		}

		src, err := ioutil.ReadFile(file)
		if err != nil {
			return d, err
		}

		var buf bytes.Buffer
		err = htmlGen(&buf, src, profile)
		if err != nil {
			return d, err
		}

		d.Files = append(d.Files, &templateFile{
			Name:     fn,
			Body:     template.HTML(buf.String()),
			Coverage: percentCovered(profile),
			ID:       k,
		})
	}

	return d, nil
}

// htmlOutput reads the profile data from profile and generates an HTML
// coverage report, writing it to outfile. If outfile is empty,
// it writes the report to a temporary file and opens it in a web browser.
func htmlOutput(profile, outfile string) error {
	d, err := getTemplateData(profile)
	if err != nil {
		return err
	}

	var out *os.File
	if outfile == "" {
		var dir string

		dir, err = ioutil.TempDir("", "cover")
		if err != nil {
			return err
		}

		out, err = os.Create(filepath.Join(dir, "coverage.html"))
		if err != nil {
			return err
		}
	} else {
		out, err = os.Create(outfile)
		if err != nil {
			return err
		}
	}

	err = getTemplate(out, &d)
	if err == nil {
		err = out.Close()
	}

	if err != nil {
		return err
	}

	if outfile == "" {
		if !startBrowser("file://" + out.Name()) {
			fmt.Fprintf(os.Stderr, "HTML output written to %s\n", out.Name())
		}
	}

	return nil
}

// startBrowser tries to open the URL in a browser
// and reports whether it succeeds.
func startBrowser(url string) bool {
	// try to start the browser
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"open"}
	case "windows":
		args = []string{"cmd", "/c", "start"}
	default:
		args = []string{"xdg-open"}
	}
	cmd := exec.Command(args[0], append(args[1:], url)...)
	return cmd.Start() == nil
}
