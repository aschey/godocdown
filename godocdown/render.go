package main

import (
	"fmt"
	"go/doc"
	"io"
	"strings"
	"github.com/lithammer/dedent"
)

func renderConstantSectionTo(writer io.Writer, list []*doc.Value) {
	for _, entry := range list {
		fmt.Fprintf(writer, "%s\n%s\n", indentCode(sourceOfNode(entry.Decl)), formatIndent(filterText(entry.Doc)))
	}
}

func renderVariableSectionTo(writer io.Writer, list []*doc.Value) {
	for _, entry := range list {
		fmt.Fprintf(writer, "%s\n%s\n", indentCode(sourceOfNode(entry.Decl)), formatIndent(filterText(entry.Doc)))
	}
}

func renderFunctionSectionTo(writer io.Writer, list []*doc.Func, inTypeSection bool, examples map[string][]*doc.Example) {

	header := RenderStyle.FunctionHeader
	if inTypeSection {
		header = RenderStyle.TypeFunctionHeader
	}

	for _, entry := range list {
		receiver := " "
		if entry.Recv != "" {
			receiver = fmt.Sprintf("(%s) ", entry.Recv)
		}
		decl := indentCode(sourceOfNode(entry.Decl))
		comment := formatIndent(filterText(entry.Doc))
		fmt.Fprintf(writer, "%s <a name='%s'></a> func %s[%s]()\n\n%s\n%s\n",
			header,
			entry.Name,
			receiver,
			entry.Name,
			decl,
			comment)

		if examples != nil {
			for _, ex := range examples[entry.Name] {
				renderExample(writer, ex)
			}
		}
	}
}



func renderExample(w io.Writer, ex *doc.Example) {
	code := sourceOfNode(ex.Code)
	if code[0] == '{' {
		// example code night be wrapped in a weird enclosing brackets. clean it
		// up.
		code = code[3:len(code) - 3]
	}
	code = dedent.Dedent(code)
	code = indentCode(code)


	fmt.Fprintf(w, "<a name='Example%s'></a><details><summary>Example</summary><p>\n\n%s\n%s\n\nOutput:\n```\n%s```\n</p></details>\n\n",
		ex.Name,
		formatIndent(filterText(ex.Doc)),
		code,
		ex.Output)
}

func renderTypeSectionTo(writer io.Writer, list []*doc.Type, examples map[string][]*doc.Example) {
	header := RenderStyle.TypeHeader

	for _, entry := range list {
		fmt.Fprintf(writer, "%s <a name='%s'></a>type [%s]()\n\n%s\n\n%s\n",
			header,
			entry.Name,
			entry.Name,
			indentCode(sourceOfNode(entry.Decl)),
			formatIndent(filterText(entry.Doc)))

		for _, ex := range examples[entry.Name] {
			renderExample(writer, ex)
		}

		renderConstantSectionTo(writer, entry.Consts)
		renderVariableSectionTo(writer, entry.Vars)
		renderFunctionSectionTo(writer, entry.Funcs, true, examples)
		renderFunctionSectionTo(writer, entry.Methods, true, nil)
	}
}

func renderHeaderTo(writer io.Writer, document *_document) {
	fmt.Fprintf(writer, "# %s\n--\n", document.Name)

	if !document.IsCommand {
		// Import
		if RenderStyle.IncludeImport {
			if document.ImportPath != "" {
				fmt.Fprintf(writer, spacer(4)+"import \"%s\"\n\n", document.ImportPath)
			}
		}
	}
}

func renderSynopsisTo(writer io.Writer, document *_document) {
	fmt.Fprintf(writer, "%s\n", headifySynopsis(formatIndent(filterText(document.pkg.Doc))))
}

func renderUsageTo(writer io.Writer, document *_document) {

	examples := map[string][]*doc.Example{}
	for _, f := range document.testFiles {
		for _, e := range doc.Examples(f) {
			root := strings.SplitN(e.Name, "_", 2)[0]
			examples[root] = append(examples[root], e)
		}
	}

	// Usage
	fmt.Fprintf(writer, "%s\n", RenderStyle.UsageHeader)

	// render index
	renderIndex(writer, document)

	// Constant Section
	renderConstantSectionTo(writer, document.pkg.Consts)

	// Variable Section
	renderVariableSectionTo(writer, document.pkg.Vars)

	// Function Section
	renderFunctionSectionTo(writer, document.pkg.Funcs, false, examples)

	// Type Section
	renderTypeSectionTo(writer, document.pkg.Types, examples)
}

func renderSignatureTo(writer io.Writer) {
	if RenderStyle.IncludeSignature {
		fmt.Fprintf(writer, "\n\n--\n**godocdown** http://github.com/avinoamr/godocdown\n")
	}
}

func renderFunctionIndexTo(w io.Writer, list []*doc.Func, inType bool) {
	prefix := ""
	if inType {
		prefix = "    "
	}

	for _, e := range list {
		decl := sourceOfNode(e.Decl)
		fmt.Fprintf(w, "%s - [%s](#%s)\n", prefix, decl, e.Name)
	}
}

func renderTypeIndexTo(w io.Writer, list []*doc.Type) {
	for _, e := range list {
		fmt.Fprintf(w, " - [type %s](#%s)\n", e.Name, e.Name)
		renderFunctionIndexTo(w, e.Funcs, true)
	}
}

func renderIndex(w io.Writer, d *_document) {
	renderFunctionIndexTo(w, d.pkg.Funcs, false)
	renderTypeIndexTo(w, d.pkg.Types)
	fmt.Fprintf(w, "\n")
}
