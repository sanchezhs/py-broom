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
	Method     Method   `json:"method"`
	Usages     []string `json:"usages"`
	UsageCount int      `json:"usage_count"`
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

func searchMethodUsages(methodName string, rootDir string, skipTests bool) ([]string, error) {
	globs := []string{"*.py"}
	if skipTests {
		globs = append(globs, "!test_*.py", "!*_test.py")
	}

	args := []string{"--vimgrep"}
	for _, g := range globs {
		args = append(args, "--glob", g)
	}

	searchPattern := fmt.Sprintf(`\b%s\s*\(`, regexp.QuoteMeta(methodName))
	args = append(args, searchPattern, rootDir)

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

func FilterUsages(usages []string, skipImports bool, skipDefinitions bool) []string {
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

func AnalyzeMethodUsages(methods []Method, rootDir string, filters FileFilter) []MethodUsage {
	resultsChan := make(chan MethodUsage, len(methods))
	var wg sync.WaitGroup

	for _, method := range methods {
		wg.Add(1)
		go func(m Method) {
			defer wg.Done()

			usages, err := searchMethodUsages(m.Name, rootDir, filters.SkipTests)
			if err != nil {
				log.Printf("Error searching for method %s: %v", m.Name, err)
				resultsChan <- MethodUsage{
					Method:     m,
					Usages:     []string{},
					UsageCount: 0,
				}
				return
			}

			filteredUsages := FilterUsages(usages, filters.SkipImports, filters.SkipDefinitions)

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

func FilterByUsageCount(results []MethodUsage, minUsages, maxUsages int) []MethodUsage {
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

func SortResults(results []MethodUsage, sortBy string) {
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
