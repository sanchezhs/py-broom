// Package printers has methods for different output formats
package printers

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sanchezhs/py-broom/colors"
	"github.com/sanchezhs/py-broom/finder"
)

type Printer interface {
	Print(w io.Writer, methodUsage []finder.MethodUsage) error
}

// ================================================================================
// Console
// ================================================================================

type ConsolePrinter struct {
	NoColor bool
}

func (p ConsolePrinter) Print(w io.Writer, results []finder.MethodUsage) error {
	for _, result := range results {
		if err := p.printMethodUsage(w, result); err != nil {
			return err
		}
	}
	return nil
}

func (p ConsolePrinter) printMethodUsage(w io.Writer, mu finder.MethodUsage) error {
	methodName := colors.Colorize(mu.Method.Name, colors.ColorBold+colors.ColorCyan, p.NoColor)
	fmt.Fprintf(w, "Method: %s\n", methodName)

	location := fmt.Sprintf("%s:%d", mu.Method.Filename, mu.Method.LineNo)
	fmt.Fprintf(w, "Defined in: %s\n", colors.Colorize(location, colors.ColorBlue, p.NoColor))

	usageColor := p.getUsageCountColor(mu.TotalUsages)
	totalUsages := colors.Colorize(fmt.Sprintf("%d", mu.TotalUsages), usageColor, p.NoColor)
	fmt.Fprintf(w, "Total usages: %s\n", totalUsages)

	if mu.TotalUsages == 0 {
		noUsages := colors.Colorize("  (No usages found)", colors.ColorYellow, p.NoColor)
		fmt.Fprintln(w, noUsages)
		fmt.Fprintln(w, colors.Colorize(strings.Repeat("-", 80), colors.ColorReset, p.NoColor))
		return nil
	}

	if len(mu.UsagesByType) > 0 {
		fmt.Fprintln(w, colors.Colorize("Usage breakdown:", colors.ColorBold, p.NoColor))

		for _, callType := range finder.GetCallTypeOrder() {
			if count, exists := mu.UsagesByType[callType]; exists && count > 0 {
				label := finder.GetCallTypeLabel(callType)
				typeColor := p.getCallTypeColor(callType)
				coloredLabel := colors.Colorize(label, typeColor, p.NoColor)
				fmt.Fprintf(w, "  - %s: %s\n", coloredLabel,
					colors.Colorize(fmt.Sprintf("%d", count), typeColor, p.NoColor))
			}
		}
	}

	usagesByType := make(map[finder.CallType][]finder.Usage)
	for _, usage := range mu.Usages {
		usagesByType[usage.CallType] = append(usagesByType[usage.CallType], usage)
	}

	for _, callType := range finder.GetCallTypeOrder() {
		usages, exists := usagesByType[callType]
		if !exists || len(usages) == 0 {
			continue
		}

		typeColor := p.getCallTypeColor(callType)
		header := colors.Colorize(finder.GetCallTypeLabel(callType)+":", colors.ColorBold+typeColor, p.NoColor)
		fmt.Fprintf(w, "\n%s\n", header)

		for _, usage := range usages {
			fmt.Fprintf(w, "  - %s\n", colors.Colorize(usage.Location, colors.ColorWhite, p.NoColor))
			fmt.Fprintf(w, "    %s\n", usage.Context)
		}
	}

	// Separator
	separator := colors.Colorize(strings.Repeat("-", 80), colors.ColorReset, p.NoColor)
	fmt.Fprintln(w, separator)

	return nil
}

func (p ConsolePrinter) getUsageCountColor(count int) string {
	switch {
	case count == 0:
		return colors.ColorYellow
	case count <= 2:
		return colors.ColorRed
	case count <= 5:
		return colors.ColorYellow
	default:
		return colors.ColorGreen
	}
}

func (p ConsolePrinter) getCallTypeColor(ct finder.CallType) string {
	switch ct {
	case finder.CallTypeDefinition:
		return colors.ColorPurple
	case finder.CallTypeInstance:
		return colors.ColorGreen
	case finder.CallTypeClass:
		return colors.ColorCyan
	case finder.CallTypeStatic:
		return colors.ColorBlue
	case finder.CallTypeFunction:
		return colors.ColorYellow
	case finder.CallTypeDecorator:
		return colors.ColorPurple
	default:
		return colors.ColorWhite
	}
}

func (p ConsolePrinter) PrintSummary(w io.Writer, results []finder.MethodUsage) error {
	totalMethods := len(results)

	var unused, lowUsage, mediumUsage, highUsage int
	totalInstanceCalls := 0
	totalClassCalls := 0
	totalStaticCalls := 0
	totalFunctionCalls := 0
	totalDecoratorCalls := 0

	for _, result := range results {
		switch {
		case result.TotalUsages == 0:
			unused++
		case result.TotalUsages <= 2:
			lowUsage++
		case result.TotalUsages <= 5:
			mediumUsage++
		default:
			highUsage++
		}

		totalInstanceCalls += result.UsagesByType[finder.CallTypeInstance]
		totalClassCalls += result.UsagesByType[finder.CallTypeClass]
		totalStaticCalls += result.UsagesByType[finder.CallTypeStatic]
		totalFunctionCalls += result.UsagesByType[finder.CallTypeFunction]
		totalDecoratorCalls += result.UsagesByType[finder.CallTypeDecorator]
	}

	separator := colors.Colorize(strings.Repeat("=", 80), colors.ColorBold, p.NoColor)
	title := colors.Colorize("SUMMARY", colors.ColorBold+colors.ColorCyan, p.NoColor)

	fmt.Fprintln(w, "\n"+separator)
	fmt.Fprintln(w, title)
	fmt.Fprintln(w, separator)

	totalMethodsStr := colors.Colorize(fmt.Sprintf("%d", totalMethods), colors.ColorBold+colors.ColorGreen, p.NoColor)
	fmt.Fprintf(w, "Total methods analyzed: %s\n\n", totalMethodsStr)

	fmt.Fprintln(w, colors.Colorize("Methods by usage count:", colors.ColorBold, p.NoColor))

	// Unused - red
	unusedPct := float64(unused) / float64(totalMethods) * 100
	fmt.Fprintf(w, "  - %s: %s (%s)\n",
		colors.Colorize("Unused (0 usages)", colors.ColorYellow, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", unused), colors.ColorYellow, p.NoColor),
		colors.Colorize(fmt.Sprintf("%.1f%%", unusedPct), colors.ColorYellow, p.NoColor))

	// Low usage - yellow
	lowPct := float64(lowUsage) / float64(totalMethods) * 100
	fmt.Fprintf(w, "  - %s: %s (%s)\n",
		colors.Colorize("Low usage (1-2 usages)", colors.ColorRed, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", lowUsage), colors.ColorRed, p.NoColor),
		colors.Colorize(fmt.Sprintf("%.1f%%", lowPct), colors.ColorRed, p.NoColor))

	// Medium usage - cyan
	medPct := float64(mediumUsage) / float64(totalMethods) * 100
	fmt.Fprintf(w, "  - %s: %s (%s)\n",
		colors.Colorize("Medium (3-5 usages)", colors.ColorYellow, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", mediumUsage), colors.ColorYellow, p.NoColor),
		colors.Colorize(fmt.Sprintf("%.1f%%", medPct), colors.ColorYellow, p.NoColor))

	// High usage - green
	highPct := float64(highUsage) / float64(totalMethods) * 100
	fmt.Fprintf(w, "  - %s: %s (%s)\n\n",
		colors.Colorize("High usage (6+ usages)", colors.ColorGreen, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", highUsage), colors.ColorGreen, p.NoColor),
		colors.Colorize(fmt.Sprintf("%.1f%%", highPct), colors.ColorGreen, p.NoColor))

	fmt.Fprintln(w, colors.Colorize("Call type distribution:", colors.ColorBold, p.NoColor))
	fmt.Fprintf(w, "  - %s: %s\n",
		colors.Colorize("Instance calls", colors.ColorGreen, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", totalInstanceCalls), colors.ColorGreen, p.NoColor))
	fmt.Fprintf(w, "  - %s: %s\n",
		colors.Colorize("Class calls", colors.ColorCyan, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", totalClassCalls), colors.ColorCyan, p.NoColor))
	fmt.Fprintf(w, "  - %s: %s\n",
		colors.Colorize("Static calls", colors.ColorBlue, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", totalStaticCalls), colors.ColorBlue, p.NoColor))
	fmt.Fprintf(w, "  - %s: %s\n",
		colors.Colorize("Function calls", colors.ColorYellow, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", totalFunctionCalls), colors.ColorYellow, p.NoColor))
	fmt.Fprintf(w, "  - %s: %s\n",
		colors.Colorize("Decorator usage", colors.ColorPurple, p.NoColor),
		colors.Colorize(fmt.Sprintf("%d", totalDecoratorCalls), colors.ColorPurple, p.NoColor))

	fmt.Fprintln(w, separator)

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
			loc := strings.TrimSpace(u.Location) // expected "path:line:col"
			ctx := sanitizeContext(u.Context)

			if ctx == "" {
				if u.CallType != "" {
					ctx = fmt.Sprintf("%s [%s]", r.Method.Name, string(u.CallType))
				} else {
					ctx = r.Method.Name
				}
			}

			if _, err := fmt.Fprintf(w, "%s:%s\n", loc, ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func sanitizeContext(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.TrimLeft(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

//================================================================================
// Graphviz
//================================================================================

type GraphvizPrinter struct {
	opts Options
}

func NewGraphvizPrinter(opts Options) *GraphvizPrinter {
	return &GraphvizPrinter{opts: opts}
}

func extractPathFromUsage(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) >= 4 {
		return strings.Join(parts[:len(parts)-3], ":")
	}
	if len(parts) == 3 {
		return parts[0]
	}
	return s
}

func (GraphvizPrinter) Print(w io.Writer, results []finder.MethodUsage) error {
	fmt.Fprintln(w, "digraph G {")
	fmt.Fprintln(w, `  rankdir=LR;`)
	fmt.Fprintln(w, `  node [shape=box, fontsize=10];`)

	nodes := make(map[string]struct{})
	edges := make(map[string]struct{})

	normalizeNode := func(filePath, funcName string) string {
		base := filepath.Base(filePath)
		ext := filepath.Ext(base)
		if len(ext) > 0 {
			base = strings.TrimSuffix(base, ext)
		}
		return base + ":" + funcName
	}

	for _, r := range results {
		callee := normalizeNode(r.Method.Filename, r.Method.Name)
		nodes[callee] = struct{}{}

		for _, u := range r.Usages {
			useFile := extractPathFromUsage(u.Context)
			if useFile == "" {
				continue
			}
			caller := normalizeNode(useFile, "<module>")
			nodes[caller] = struct{}{}

			edgeKey := `"` + caller + `"->"` + callee + `"`
			edges[edgeKey] = struct{}{}
		}
	}

	for n := range nodes {
		fmt.Fprintf(w, "  %q;\n", n)
	}
	for e := range edges {
		fmt.Fprintf(w, "  %s;\n", e)
	}

	fmt.Fprintln(w, "}")
	return nil
}

//================================================================================
// Factory
//================================================================================

type Kind string

const (
	KindConsole  Kind = "console"
	KindJSON     Kind = "json"
	KindVimGrep  Kind = "vimgrep"
	KindGraphviz Kind = "graphviz"
)

var OutputKinds = map[string]Kind{
	"console":  KindConsole,
	"json":     KindJSON,
	"vimgrep":  KindVimGrep,
	"graphviz": KindGraphviz,
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
	case KindGraphviz:
		return GraphvizPrinter{}
	case KindConsole:
		fallthrough
	default:
		return ConsolePrinter{NoColor: opts.NoColor}
	}
}
