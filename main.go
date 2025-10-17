package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/sanchezhs/py-broom/finder"
	"github.com/sanchezhs/py-broom/printers"
	"github.com/spf13/cobra"
)

const programName = "pybr"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		dir             string
		output          string
		verbose         bool
		format          string
		skipImports     bool
		skipPrivate     bool
		skipTests       bool
		skipDefinitions bool
		noColor         bool
		minUsages       int
		maxUsages       int
		sortBy          string
	)

	rootCmd := &cobra.Command{
		Use:          programName,
		Short:        "Analyze Python method usages across a repository",
		SilenceUsage: true,         // Do not print usage on handled errors
		Args:         cobra.NoArgs, // No unexpected positional args allowed
		RunE: func(cmd *cobra.Command, args []string) error {
			// Verbose banner
			if verbose {
				log.Printf("Searching for Python files in: %s\n", dir)
			}

			// Check ripgrep
			if _, err := exec.LookPath("rg"); err != nil {
				fmt.Printf("%s: Error ripgrep (rg) is not installed. Please install it first.\n", programName)
				return nil
			}

			// Read python files
			files, err := finder.ReadDir(dir)
			if err != nil {
				fmt.Printf("%s: Error reading directory: %v\n", programName, err)
				return nil
			}
			if verbose {
				log.Printf("Found %d Python files\n", len(files))
			}
			if len(files) == 0 {
				fmt.Printf("%s: No Python files found in the specified directory\n", programName)
				return nil
			}

			// Find methods
			methodFilters := finder.MethodFilter{
				SkipPrivate: skipPrivate,
			}
			methods := finder.FindMethods(files, methodFilters)
			if verbose {
				log.Printf("Found %d methods\n", len(methods))
			}
			if len(methods) == 0 {
				fmt.Printf("%s: No method definitions found\n", programName)
				return nil
			}

			// Analyze usages
			if verbose {
				log.Println("Analyzing method usages...")
			}

			fileFilters := finder.FileFilter{
				SkipImports:     skipImports,
				SkipTests:       skipTests,
				SkipDefinitions: skipDefinitions,
			}
			results := finder.AnalyzeMethodUsages(methods, dir, fileFilters)

			// Filter by usages
			if minUsages >= 0 || maxUsages >= 0 {
				results = finder.FilterByUsageCount(results, minUsages, maxUsages)
				if verbose {
					log.Printf("Filtered to %d methods based on usage count\n", len(results))
				}
			}
			if len(results) == 0 {
				fmt.Printf("%s: No methods found matching the filter criteria\n", programName)
				return nil
			}

			// Sort
			finder.SortResults(results, sortBy)
			if verbose {
				log.Printf("Results sorted by: %s\n", sortBy)
			}

			// Console printer
			kind := printers.OutputKinds[format]
			if kind == "" {
				_ = cmd.Usage()
				return fmt.Errorf("%s: invalid output format '%s'", programName, format)
			}

			pr := printers.New(kind, printers.Options{NoColor: noColor})

			if output != "" {
				f, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("error saving results: %w", err)
				}

				defer f.Close()

				w := bufio.NewWriter(f)
				err = pr.Print(w, results)
				if err != nil {
					return fmt.Errorf("error saving results: %w", err)
				}
				if err := w.Flush(); err != nil {
					return err
				}
				f.Sync()
			} else {
				err = pr.Print(os.Stdout, results)
				if err != nil {
					return fmt.Errorf("error saving results: %w", err)
				}
			}
			return nil
		},
	}

	rootCmd.Flags().StringVarP(&dir, "dir", "d", ".", "Directory to search for Python files")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file (optional, defaults to stdout)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed information during execution")
	rootCmd.Flags().StringVar(&format, "format", "console", fmt.Sprintf("How to output results, valid types are %v", printers.GetKinds()))
	rootCmd.Flags().BoolVar(&skipImports, "skip-imports", false, "Skip import statements in usage results")
	rootCmd.Flags().BoolVar(&skipPrivate, "skip-private", false, "Skip private methods (starting with _)")
	rootCmd.Flags().BoolVar(&skipTests, "skip-tests", true, "Skip methods definitions in tests files")
	rootCmd.Flags().BoolVar(&skipDefinitions, "skip-definitions", false, "Skip methods definitions")
	rootCmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.Flags().IntVar(&minUsages, "min-usages", -1, "Filter methods with at least N usages (-1 = no filter)")
	rootCmd.Flags().IntVar(&maxUsages, "max-usages", -1, "Filter methods with at most N usages (-1 = no filter)")
	rootCmd.Flags().StringVar(&sortBy, "sort", "file", "Sort results by: name, file, usages, usages-desc")

	return rootCmd
}
