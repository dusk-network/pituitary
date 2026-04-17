package cmd

import (
	"io"
	"os"
	"strings"
)

type cliPresentationWriter interface {
	io.Writer
	cliColorMode() string
	cliUnderlyingWriter() io.Writer
}

type wrappedCLIWriter struct {
	io.Writer
	colorMode string
}

func wrapCLIWriter(w io.Writer, colorMode string) io.Writer {
	if w == nil {
		return nil
	}
	return wrappedCLIWriter{Writer: w, colorMode: colorMode}
}

func (w wrappedCLIWriter) cliColorMode() string {
	return w.colorMode
}

func (w wrappedCLIWriter) cliUnderlyingWriter() io.Writer {
	return w.Writer
}

type renderPresentation struct {
	color bool
	utf8  bool
}

func presentationForWriter(w io.Writer) renderPresentation {
	mode := colorModeAuto
	target := w
	if wrapped, ok := w.(cliPresentationWriter); ok {
		mode = wrapped.cliColorMode()
		target = wrapped.cliUnderlyingWriter()
	}
	return renderPresentation{
		color: shouldColorize(target, mode),
		utf8:  supportsUTF8(),
	}
}

func shouldColorize(w io.Writer, mode string) bool {
	switch mode {
	case colorModeAlways:
		return true
	case colorModeNever:
		return false
	}

	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func supportsUTF8() bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := strings.ToUpper(strings.TrimSpace(os.Getenv(name)))
		if value == "" {
			continue
		}
		return strings.Contains(value, "UTF-8") || strings.Contains(value, "UTF8")
	}
	return true
}

func (p renderPresentation) apply(code, value string) string {
	if !p.color || value == "" {
		return value
	}
	return "\033[" + code + "m" + value + "\033[0m"
}

func (p renderPresentation) bold(value string) string   { return p.apply("1", value) }
func (p renderPresentation) dim(value string) string    { return p.apply("2", value) }
func (p renderPresentation) red(value string) string    { return p.apply("1;31", value) }
func (p renderPresentation) green(value string) string  { return p.apply("1;32", value) }
func (p renderPresentation) yellow(value string) string { return p.apply("1;33", value) }
func (p renderPresentation) cyan(value string) string   { return p.apply("0;36", value) }
func (p renderPresentation) white(value string) string  { return p.apply("1;37", value) }
func (p renderPresentation) headerMark() string {
	if p.utf8 {
		return p.bold("━━◈")
	}
	return p.bold("---*")
}
func (p renderPresentation) cross() string {
	if p.utf8 {
		return p.red("✗")
	}
	return p.red("x")
}
func (p renderPresentation) check() string {
	if p.utf8 {
		return p.green("✓")
	}
	return p.green("+")
}
func (p renderPresentation) arrow() string {
	if p.utf8 {
		return p.yellow("→")
	}
	return p.yellow("->")
}
func (p renderPresentation) info() string {
	if p.utf8 {
		return p.dim("ℹ")
	}
	return p.dim("i")
}
func (p renderPresentation) treeMid() string {
	if p.utf8 {
		return p.dim("├─")
	}
	return p.dim("|-")
}
func (p renderPresentation) treeLast() string {
	if p.utf8 {
		return p.dim("└─")
	}
	return p.dim("\\-")
}
func (p renderPresentation) treeBranch(last bool) string {
	if last {
		return p.treeLast()
	}
	return p.treeMid()
}
func (p renderPresentation) treeItem(last bool) string {
	return p.treeBranch(last)
}
func (p renderPresentation) blockHigh() string {
	if p.utf8 {
		return p.red("██")
	}
	return p.red("[!]")
}
func (p renderPresentation) blockMedium() string {
	if p.utf8 {
		return p.yellow("▒▒")
	}
	return p.yellow("[~]")
}
func (p renderPresentation) okBadge() string {
	if p.utf8 {
		return p.green("██ OK")
	}
	return p.green("[OK]")
}
func (p renderPresentation) driftBadge() string {
	if p.utf8 {
		return p.red("██ DRIFT")
	}
	return p.red("[DRIFT]")
}
func (p renderPresentation) reviewBadge() string {
	if p.utf8 {
		return p.yellow("▒▒ REVIEW")
	}
	return p.yellow("[REVIEW]")
}
func (p renderPresentation) conflictBadge() string {
	if p.utf8 {
		return p.red("██ CONFLICT")
	}
	return p.red("[CONFLICT]")
}
func (p renderPresentation) compliantBadge() string {
	if p.utf8 {
		return p.green("██ OK")
	}
	return p.green("[OK]")
}
func (p renderPresentation) unspecifiedBadge() string {
	if p.utf8 {
		return p.yellow("▒▒ UNSPEC")
	}
	return p.yellow("[UNSPEC]")
}
func (p renderPresentation) headerLine(command, suffix string) string {
	line := p.headerMark() + " " + p.white(command)
	if suffix != "" {
		line += suffix
	}
	return line
}
