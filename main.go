package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/sanchezhs/py-broom/colors"
	"github.com/sanchezhs/py-broom/finder"
	"github.com/sanchezhs/py-broom/printers"
	"github.com/sanchezhs/py-broom/savers"
)

const programName = "pybr"

func main() {
	dir := flag.String("dir", ".", "Directory to search for Python files")
	output := flag.String("output", "", "Output file (optional, defaults to stdout)")
	jsonOutput := flag.String("json", "", "Output results as JSON file")
	verbose := flag.Bool("verbose", false, "Show detailed information during execution")
	format := flag.String("format", "console", fmt.Sprintf("How to output results, valid types are %v", printers.GetKinds()))
	skipImports := flag.Bool("skip-imports", false, "Skip import statements in usage results")
	skipPrivate := flag.Bool("skip-private", false, "Skip private methods (starting with _)")
	skipTests := flag.Bool("skip-tests", true, "Skip methods definitions in tests files")
	noColor := flag.Bool("no-color", false, "Disable colored output")
	minUsages := flag.Int("min-usages", -1, "Filter methods with at least N usages (-1 = no filter)")
	maxUsages := flag.Int("max-usages", -1, "Filter methods with at most N usages (-1 = no filter)")
	sortBy := flag.String("sort", "file", "Sort results by: name, file, usages, usages-desc")
	flag.Parse()

	if *verbose {
		log.Printf("Searching for Python files in: %s\n", *dir)
	}

	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Printf("%s: Error ripgrep (rg) is not installed. Please install it first.\n", programName)
		return
	}

	files, err := finder.ReadDir(*dir)
	if err != nil {
		fmt.Printf("%s: Error reading directory: %v\n", programName, err)
		return
	}

	if *verbose {
		log.Printf("Found %d Python files\n", len(files))
	}

	if len(files) == 0 {
		fmt.Printf("%s: No Python files found in the specified directory\n", programName)
		return
	}

	methodFilters := finder.MethodFilter{
		SkipPrivate: *skipPrivate,
	}
	methods := finder.FindMethods(files, methodFilters)
	if *verbose {
		log.Printf("Found %d methods\n", len(methods))
	}

	if len(methods) == 0 {
		fmt.Printf("%s: No method definitions found\n", programName)
		return
	}

	if *verbose {
		log.Println("Analyzing method usages...")
	}

	fileFilters := finder.FileFilter{
		SkipImports: *skipImports,
		SkipTests:   *skipTests,
	}
	results := finder.AnalyzeMethodUsages(methods, *dir, fileFilters)

	if *minUsages >= 0 || *maxUsages >= 0 {
		results = finder.FilterByUsageCount(results, *minUsages, *maxUsages)
		if *verbose {
			log.Printf("Filtered to %d methods based on usage count\n", len(results))
		}
	}

	if len(results) == 0 {
		fmt.Printf("%s: No methods found matching the filter criteria\n", programName)
		return
	}

	finder.SortResults(results, *sortBy)
	if *verbose {
		log.Printf("Results sorted by: %s\n", *sortBy)
	}

	if *jsonOutput != "" {
		if err := savers.SaveResultsJSON(results, *jsonOutput); err != nil {
			log.Fatalf("Error saving JSON results: %v", err)
		}
		fmt.Printf("%s %s\n",
			colors.Colorize("Results saved to:", colors.ColorGreen, *noColor),
			colors.Colorize(*jsonOutput, colors.ColorCyan+colors.ColorBold, *noColor))
	} else if *output != "" {
		if err := savers.SaveResults(results, *output); err != nil {
			log.Fatalf("Error saving results: %v", err)
		}
		fmt.Printf("%s %s\n",
			colors.Colorize("Results saved to:", colors.ColorGreen, *noColor),
			colors.Colorize(*output, colors.ColorCyan+colors.ColorBold, *noColor))
	} else {
		kind := printers.OutputKinds[*format]

		if kind == "" {
			fmt.Printf("%s: Invalid output format '%s'\n\n", programName, *format)
			flag.PrintDefaults()
			return
		}

		pr := printers.New(kind, printers.Options{NoColor: *noColor})
		_ = pr.Print(os.Stdout, results)
	}
}
