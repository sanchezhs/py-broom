// Package savers mananges how results are stored, if stored
package savers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sanchezhs/py-broom/finder"
)

func SaveResults(results []finder.MethodUsage, outputFile string) error {
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

func SaveResultsJSON(results []finder.MethodUsage, outputFile string) error {
	analysis := finder.AnalysisResult{
		TotalMethods: len(results),
		Results:      results,
	}

	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputFile, data, 0644)
}
