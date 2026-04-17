package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
)

func renderFixResult(w io.Writer, result *app.FixResult) {
	p := presentationForWriter(w)
	suffix := ""
	if strings.TrimSpace(result.Selector) != "" {
		suffix = " " + p.dim(result.Selector)
	}
	fmt.Fprintln(w, p.headerLine("fix", suffix))
	fmt.Fprintln(w)

	if len(result.Files) == 0 {
		fmt.Fprintf(w, "  %s no deterministic doc-drift edits available\n", p.info())
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
		return
	}

	for i, file := range result.Files {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderFixPromptFile(w, result.Selector, file)
		if file.Status == "applied" {
			fmt.Fprintf(w, "    %s applied %d edit%s\n", p.check(), len(file.Edits), pluralSuffix(len(file.Edits)))
		}
		if file.Reason != "" {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), file.Reason)
		}
		for _, warning := range file.Warnings {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), warning)
		}
	}
	if len(result.Guidance) > 0 {
		fmt.Fprintln(w)
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
	}
}

func renderCompileResult(w io.Writer, result *app.CompileResult) {
	p := presentationForWriter(w)
	suffix := ""
	if strings.TrimSpace(result.Scope) != "" {
		suffix = " " + p.dim(result.Scope)
	}
	fmt.Fprintln(w, p.headerLine("compile", suffix))
	fmt.Fprintln(w)

	if len(result.Files) == 0 {
		fmt.Fprintf(w, "  %s no actionable terminology edits found\n", p.info())
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
		return
	}

	for i, file := range result.Files {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "  %s\n\n", p.dim(file.Path))
		if len(file.Edits) == 0 {
			fmt.Fprintf(w, "    %s %s\n", p.info(), "no unambiguous terminology edits available")
			continue
		}
		for _, edit := range file.Edits {
			fmt.Fprintf(w, "    %s %s\n", p.red("-"), edit.Before)
			fmt.Fprintf(w, "    %s %s\n", p.green("+"), edit.After)
			fmt.Fprintln(w)
		}
		if file.Status == "applied" {
			fmt.Fprintf(w, "    %s applied %d edit%s\n", p.check(), len(file.Edits), pluralSuffix(len(file.Edits)))
		}
		if file.Reason != "" {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), file.Reason)
		}
		for _, warning := range file.Warnings {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), warning)
		}
	}
	if len(result.Guidance) > 0 {
		fmt.Fprintln(w)
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
	}
}

func renderFixPromptFile(w io.Writer, selector string, file app.FixFileResult) {
	p := presentationForWriter(w)
	fmt.Fprintf(w, "  %s\n\n", p.dim(file.Path))
	if len(file.Edits) == 0 {
		fmt.Fprintf(w, "    %s %s\n", p.info(), "no deterministic replace-claim edits available")
		return
	}
	for _, edit := range file.Edits {
		fmt.Fprintf(w, "    %s %s\n", p.red("-"), edit.Before)
		fmt.Fprintf(w, "    %s %s\n", p.green("+"), edit.After)
		if edit.Summary != "" {
			fmt.Fprintf(w, "      %s\n", p.dim(edit.Summary))
		}
		fmt.Fprintln(w)
	}
}
