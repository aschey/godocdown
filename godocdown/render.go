package main

import (
	"fmt"
	"go/doc"
	"io"
	"strings"
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
		fmt.Fprintf(writer, "%s func %s%s\n\n%s\n%s\n", header, receiver, entry.Name, indentCode(sourceOfNode(entry.Decl)), formatIndent(filterText(entry.Doc)))

		if examples != nil {
			for _, ex := range examples[entry.Name] {
				renderExample(writer, ex)
			}
		}
	}
}

func renderExample(writer io.Writer, ex *doc.Example) {
	code := sourceOfNode(ex.Code)
	code = strings.Trim(code, "{} \n")
	code = indentCode(code)
	fmt.Fprintf(writer, "<details><summary>Example</summary><p>\n\n%s\n%s\n\nOutput:\n```\n%s```\n</p></details>\n\n", formatIndent(filterText(ex.Doc)), code, ex.Output)
}

func renderTypeSectionTo(writer io.Writer, list []*doc.Type) {

	header := RenderStyle.TypeHeader

	for _, entry := range list {
		fmt.Fprintf(writer, "%s type %s\n\n%s\n\n%s\n", header, entry.Name, indentCode(sourceOfNode(entry.Decl)), formatIndent(filterText(entry.Doc)))
		renderConstantSectionTo(writer, entry.Consts)
		renderVariableSectionTo(writer, entry.Vars)
		renderFunctionSectionTo(writer, entry.Funcs, true, nil)
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

	// Constant Section
	renderConstantSectionTo(writer, document.pkg.Consts)

	// Variable Section
	renderVariableSectionTo(writer, document.pkg.Vars)

	// Function Section
	renderFunctionSectionTo(writer, document.pkg.Funcs, false, examples)

	// Type Section
	renderTypeSectionTo(writer, document.pkg.Types)
}

func renderSignatureTo(writer io.Writer) {
	if RenderStyle.IncludeSignature {
		fmt.Fprintf(writer, "\n\n--\n**godocdown** http://github.com/avinoamr/godocdown\n")
	}
}
