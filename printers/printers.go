// Package printers has methods for different output formats
package printers

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sanchezhs/py-broom/colors"
	"github.com/sanchezhs/py-broom/finder"
)

type Printer interface {
	Print(w io.Writer, methodUsage []finder.MethodUsage) error
}

//================================================================================
// Console
//================================================================================

type ConsolePrinter struct {
	NoColor bool
}

func (p ConsolePrinter) Print(w io.Writer, results []finder.MethodUsage) error {
	for _, result := range results {
		fmt.Fprintf(w, "%s %s\n",
			colors.Colorize("Method:", colors.ColorYellow+colors.ColorBold, p.NoColor),
			colors.Colorize(result.Method.Name, colors.ColorGreen+colors.ColorBold, p.NoColor))

		fmt.Fprintf(w, "%s %s:%d\n",
			colors.Colorize("Defined in:", colors.ColorYellow, p.NoColor),
			result.Method.Filename,
			result.Method.LineNo)

		usageColor := colors.ColorGreen
		if result.UsageCount == 0 {
			usageColor = colors.ColorRed
		}
		fmt.Fprintf(w, "%s %s\n",
			colors.Colorize("Usages found:", colors.ColorYellow, p.NoColor),
			colors.Colorize(fmt.Sprintf("%d", result.UsageCount), usageColor+colors.ColorBold, p.NoColor))

		if len(result.Usages) > 0 {
			fmt.Fprintln(w, colors.Colorize("Locations:", colors.ColorYellow, p.NoColor))
			for _, u := range result.Usages {
				fmt.Fprintf(w, "  %s %s\n",
					colors.Colorize("-", colors.ColorBlue, p.NoColor),
					u)
			}
		} else {
			fmt.Fprintf(w, "  %s\n", colors.Colorize("(No usages found)", colors.ColorRed, p.NoColor))
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, colors.Colorize(strings.Repeat("-", 80), colors.ColorPurple, p.NoColor))
		fmt.Fprintln(w)
	}
	return nil
}

//================================================================================
// Json
//================================================================================

type JSONPrinter struct {
	Indent bool
}

func (p JSONPrinter) Print(w io.Writer, results []finder.MethodUsage) error {
	enc := json.NewEncoder(w)
	if p.Indent {
		enc.SetIndent("", "  ")
	}
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

//================================================================================
// Vim grep
//================================================================================

type VimPrinter struct{}

func (VimPrinter) Print(w io.Writer, results []finder.MethodUsage) error {
	for _, r := range results {
		for _, u := range r.Usages {
			// u is expected to be: "<file>:<line>:<col>:<context>"
			// We just pass it through as-is.
			if _, err := fmt.Fprintln(w, u); err != nil {
				return err
			}
		}
	}
	return nil
}

//================================================================================
// Factory
//================================================================================

type Kind string

const (
	KindConsole Kind = "console"
	KindJSON    Kind = "json"
	KindVimGrep Kind = "vimgrep"
)

var OutputKinds = map[string]Kind{
	"console": KindConsole,
	"json":    KindJSON,
	"vim":     KindVimGrep,
}

type Options struct {
	NoColor bool
	Indent  bool
}

func GetKinds() string {
	var keys []string
	for k := range OutputKinds {
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

func New(kind Kind, opts Options) Printer {
	switch kind {
	case KindJSON:
		return JSONPrinter{Indent: opts.Indent}
	case KindVimGrep:
		return VimPrinter{}
	case KindConsole:
		fallthrough
	default:
		return ConsolePrinter{NoColor: opts.NoColor}
	}
}
