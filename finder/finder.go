// Package finder has the core logic to find Python methods within files
package finder

import (
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

type CallType string

const (
	CallTypeInstance   CallType = "instance"   // self.method()
	CallTypeClass      CallType = "class"      // cls.method()
	CallTypeStatic     CallType = "static"     // ClassName.method()
	CallTypeFunction   CallType = "function"   // method() - standalone or imported
	CallTypeDefinition CallType = "definition" // def method():
	CallTypeDecorator  CallType = "decorator"  // @decorator
)

type Usage struct {
	Location string   `json:"location"`
	CallType CallType `json:"call_type"`
	Context  string   `json:"context"` // The actual line of code
}

type Method struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	LineNo   int    `json:"line_number"`
}

type File struct {
	Dir  string
	Base string
	Path string
}

type MethodUsage struct {
	Method       Method           `json:"method"`
	Usages       []Usage          `json:"usages"`
	UsagesByType map[CallType]int `json:"usages_by_type"`
	TotalUsages  int              `json:"total_usages"`
}

type AnalysisResult struct {
	TotalMethods int           `json:"total_methods"`
	Results      []MethodUsage `json:"results"`
}

type MethodFilter struct {
	SkipPrivate bool
}

type FileFilter struct {
	SkipImports     bool
	SkipTests       bool
	SkipDefinitions bool
}

type CallPattern struct {
	Type    CallType
	Pattern *regexp.Regexp
}

func isPythonFile(filename string) bool {
	return strings.HasSuffix(filename, ".py")
}

func readEntireFile(filepath string) ([]byte, error) {
	return os.ReadFile(filepath)
}

func ReadDir(rootDir string) ([]File, error) {
	var pythonFiles []File
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
				pyFile := File{
					Dir:  filepath.Dir(path),
					Base: filepath.Base(path),
					Path: path,
				}
				pythonFiles = append(pythonFiles, pyFile)
			}
			return nil
		})
	return pythonFiles, err
}

func buildCallPatterns(methodName string) []CallPattern {
	escaped := regexp.QuoteMeta(methodName)

	return []CallPattern{
		// Definition: def method_name(
		{
			Type:    CallTypeDefinition,
			Pattern: regexp.MustCompile(`^\s*def\s+` + escaped + `\s*\(`),
		},
		// Decorator: @method_name or @something.method_name
		{
			Type:    CallTypeDecorator,
			Pattern: regexp.MustCompile(`^\s*@(\w+\.)*` + escaped + `\s*$`),
		},
		// Instance call: self.method_name(
		{
			Type:    CallTypeInstance,
			Pattern: regexp.MustCompile(`\bself\.` + escaped + `\s*\(`),
		},
		// Class call: cls.method_name(
		{
			Type:    CallTypeClass,
			Pattern: regexp.MustCompile(`\bcls\.` + escaped + `\s*\(`),
		},
		// Static/Class name call: ClassName.method_name( or obj.method_name(
		// This pattern matches any identifier followed by .method_name(
		{
			Type:    CallTypeStatic,
			Pattern: regexp.MustCompile(`\b[A-Z][a-zA-Z0-9_]*\.` + escaped + `\s*\(`),
		},
		// Function call: method_name( (not preceded by a dot)
		// Negative lookbehind would be ideal but Go doesn't support it
		// So we'll check this separately in classifyUsage
		{
			Type:    CallTypeFunction,
			Pattern: regexp.MustCompile(`(?:^|[^\w.])` + escaped + `\s*\(`),
		},
	}
}

func classifyUsage(line string, methodName string) (CallType, bool) {
	trimmed := strings.TrimSpace(line)

	if strings.HasPrefix(trimmed, "#") {
		return "", false
	}

	if idx := strings.Index(line, "#"); idx != -1 {
		codePart := line[:idx]
		if !strings.Contains(codePart, methodName) {
			return "", false
		}
		line = codePart
	}

	patterns := buildCallPatterns(methodName)

	for _, pattern := range patterns {
		if pattern.Pattern.MatchString(line) {
			if pattern.Type == CallTypeFunction {
				if strings.Contains(line, "."+methodName) {
					continue
				}
			}
			return pattern.Type, true
		}
	}

	return "", false
}

func searchMethodUsages(methodName string, searchDir string, skipTests bool) ([]string, error) {
	globs := []string{"*.py"}
	if skipTests {
		globs = append(globs, "!test_*.py", "!*_test.py")
	}

	args := []string{"--vimgrep"}
	for _, g := range globs {
		args = append(args, "--glob", g)
	}

	searchPattern := fmt.Sprintf(`\b%s\s*\(`, regexp.QuoteMeta(methodName))
	args = append(args, searchPattern, searchDir)

	cmd := exec.Command("rg", args...)
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

func ParseUsages(rawUsages []string, methodName string, filters FileFilter, definedFile string) []Usage {
	var usages []Usage

	for _, rawUsage := range rawUsages {
		parts := strings.SplitN(rawUsage, ":", 4)
		if len(parts) < 4 {
			continue
		}

		filepath := parts[0]
		lineNo := parts[1]
		colNo := parts[2]
		lineContent := parts[3]

		if filters.SkipImports && isImportLine(lineContent) {
			continue
		}

		callType, valid := classifyUsage(lineContent, methodName)
		if !valid {
			continue
		}

		if filters.SkipDefinitions && callType == CallTypeDefinition {
			if filepath != definedFile {
				continue
			}
		}

		location := fmt.Sprintf("%s:%s:%s", filepath, lineNo, colNo)
		usages = append(usages, Usage{
			Location: location,
			CallType: callType,
			Context:  strings.TrimSpace(lineContent),
		})
	}

	return usages
}

func isPrivateMethod(methodName string) bool {
	return strings.HasPrefix(methodName, "_")
}

func FindMethods(files []File, filters MethodFilter) []Method {
	re := regexp.MustCompile(`def\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

	methodsChan := make(chan []Method, len(files))
	var wg sync.WaitGroup

	for _, pyFile := range files {
		wg.Add(1)
		go func(file File) {
			defer wg.Done()

			var fileMethods []Method
			data, err := readEntireFile(file.Path)
			if err != nil {
				log.Printf("Error reading file %s: %v", file.Path, err)
				methodsChan <- fileMethods
				return
			}

			lines := strings.Split(string(data), "\n")
			for lineNo, line := range lines {
				matches := re.FindStringSubmatch(line)
				if len(matches) > 1 {
					methodName := matches[1]

					if filters.SkipPrivate && isPrivateMethod(methodName) {
						continue
					}

					fileMethods = append(fileMethods, Method{
						Name:     methodName,
						Filename: file.Path,
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

	var allMethods []Method
	for methods := range methodsChan {
		allMethods = append(allMethods, methods...)
	}

	return allMethods
}

func AnalyzeMethodUsages(methods []Method, searchDir string, filters FileFilter) []MethodUsage {
	resultsChan := make(chan MethodUsage, len(methods))
	var wg sync.WaitGroup

	for _, method := range methods {
		wg.Add(1)
		go func(m Method) {
			defer wg.Done()

			rawUsages, err := searchMethodUsages(m.Name, searchDir, filters.SkipTests)
			if err != nil {
				log.Printf("Error searching for method %s: %v", m.Name, err)
				resultsChan <- MethodUsage{
					Method:       m,
					Usages:       []Usage{},
					UsagesByType: make(map[CallType]int),
					TotalUsages:  0,
				}
				return
			}

			usages := ParseUsages(rawUsages, m.Name, filters, m.Filename)

			// Count usages by type
			usagesByType := make(map[CallType]int)
			for _, usage := range usages {
				usagesByType[usage.CallType]++
			}

			resultsChan <- MethodUsage{
				Method:       m,
				Usages:       usages,
				UsagesByType: usagesByType,
				TotalUsages:  len(usages),
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

func FilterByUsageCount(results []MethodUsage, minUsages, maxUsages int) []MethodUsage {
	var filtered []MethodUsage

	for _, result := range results {
		if minUsages >= 0 && result.TotalUsages < minUsages {
			continue
		}

		if maxUsages >= 0 && result.TotalUsages > maxUsages {
			continue
		}

		filtered = append(filtered, result)
	}

	return filtered
}

func SortResults(results []MethodUsage, sortBy string, asc bool) {
	sortBy = strings.ToLower(sortBy)

	less := func(i, j int) bool {
		switch sortBy {
		case "name":
			return results[i].Method.Name < results[j].Method.Name

		case "file":
			if results[i].Method.Filename == results[j].Method.Filename {
				return results[i].Method.LineNo < results[j].Method.LineNo
			}
			return results[i].Method.Filename < results[j].Method.Filename

		case "usages":
			if results[i].TotalUsages == results[j].TotalUsages {
				return results[i].Method.Name < results[j].Method.Name
			}
			return results[i].TotalUsages < results[j].TotalUsages

		default:
			if results[i].Method.Filename == results[j].Method.Filename {
				return results[i].Method.LineNo < results[j].Method.LineNo
			}
			return results[i].Method.Filename < results[j].Method.Filename
		}
	}

	if asc {
		sort.Slice(results, less)
	} else {
		sort.Slice(results, func(i, j int) bool { return !less(i, j) })
	}
}

func GetCallTypeOrder() []CallType {
	return []CallType{
		CallTypeDefinition,
		CallTypeInstance,
		CallTypeClass,
		CallTypeStatic,
		CallTypeFunction,
		CallTypeDecorator,
	}
}

func GetCallTypeLabel(ct CallType) string {
	switch ct {
	case CallTypeDefinition:
		return "Definition"
	case CallTypeInstance:
		return "Instance calls"
	case CallTypeClass:
		return "Class calls"
	case CallTypeStatic:
		return "Static calls"
	case CallTypeFunction:
		return "Function calls"
	case CallTypeDecorator:
		return "Decorator usage"
	default:
		return string(ct)
	}
}
