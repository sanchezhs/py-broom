package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

type PyMethod struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	LineNo   int    `json:"line_number"`
}

type PyFile struct {
	dir  string
	base string
	path string
}

type MethodUsage struct {
	Method     PyMethod `json:"method"`
	Usages     []string `json:"usages"`
	UsageCount int      `json:"usage_count"`
}

type AnalysisResult struct {
	TotalMethods int           `json:"total_methods"`
	Results      []MethodUsage `json:"results"`
}

func isPythonFile(filename string) bool {
	return strings.HasSuffix(filename, ".py")
}

func readEntireFile(filepath string) ([]byte, error) {
	return os.ReadFile(filepath)
}

func readDir(rootDir string) ([]PyFile, error) {
	var pythonFiles []PyFile
	err := filepath.WalkDir(rootDir,
		func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				name := entry.Name()
				if name == ".git" || name == "__pycache__" || name == ".venv" || name == "venv" || name == "node_modules" {
					return filepath.SkipDir
				}
			}
			if isPythonFile(path) {
				pyFile := PyFile{
					dir:  filepath.Dir(path),
					base: filepath.Base(path),
					path: path,
				}
				pythonFiles = append(pythonFiles, pyFile)
			}
			return nil
		})
	return pythonFiles, err
}

func searchMethodUsages(methodName string, rootDir string) ([]string, error) {
	cmd := exec.Command("rg", "--vimgrep", "--glob", "*.py", methodName, rootDir)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines, nil
}

func isImportLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ")
}

func filterUsages(usages []string, skipImports bool, skipDefinitions bool) []string {
	if !skipImports && !skipDefinitions {
		return usages
	}

	var filtered []string
	defRegex := regexp.MustCompile(`def\s+\w+\s*\(`)

	for _, usage := range usages {
		parts := strings.SplitN(usage, ":", 4)
		if len(parts) < 4 {
			filtered = append(filtered, usage)
			continue
		}

		lineContent := parts[3]

		if skipImports && isImportLine(lineContent) {
			continue
		}

		if skipDefinitions && defRegex.MatchString(lineContent) {
			continue
		}

		filtered = append(filtered, usage)
	}

	return filtered
}

func isPrivateMethod(methodName string) bool {
	return strings.HasPrefix(methodName, "_")
}

func findMethods(files []PyFile, skipPrivate bool) []PyMethod {
	re := regexp.MustCompile(`def\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

	methodsChan := make(chan []PyMethod, len(files))
	var wg sync.WaitGroup

	for _, pyFile := range files {
		wg.Add(1)
		go func(file PyFile) {
			defer wg.Done()

			var fileMethods []PyMethod
			data, err := readEntireFile(file.path)
			if err != nil {
				log.Printf("Error reading file %s: %v", file.path, err)
				methodsChan <- fileMethods
				return
			}

			lines := strings.Split(string(data), "\n")
			for lineNo, line := range lines {
				matches := re.FindStringSubmatch(line)
				if len(matches) > 1 {
					methodName := matches[1]

					if skipPrivate && isPrivateMethod(methodName) {
						continue
					}

					fileMethods = append(fileMethods, PyMethod{
						Name:     methodName,
						Filename: file.path,
						LineNo:   lineNo + 1,
					})
				}
			}
			methodsChan <- fileMethods
		}(pyFile)
	}

	go func() {
		wg.Wait()
		close(methodsChan)
	}()

	var allMethods []PyMethod
	for methods := range methodsChan {
		allMethods = append(allMethods, methods...)
	}

	return allMethods
}

func analyzeMethodUsages(methods []PyMethod, rootDir string, skipImports bool) []MethodUsage {
	resultsChan := make(chan MethodUsage, len(methods))
	var wg sync.WaitGroup

	for _, method := range methods {
		wg.Add(1)
		go func(m PyMethod) {
			defer wg.Done()

			usages, err := searchMethodUsages(m.Name, rootDir)
			if err != nil {
				log.Printf("Error searching for method %s: %v", m.Name, err)
				resultsChan <- MethodUsage{
					Method:     m,
					Usages:     []string{},
					UsageCount: 0,
				}
				return
			}

			filteredUsages := filterUsages(usages, skipImports, false)

			resultsChan <- MethodUsage{
				Method:     m,
				Usages:     filteredUsages,
				UsageCount: len(filteredUsages),
			}
		}(method)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var results []MethodUsage
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

func filterByUsageCount(results []MethodUsage, minUsages, maxUsages int) []MethodUsage {
	var filtered []MethodUsage

	for _, result := range results {
		if minUsages >= 0 && result.UsageCount < minUsages {
			continue
		}

		if maxUsages >= 0 && result.UsageCount > maxUsages {
			continue
		}

		filtered = append(filtered, result)
	}

	return filtered
}

func sortResults(results []MethodUsage, sortBy string) {
	switch sortBy {
	case "name":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Method.Name < results[j].Method.Name
		})
	case "file":
		sort.Slice(results, func(i, j int) bool {
			if results[i].Method.Filename == results[j].Method.Filename {
				return results[i].Method.LineNo < results[j].Method.LineNo
			}
			return results[i].Method.Filename < results[j].Method.Filename
		})
	case "usages":
		sort.Slice(results, func(i, j int) bool {
			if results[i].UsageCount == results[j].UsageCount {
				return results[i].Method.Name < results[j].Method.Name
			}
			return results[i].UsageCount < results[j].UsageCount
		})
	case "usages-desc":
		sort.Slice(results, func(i, j int) bool {
			if results[i].UsageCount == results[j].UsageCount {
				return results[i].Method.Name < results[j].Method.Name
			}
			return results[i].UsageCount > results[j].UsageCount
		})
	default:
		sort.Slice(results, func(i, j int) bool {
			if results[i].Method.Filename == results[j].Method.Filename {
				return results[i].Method.LineNo < results[j].Method.LineNo
			}
			return results[i].Method.Filename < results[j].Method.Filename
		})
	}
}

func colorize(text string, color string, noColor bool) string {
	if noColor {
		return text
	}
	return color + text + ColorReset
}

func printResults(results []MethodUsage, noColor bool) {
	for _, result := range results {
		fmt.Printf("%s %s\n",
			colorize("Method:", ColorYellow+ColorBold, noColor),
			colorize(result.Method.Name, ColorGreen+ColorBold, noColor))
		fmt.Printf("%s %s:%d\n",
			colorize("Defined in:", ColorYellow, noColor),
			result.Method.Filename,
			result.Method.LineNo)

		usageColor := ColorGreen
		if result.UsageCount == 0 {
			usageColor = ColorRed
		}
		fmt.Printf("%s %s\n",
			colorize("Usages found:", ColorYellow, noColor),
			colorize(fmt.Sprintf("%d", result.UsageCount), usageColor+ColorBold, noColor))

		if len(result.Usages) > 0 {
			fmt.Println(colorize("Locations:", ColorYellow, noColor))
			for _, usage := range result.Usages {
				fmt.Printf("  %s %s\n",
					colorize("-", ColorBlue, noColor),
					usage)
			}
		} else {
			fmt.Printf("  %s\n", colorize("(No usages found)", ColorRed, noColor))
		}
		fmt.Println()
		fmt.Println(colorize(strings.Repeat("-", 80), ColorPurple, noColor))
		fmt.Println()
	}
}

func saveResults(results []MethodUsage, outputFile string) error {
	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, result := range results {
		fmt.Fprintf(f, "Method: %s\n", result.Method.Name)
		fmt.Fprintf(f, "Defined in: %s:%d\n", result.Method.Filename, result.Method.LineNo)
		fmt.Fprintf(f, "Usages found: %d\n", result.UsageCount)

		if len(result.Usages) > 0 {
			fmt.Fprintln(f, "Locations:")
			for _, usage := range result.Usages {
				fmt.Fprintf(f, "  - %s\n", usage)
			}
		} else {
			fmt.Fprintln(f, "  (No usages found)")
		}
		fmt.Fprintln(f)
		fmt.Fprintln(f, strings.Repeat("-", 80))
		fmt.Fprintln(f)
	}

	return nil
}

func saveResultsJSON(results []MethodUsage, outputFile string) error {
	analysis := AnalysisResult{
		TotalMethods: len(results),
		Results:      results,
	}

	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputFile, data, 0644)
}

func main() {
	dir := flag.String("dir", ".", "Directory to search for Python files")
	output := flag.String("output", "", "Output file (optional, defaults to stdout)")
	jsonOutput := flag.String("json", "", "Output results as JSON file")
	verbose := flag.Bool("verbose", false, "Show detailed information during execution")
	skipImports := flag.Bool("skip-imports", false, "Skip import statements in usage results")
	skipPrivate := flag.Bool("skip-private", false, "Skip private methods (starting with _)")
	noColor := flag.Bool("no-color", false, "Disable colored output")
	minUsages := flag.Int("min-usages", -1, "Filter methods with at least N usages (-1 = no filter)")
	maxUsages := flag.Int("max-usages", -1, "Filter methods with at most N usages (-1 = no filter)")
	sortBy := flag.String("sort", "file", "Sort results by: name, file, usages, usages-desc")
	flag.Parse()

	if *verbose {
		log.Printf("Searching for Python files in: %s\n", *dir)
	}

	if _, err := exec.LookPath("rg"); err != nil {
		log.Fatal("Error: ripgrep (rg) is not installed. Please install it first.")
	}

	files, err := readDir(*dir)
	if err != nil {
		log.Fatalf("Error reading directory: %v", err)
	}

	if *verbose {
		log.Printf("Found %d Python files\n", len(files))
	}

	if len(files) == 0 {
		log.Fatal("No Python files found in the specified directory")
	}

	methods := findMethods(files, *skipPrivate)
	if *verbose {
		log.Printf("Found %d methods\n", len(methods))
	}

	if len(methods) == 0 {
		log.Fatal("No method definitions found")
	}

	if *verbose {
		log.Println("Analyzing method usages...")
	}
	results := analyzeMethodUsages(methods, *dir, *skipImports)

	if *minUsages >= 0 || *maxUsages >= 0 {
		results = filterByUsageCount(results, *minUsages, *maxUsages)
		if *verbose {
			log.Printf("Filtered to %d methods based on usage count\n", len(results))
		}
	}

	if len(results) == 0 {
		fmt.Println(colorize("No methods found matching the filter criteria", ColorYellow, *noColor))
		return
	}

	sortResults(results, *sortBy)
	if *verbose {
		log.Printf("Results sorted by: %s\n", *sortBy)
	}

	if *jsonOutput != "" {
		if err := saveResultsJSON(results, *jsonOutput); err != nil {
			log.Fatalf("Error saving JSON results: %v", err)
		}
		fmt.Printf("%s %s\n",
			colorize("Results saved to:", ColorGreen, *noColor),
			colorize(*jsonOutput, ColorCyan+ColorBold, *noColor))
	} else if *output != "" {
		if err := saveResults(results, *output); err != nil {
			log.Fatalf("Error saving results: %v", err)
		}
		fmt.Printf("%s %s\n",
			colorize("Results saved to:", ColorGreen, *noColor),
			colorize(*output, ColorCyan+ColorBold, *noColor))
	} else {
		printResults(results, *noColor)
	}
}
