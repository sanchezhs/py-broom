package finder

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func writeFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return p
}

func sortMethods(ms []Method) {
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].Filename != ms[j].Filename {
			return ms[i].Filename < ms[j].Filename
		}
		if ms[i].LineNo != ms[j].LineNo {
			return ms[i].LineNo < ms[j].LineNo
		}
		return ms[i].Name < ms[j].Name
	})
}

func lessMU(a, b MethodUsage) bool {
	if a.Method.Filename != b.Method.Filename {
		return a.Method.Filename < b.Method.Filename
	}
	if a.Method.LineNo != b.Method.LineNo {
		return a.Method.LineNo < b.Method.LineNo
	}
	return a.Method.Name < b.Method.Name
}

func sortMUs(xs []MethodUsage) {
	for i := range xs {
		sort.Strings(xs[i].Usages)
	}
	sort.Slice(xs, func(i, j int) bool { return lessMU(xs[i], xs[j]) })
}

func TestFindMethods_SkipPrivateFalse(t *testing.T) {
	dir := t.TempDir()

	p1 := writeFile(t, dir, "main.py", `
def public_fn():
    pass

def _private_fn():
    pass

class C:
    def method_in_class(self):
        pass
`)

	p2 := writeFile(t, dir, "mock/b.py", `
# comment
def another_fn(a, b):
    return a + b
`)

	missing := filepath.Join(dir, "does_not_exist.py")

	files := []File{
		{
			Dir:  filepath.Dir(p1),
			Base: filepath.Base(p1),
			Path: p1,
		},
		{
			Dir:  filepath.Dir(p2),
			Base: filepath.Base(p2),
			Path: p2,
		},
		{
			Dir:  filepath.Dir(missing),
			Base: filepath.Base(missing),
			Path: missing,
		},
	}

	filter := MethodFilter{SkipPrivate: false}
	got := FindMethods(files, filter)
	sortMethods(got)

	want := []Method{
		{Name: "public_fn", Filename: p1, LineNo: 2},
		{Name: "_private_fn", Filename: p1, LineNo: 5},
		{Name: "method_in_class", Filename: p1, LineNo: 9},
		{Name: "another_fn", Filename: p2, LineNo: 3},
	}
	sortMethods(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findMethods(skipPrivate=false) mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFindMethods_SkipPrivateTrue(t *testing.T) {
	dir := t.TempDir()

	p := writeFile(t, dir, "file.py", `
def a():
    pass
def _hidden():
    pass
def b1_x():
    pass
`)

	files := []File{
		{
			Dir:  filepath.Dir(p),
			Base: filepath.Base(p),
			Path: p,
		},
	}
	filter := MethodFilter{SkipPrivate: true}
	got := FindMethods(files, filter)
	sortMethods(got)

	want := []Method{
		{Name: "a", Filename: p, LineNo: 2},
		{Name: "b1_x", Filename: p, LineNo: 6},
	}
	sortMethods(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findMethods(skipPrivate=true) mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAnalyzeMethodUsages_SkipImportsFalse(t *testing.T) {
	dir := t.TempDir()

	p := writeFile(t, dir, "file.py", `
def testAnalyzeMethodUsages_SkipImportsFalse_one():
    pass
def _hidden():
    pass
def testAnalyzeMethodUsages_SkipImportsFalse_two():
    pass
`)

	methods := []Method{
		{
			Name:     "testAnalyzeMethodUsages_SkipImportsFalse_one",
			Filename: p,
			LineNo:   2,
		},
		{
			Name:     "testAnalyzeMethodUsages_SkipImportsFalse_two",
			Filename: p,
			LineNo:   6,
		},
	}
	filters := FileFilter{
		SkipImports: false,
		SkipTests:   true,
	}

	got := AnalyzeMethodUsages(methods, dir, filters)

	want := []MethodUsage{
		{
			Method: Method{
				Name:     "testAnalyzeMethodUsages_SkipImportsFalse_one",
				Filename: p,
				LineNo:   2,
			},
			Usages: []string{
				fmt.Sprintf("%s:2:5:def testAnalyzeMethodUsages_SkipImportsFalse_one():", p),
			},
			UsageCount: 1,
		},
		{
			Method: Method{
				Name:     "testAnalyzeMethodUsages_SkipImportsFalse_two",
				Filename: p,
				LineNo:   6,
			},
			Usages: []string{
				fmt.Sprintf("%s:6:5:def testAnalyzeMethodUsages_SkipImportsFalse_two():", p),
			},
			UsageCount: 1,
		},
	}

	sortMUs(got)
	sortMUs(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AnalyzeMethodUsages mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
