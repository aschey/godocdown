/*
Command godocdown generates Go documentation in a GitHub-friendly Markdown format.

	$ go get github.com/aschey/godocdown/godocdown                         
	                                                                             
	$ godocdown /path/to/package > README.markdown                               
	                                                                             
	# Generate documentation for the package/command in the current directory    
	$ godocdown > README.markdown                                                
	                                                                             
	# Generate standard Markdown                                                 
	$ godocdown -plain .                                                         

This program is targeted at providing nice-looking documentation for GitHub. With this in
mind, it generates GitHub Flavored Markdown (http://github.github.com/github-flavored-markdown/) by
default. This can be changed with the use of the "plain" flag to generate standard Markdown.

Install

	go get github.com/aschey/godocdown/godocdown

# Example

http://github.com/aschey/godocdown/blob/master/example.markdown

Usage

	-output=""                                                                       
	    Write output to a file instead of stdout                                     
	    Write to stdout with -                                                       
	                                                                                 
	-template=""                                                                     
	    The template file to use                                                     
	                                                                                 
	-no-template=false                                                               
	    Disable template processing                                                  
	                                                                                 
	-plain=false                                                                     
	    Emit standard Markdown, rather than Github Flavored Markdown                 
	                                                                                 
	-heading="TitleCase1Word"                                                        
	    Heading detection method: 1Word, TitleCase, Title, TitleCase1Word, ""        
	    For each line of the package declaration, godocdown attempts to detect if    
	    a heading is present via a pattern match. If a heading is detected,          
	    it prefixes the line with a Markdown heading indicator (typically "###").    
	                                                                                 
	    1Word: Only a single word on the entire line                                 
	        [A-Za-z0-9_-]+                                                           
	                                                                                 
	    TitleCase: A line where each word has the first letter capitalized           
	        ([A-Z][A-Za-z0-9_-]\s*)+                                                 
	                                                                                 
	    Title: A line without punctuation (e.g. a period at the end)                 
	        ([A-Za-z0-9_-]\s*)+                                                      
	                                                                                 
	    TitleCase1Word: The line matches either the TitleCase or 1Word pattern       

# Templating

In addition to Markdown rendering, godocdown provides templating via text/template (http://golang.org/pkg/text/template/)
for further customization. By putting a file named ".godocdown.template" (or one from the list below) in the same directory as your
package/command, godocdown will know to use the file as a template.

	# text/template
	.godocdown.markdown
	.godocdown.md
	.godocdown.template
	.godocdown.tmpl

A template file can also be specified with the "-template" parameter

Along with the standard template functionality, the starting data argument has the following interface:

	{{ .Emit }}                                                                                       
	// Emit the standard documentation (what godocdown would emit without a template)                 
	                                                                                                  
	{{ .EmitHeader }}                                                                                 
	// Emit the package name and an import line (if one is present/needed)                            
	                                                                                                  
	{{ .EmitSynopsis }}                                                                               
	// Emit the package declaration                                                                   
	                                                                                                  
	{{ .EmitUsage }}                                                                                  
	// Emit package usage, which includes a constants section, a variables section,                   
	// a functions section, and a types section. In addition, each type may have its own constant,    
	// variable, and/or function/method listing.                                                      
	                                                                                                  
	{{ if .IsCommand  }} ... {{ end }}                                                                
	// A boolean indicating whether the given package is a command or a plain package                 
	                                                                                                  
	{{ .Name }}                                                                                       
	// The name of the package/command (string)                                                       
	                                                                                                  
	{{ .ImportPath }}                                                                                 
	// The import path for the package (string)                                                       
	// (This field will be the empty string if godocdown is unable to guess it)                       
*/
package main

import (
	"bytes"
	Flag "flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	Template "text/template"
	Time "time"

	"github.com/lithammer/dedent"
	"golang.org/x/mod/modfile"
)

const (
	punchCardWidth = 80
	debug          = false
)

var (
	flag            = Flag.NewFlagSet("", Flag.ExitOnError)
	flag_signature  = flag.Bool("signature", false, string(0))
	flag_plain      = flag.Bool("plain", false, "Emit standard Markdown, rather than Github Flavored Markdown (the default)")
	flag_heading    = flag.String("heading", "TitleCase1Word", "Heading detection method: 1Word, TitleCase, Title, TitleCase1Word, \"\"")
	flag_template   = flag.String("template", "", "The template file to use")
	flag_noTemplate = flag.Bool("no-template", false, "Disable template processing")
	flag_noFuncs    = flag.Bool("no-funcs", false, "Ignore Funcs")
	flag_output     = ""
	_               = func() byte {
		flag.StringVar(&flag_output, "output", flag_output, "Write output to a file instead of stdout. Write to stdout with -")
		flag.StringVar(&flag_output, "o", flag_output, string(0))
		return 0
	}()
)

var (
	fset *token.FileSet

	synopsisHeading1Word_Regexp          = regexp.MustCompile("(?m)^([A-Za-z0-9_-]+)$")
	synopsisHeadingTitleCase_Regexp      = regexp.MustCompile("(?m)^((?:[A-Z][A-Za-z0-9_-]*)(?:[ \t]+[A-Z][A-Za-z0-9_-]*)*)$")
	synopsisHeadingTitle_Regexp          = regexp.MustCompile("(?m)^((?:[A-Za-z0-9_-]+)(?:[ \t]+[A-Za-z0-9_-]+)*)$")
	synopsisHeadingTitleCase1Word_Regexp = regexp.MustCompile("(?m)^((?:[A-Za-z0-9_-]+)|(?:(?:[A-Z][A-Za-z0-9_-]*)(?:[ \t]+[A-Z][A-Za-z0-9_-]*)*))$")

	strip_Regexp           = regexp.MustCompile("(?m)^\\s*// contains filtered or unexported fields\\s*\n")
	indent_Regexp          = regexp.MustCompile("(?m)^([^\\n])") // Match at least one character at the start of the line
	synopsisHeading_Regexp = synopsisHeading1Word_Regexp
	match_7f               = regexp.MustCompile(`(?m)[\t ]*\x7f[\t ]*$`)
)

var DefaultStyle = Style{
	IncludeImport: true,

	SynopsisHeader:  "####",
	SynopsisHeading: synopsisHeadingTitleCase1Word_Regexp,

	UsageHeader: "#### Index\n",

	ConstantHeader:     "####",
	VariableHeader:     "####",
	FunctionHeader:     "####",
	TypeHeader:         "####",
	TypeFunctionHeader: "####",

	IncludeSignature: false,
}
var RenderStyle = DefaultStyle

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	executable, err := os.Stat(os.Args[0])
	if err != nil {
		return
	}
	time := executable.ModTime()
	since := Time.Since(time)
	fmt.Fprintf(os.Stderr, "---\n%s (%.2f)\n", time.Format("2006-01-02 15:04 MST"), since.Minutes())
}

func init() {
	flag.Usage = usage
}

type Style struct {
	IncludeImport bool

	SynopsisHeader  string
	SynopsisHeading *regexp.Regexp

	UsageHeader string

	ConstantHeader     string
	VariableHeader     string
	FunctionHeader     string
	TypeHeader         string
	TypeFunctionHeader string

	IncludeSignature bool
}

type _document struct {
	Name       string
	pkg        *doc.Package
	absPath    string
	testFiles  map[string]*ast.File
	IsCommand  bool
	ImportPath string
	Examples   examples
}

func takeOut7f(input string) string {
	return match_7f.ReplaceAllString(input, "")
}

// func _formatIndent(target, indent, preIndent string) string {
// 	var buffer bytes.Buffer
// 	toText(&buffer, target, indent, preIndent, punchCardWidth-2*len(indent))
// 	s := buffer.String()
// 	return dedent.Dedent(s)
// }
//
//
// func formatIndent(target string) string {
// 	return _formatIndent(target, spacer(0), spacer(0))
// }

// filterExamples filters the list of examples to only includes the ones that
// are associated with the provided type/func name
func filterExamples(exs []*doc.Example, name string) (res []*doc.Example) {
	for _, e := range exs {
		root := strings.SplitN(e.Name, "_", 2)[0]
		if root == name {
			res = append(res, e)
		}
	}
	return
}

func spacer(width int) string {
	return strings.Repeat(" ", width)
}

func indentCode(target string) string {
	if *flag_plain {
		return indent(target+"\n", spacer(4))
	}
	if target[0] == '{' && target[len(target)-1] == '}' {
		// example code night be wrapped in a weird enclosing brackets. clean it
		// up.
		target = target[1 : len(target)-1]
	}
	target = dedent.Dedent(target)
	target = strings.Trim(target, "\n")
	return fmt.Sprintf("```go\n%s\n```", target)
}

func headifySynopsis(target string) string {
	detect := RenderStyle.SynopsisHeading
	if detect == nil {
		return target
	}
	return detect.ReplaceAllStringFunc(target, func(heading string) string {
		return fmt.Sprintf("%s %s", RenderStyle.SynopsisHeader, heading)
	})
}

func exampleNames(name string) (base, sub string) {
	comps := strings.SplitN(name, "_", 2)
	base = comps[0]
	if len(comps) > 1 {
		sub = " (" + strings.Replace(comps[1], "_", " ", -1) + ")"
	}
	return
}

func headlineSynopsis(synopsis, header string, scanner *regexp.Regexp) string {
	return scanner.ReplaceAllStringFunc(synopsis, func(headline string) string {
		return fmt.Sprintf("%s %s", header, headline)
	})
}

func sourceOfNode(target interface{}) string {
	var buffer bytes.Buffer
	mode := printer.TabIndent | printer.UseSpaces
	err := (&printer.Config{Mode: mode, Tabwidth: 4}).Fprint(&buffer, fset, target)
	if err != nil {
		return ""
	}
	return strip_Regexp.ReplaceAllString(buffer.String(), "")
}

func indentNode(target interface{}) string {
	return indentCode(sourceOfNode(target))
}

func indent(target string, indent string) string {
	return indent_Regexp.ReplaceAllString(target, indent+"$1")
}

func filterText(input string) string {
	// Why is this here?
	// Normally, godoc will ALWAYS collapse adjacent lines separated only by whitespace.
	// However, if you place a (normally invisible) \x7f character in the documentation,
	// this collapse will not happen. Thankfully, Markdown does not need this sort of hack,
	// so we remove it.
	return takeOut7f(input)
}

func trimSpace(buffer *bytes.Buffer) {
	tmp := bytes.TrimSpace(buffer.Bytes())
	buffer.Reset()
	buffer.Write(tmp)
}

func fromSlash(path string) string {
	return filepath.FromSlash(path)
}

func exampleSubName(name string) string {
	_, sub := exampleNames(name)
	return sub
}

/*
	    This is how godoc does it:

		// Determine paths.
		//
		// If we are passed an operating system path like . or ./foo or /foo/bar or c:\mysrc,
		// we need to map that path somewhere in the fs name space so that routines
		// like getPageInfo will see it.  We use the arbitrarily-chosen virtual path "/target"
		// for this.  That is, if we get passed a directory like the above, we map that
		// directory so that getPageInfo sees it as /target.
		const target = "/target"
		const cmdPrefix = "cmd/"
		path := flag.Arg(0)
		var forceCmd bool
		var abspath, relpath string
		if filepath.IsAbs(path) {
			fs.Bind(target, OS(path), "/", bindReplace)
			abspath = target
		} else if build.IsLocalImport(path) {
			cwd, _ := os.Getwd() // ignore errors
			path = filepath.Join(cwd, path)
			fs.Bind(target, OS(path), "/", bindReplace)
			abspath = target
		} else if strings.HasPrefix(path, cmdPrefix) {
			path = path[len(cmdPrefix):]
			forceCmd = true
		} else if bp, _ := build.Import(path, "", build.FindOnly); bp.Dir != "" && bp.ImportPath != "" {
			fs.Bind(target, OS(bp.Dir), "/", bindReplace)
			abspath = target
			relpath = bp.ImportPath
		} else {
			abspath = pathpkg.Join(pkgHandler.fsRoot, path)
		}
		if relpath == "" {
			relpath = abspath
		}
*/
func buildImport(target string) (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	relPath := target
	absPath := target
	if filepath.IsAbs(target) {
		relPath, err = filepath.Rel(cwd, target) // filepath.Join(cwd, target)
		if err != nil {
			return "", "", err
		}
	} else {
		absPath = filepath.Join(cwd, target)
	}

	modPath := filepath.Join(cwd, "go.mod")
	modContents, err := os.ReadFile(modPath)
	if err != nil {
		return "", "", err
	}
	modFile, err := modfile.Parse("go.mod", modContents, nil)
	if err != nil {
		return "", "", err
	}
	modName := modFile.Module.Mod.Path
	// Ensure we use forward slashes on windows
	importPath := strings.ReplaceAll(filepath.Join(modName, relPath), "\\", "/")

	return importPath, absPath, err

}

func loadDocument(target string) (*_document, error) {

	importPath, absPath, err := buildImport(target)
	if err != nil {
		return nil, err
	}

	fset = token.NewFileSet()
	pkgSet, err := parser.ParseDir(fset, absPath, func(file os.FileInfo) bool {
		name := file.Name()
		if name[0] != '.' && strings.HasSuffix(name, ".go") { //} && !strings.HasSuffix(name, "_test.go") {
			return true
		}
		return false
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("Could not parse \"%s\": %v", absPath, err)
	}

	if read, err := ioutil.ReadFile(filepath.Join(absPath, ".godocdown.import")); err == nil {
		importPath = strings.TrimSpace(strings.Split(string(read), "\n")[0])
	}

	{
		isCommand := false
		name := ""
		var pkg *doc.Package
		var testFiles map[string]*ast.File

		// Choose the best package for documentation. Either
		// documentation, main, or whatever the package is.
		for _, parsePkg := range pkgSet {
			// we don't want to document the test files, but we do need to keep
			// them around to extract the examples from them.
			astFiles := make(map[string]*ast.File)
			for k, f := range parsePkg.Files {
				if strings.HasSuffix(k, "_test.go") {
					astFiles[k] = f
					delete(parsePkg.Files, k)
				}
			}

			tmpPkg := doc.New(parsePkg, ".", 0)
			switch tmpPkg.Name {
			case "main":
				if isCommand || name != "" {
					// We've already seen "package documentation"
					// (or something else), so favor that over main.
					continue
				}
				fallthrough
			case "documentation":
				// We're a command, this package/file contains the documentation
				// path is used to get the containing directory in the case of
				// command documentation

				_, name = filepath.Split(absPath)
				isCommand = true
				pkg = tmpPkg
			default:
				// Just a regular package
				name = tmpPkg.Name
				pkg = tmpPkg
				testFiles = astFiles
			}
		}

		if pkg != nil {
			var exs examples
			for _, f := range testFiles {
				for _, e := range doc.Examples(f) {
					exs = append(exs, e)
				}
			}

			sort.Sort(exs)
			return &_document{
				Name:       name,
				pkg:        pkg,
				absPath:    absPath,
				testFiles:  testFiles,
				IsCommand:  isCommand,
				ImportPath: importPath,
				Examples:   exs,
			}, nil
		}
	}

	return nil, nil
}

func emitString(fn func(*bytes.Buffer)) string {
	var buffer bytes.Buffer
	fn(&buffer)
	trimSpace(&buffer)
	return buffer.String()
}

// Emit
func (self *_document) Emit() string {
	return emitString(func(buffer *bytes.Buffer) {
		self.EmitTo(buffer)
	})
}

func (self *_document) EmitTo(buffer *bytes.Buffer) {

	// Header
	self.EmitHeaderTo(buffer)

	// Synopsis
	self.EmitSynopsisTo(buffer)

	// Usage
	if !self.IsCommand {
		self.EmitUsageTo(buffer)
	}

	trimSpace(buffer)
}

// Signature
func (self *_document) EmitSignature() string {
	return emitString(func(buffer *bytes.Buffer) {
		self.EmitSignatureTo(buffer)
	})
}

func (self *_document) EmitSignatureTo(buffer *bytes.Buffer) {

	renderSignatureTo(buffer)

	trimSpace(buffer)
}

// Header
func (self *_document) EmitHeader() string {
	return emitString(func(buffer *bytes.Buffer) {
		self.EmitHeaderTo(buffer)
	})
}

func (self *_document) EmitHeaderTo(buffer *bytes.Buffer) {
	renderHeaderTo(buffer, self)
}

// Synopsis
func (self *_document) EmitSynopsis() string {
	return emitString(func(buffer *bytes.Buffer) {
		self.EmitSynopsisTo(buffer)
	})
}

func (self *_document) EmitSynopsisTo(buffer *bytes.Buffer) {
	renderSynopsisTo(buffer, self)
}

// Usage
func (self *_document) EmitUsage() string {
	return emitString(func(buffer *bytes.Buffer) {
		self.EmitUsageTo(buffer)
	})
}

func (self *_document) EmitUsageTo(buffer *bytes.Buffer) {
	renderUsageTo(buffer, self)
}

var templateNameList = strings.Fields(`
	.godocdown.markdown
	.godocdown.md
	.godocdown.template
	.godocdown.tmpl
`)

func findTemplate(path string) string {

	for _, templateName := range templateNameList {
		templatePath := filepath.Join(path, templateName)
		_, err := os.Stat(templatePath)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			continue // Other error reporting?
		}
		return templatePath
	}
	return "" // Nothing found
}

func loadTemplate(document *_document) *Template.Template {
	if *flag_noTemplate {
		return nil
	}

	templatePath := *flag_template
	if templatePath == "" {
		templatePath = findTemplate(document.absPath)
	}

	if templatePath == "" {
		return nil
	}

	template := Template.New("").Funcs(Template.FuncMap{})
	template, err := template.ParseFiles(templatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing template \"%s\": %v", templatePath, err)
		os.Exit(1)
	}
	return template
}

func (self *_document) Badge() string {
	return "[![GoDocDown](https://img.shields.io/badge/docs-generated-blue.svg?longCache=true)](https://github.com/aschey/godocdown)"
}

func (*_document) ToCode(code string) string {
	return indentCode(code)
}

func (self *_document) Synopsis() string {
	return headifySynopsis(filterText(self.pkg.Doc))
}

func (self *_document) Import() string {
	return fmt.Sprintf(`import "%s"`, self.ImportPath)
}

func (self *_document) Funcs() []*doc.Func {
	return self.pkg.Funcs
}

func (self *_document) Types() []*doc.Type {
	return self.pkg.Types
}

func (self *_document) Consts() []*doc.Value {
	return self.pkg.Consts
}

func (self *_document) Vars() []*doc.Value {
	return self.pkg.Vars
}

type examples []*doc.Example

func (exs examples) Len() int           { return len(exs) }
func (exs examples) Less(i, j int) bool { return exs[i].Name < exs[j].Name }
func (exs examples) Swap(i, j int)      { exs[i], exs[j] = exs[j], exs[i] }

func main() {
	flag.Parse(os.Args[1:])
	target := flag.Arg(0)
	fallbackUsage := false
	if target == "" {
		fallbackUsage = true
		target = "."
	}

	RenderStyle.IncludeSignature = *flag_signature

	switch *flag_heading {
	case "1Word":
		RenderStyle.SynopsisHeading = synopsisHeading1Word_Regexp
	case "TitleCase":
		RenderStyle.SynopsisHeading = synopsisHeadingTitleCase_Regexp
	case "Title":
		RenderStyle.SynopsisHeading = synopsisHeadingTitle_Regexp
	case "TitleCase1Word":
		RenderStyle.SynopsisHeading = synopsisHeadingTitleCase1Word_Regexp
	case "", "-":
		RenderStyle.SynopsisHeading = nil
	}

	document, err := loadDocument(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
	if document == nil {
		// Nothing found.
		if fallbackUsage {
			usage()
			os.Exit(2)
		} else {
			fmt.Fprintf(os.Stderr, "Could not find package: %s\n", target)
			os.Exit(1)
		}
	}

	if *flag_noFuncs {
		document.pkg.Funcs = nil
		for i := range document.pkg.Types {
			document.pkg.Types[i].Funcs = nil
			document.pkg.Types[i].Methods = nil
		}
	}

	tpl := loadTemplate(document)
	var buffer bytes.Buffer
	if tpl == nil {
		document.EmitTo(&buffer)
		document.EmitSignatureTo(&buffer)

		// tpl, err = Template.New("").Funcs(Template.FuncMap{
		// 	"indentCode": indentCode,
		// 	"sourceOfNode": sourceOfNode,
		// 	"indentNode": indentNode,
		// 	"filterText": filterText,
		// 	"exampleSubName": exampleSubName,
		// 	"filterExamples": filterExamples,
		// }).Parse(tplTxt)
		//
		// if err != nil {
		// 	panic(err)
		// }
		//
		// err = tpl.Execute(&buffer, document)
		// if err != nil {
		// 	panic(err)
		// }
	} else {
		err := tpl.Templates()[0].Execute(&buffer, document)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running template: %v", err)
			os.Exit(1)
		}
		document.EmitSignatureTo(&buffer)
	}

	if debug {
		// Skip printing if we're debugging
		return
	}

	documentation := buffer.String()
	documentation = strings.TrimSpace(documentation)
	if flag_output == "" || flag_output == "-" {
		fmt.Println(documentation)
	} else {
		file, err := os.Create(flag_output)
		if err != nil {
		}
		defer file.Close()
		_, err = fmt.Fprintln(file, documentation)
	}
}
