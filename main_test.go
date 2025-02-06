package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bytes"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"
)

func TestMainTestsFail(t *testing.T) {
	t.Chdir(path.Join("testdata", "testfail"))
	var exitCode int

	stdout = new(bytes.Buffer)
	exit = func(code int) {
		exitCode = code
	}

	main()

	if r := stdout.(*bytes.Buffer).String(); r != "testfail: TestSum failed\n\n" {
		t.Errorf("output = %q, want %q", r, "testfail: TestSum failed\n\n")
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestMainTestsStderr(t *testing.T) {
	t.Chdir(path.Join("testdata", "stderr"))

	var exitCode int

	stdout = new(bytes.Buffer)
	exit = func(code int) {
		exitCode = code
	}

	main()

	if stdout.(*bytes.Buffer).Len() == 0 {
		t.Errorf("without stderr")
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestMainCmdRunTestsFail(t *testing.T) {
	executeVet = false
	defer func() {
		executeVet = true
	}()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("panic expected")
		}

		_, ok := r.(error)
		if !ok {
			t.Errorf("error expected, got %T", r)
		}
	}()

	t.Chdir(path.Join("testdata", "testfail"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	main()
}

func TestMainCmdRunVetFail(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("panic expected")
		}

		_, ok := r.(error)
		if !ok {
			t.Errorf("error expected, got %T", r)
		}
	}()

	t.Chdir(path.Join("testdata", "testfail"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	main()
}

func TestMainTimeout(t *testing.T) {
	t.Chdir(path.Join("testdata", "timeout"))

	packageTestTimeout = 1 * time.Millisecond

	stdout = new(bytes.Buffer)

	var exitCode int
	exit = func(code int) {
		exitCode = code
	}

	main()

	if stdout.(*bytes.Buffer).Len() == 0 {
		t.Errorf("without information")
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestMainHasTestsPackageLoadError(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("panic expected")
		}

		_, ok := r.(error)
		if !ok {
			t.Errorf("error expected, got %T", r)
		}
	}()

	t.Chdir(path.Join("testdata", "testok"))

	pp := checkHasTestsPackagesPath
	checkHasTestsPackagesPath = "\x00<\\/>" // I think \x00 is not allowed in Linux and Windows.
	defer func() { checkHasTestsPackagesPath = pp }()

	main()
}

func TestMainHasTestsWithoutTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "withouttests"))

	stdout = new(bytes.Buffer)

	var exitCode int
	exit = func(code int) {
		exitCode = code
	}

	main()

	if stdout.(*bytes.Buffer).Len() == 0 {
		t.Errorf("without information")
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestRunVetFail(t *testing.T) {
	t.Chdir(path.Join("testdata", "printfvet"))

	results := new(bytes.Buffer)

	ok, err := runVet(results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Errorf("ok = true, want false")
	}

	if results.Len() == 0 {
		t.Errorf("vet without output")
	}
}

func TestRunTestsFail(t *testing.T) {
	t.Chdir(path.Join("testdata", "testfail"))

	results := new(bytes.Buffer)

	ok, err := runTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Errorf("ok = true, want false")
	}

	if r := results.String(); r != "testfail: TestSum failed\n" {
		t.Errorf("output = %q, want %q", r, "testfail: TestSum failed\n")
	}
}

func TestRunTestsOk(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	results := new(bytes.Buffer)

	ok, err := runTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Errorf("ok = false, want true")
	}
}

func TestRunTestsCmdRunFail(t *testing.T) {
	t.Chdir(path.Join("testdata", "testfail"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	results := new(bytes.Buffer)

	if _, err = runTests(results); err == nil {
		t.Errorf("not returned a error")
	}
}

func TestRunTestsCmdNoJson(t *testing.T) {
	pgo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(path.Join("testdata", "nojson"))

	// create a fake go command that generates incorrect json.
	cmd := exec.Command(pgo, "build", "go.go")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(path.Join(".", "go.exe")); err != nil {
			t.Fatal(err)
		}
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	results := new(bytes.Buffer)

	if _, err = runTests(results); err == nil {
		t.Errorf("not returned a error")
	}
}

func TestRunTestsTimeout(t *testing.T) {
	t.Chdir(path.Join("testdata", "timeout"))

	results := new(bytes.Buffer)

	packageTestTimeout = 1 * time.Millisecond

	ok, err := runTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Errorf("timeout exceeded but runtests returned ok = true")
	}
}

func TestRunTestsCoverageLessThan100(t *testing.T) {
	t.Chdir(path.Join("testdata", "nocoverage"))

	results := new(bytes.Buffer)

	ok, err := runTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Errorf("coverage is ot 100%% but runtests returned ok = true")
	}
}

func TestCheckHasTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	results := new(bytes.Buffer)

	ok, err := verifyIfHasTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Errorf("ok = false, want true")
		t.Error(results.String())
	}
}

func TestCheckHasFuzzTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "testfuzz"))

	results := new(bytes.Buffer)

	ok, err := verifyIfHasTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Errorf("ok = false, want true")
		t.Error(results.String())
	}
}

func TestHasNoTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "notests"))

	results := new(bytes.Buffer)

	ok, err := verifyIfHasTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Errorf("ok = true, want false")
		return
	}

	expectedResults := "notests has no tests\nnotests/sub has no tests\n"

	if results.String() != expectedResults {
		t.Errorf("results = %q, want %q", results.String(), expectedResults)
	}
}

func TestCheckHasTestsPackageLoadError(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))
	pp := checkHasTestsPackagesPath
	checkHasTestsPackagesPath = "\x00<\\/>" // I think \x00 is not allowed in Linux and Windows.
	defer func() { checkHasTestsPackagesPath = pp }()

	results := new(bytes.Buffer)

	_, err := verifyIfHasTests(results)
	if err == nil {
		t.Errorf("want error, got nil")
	}
}

func TestCheckHasTestsWithoutTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "withouttests"))

	results := new(bytes.Buffer)

	ok, err := verifyIfHasTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Error("package withouttests do not has tests, but verifyIfHasTests returned ok = true")
	}
}

func TestCheckHasTestsNeedNotTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "noneedtests"))

	results := new(bytes.Buffer)

	ok, err := verifyIfHasTests(results)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Log(results.String())
		t.Error("package noneedtests do not need tests, but verifyIfHasTests returned ok = false")
	}
}

func TestIsTestFunction(t *testing.T) {
	code := `
		package code
		import "testing"
		type A int
		func sum(a, b int) int {return a + b}
		func Testsum(t *testing.T) {}
		func TestSum(a, b int) {}
		func TestSum2(a int) {}
		func TestSum3(a *int) {}
		func TestSum4(a *A) {}
		func TestSum5(a *testing.B) {}
		func FuzzSum(a, b int) {}
		func FuzzSum2(a int) {}
		func FuzzSum3(a *int) {}
		func FuzzSum4(a *A) {}
		func FuzzSum5(a *testing.B) {}
		func Fuzzsum() {}
	`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "code.go", code, 0)
	if err != nil {
		t.Fatal(err)
	}

	conf := types.Config{
		Importer: importer.Default(),
	}
	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
	}
	if _, err = conf.Check("code", fset, []*ast.File{f}, info); err != nil {
		t.Fatal(err)
	}

	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if isTestFunction(info, fd) {
			t.Errorf("got %s is a test function, want that it is not", fd.Name.Name)
		}
	}
}

func TestNoNeedTests(t *testing.T) {
	t.Chdir(path.Join("testdata", "noneedtests"))

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypesInfo,
	}
	pkgs, err := packages.Load(cfg, checkHasTestsPackagesPath)
	if err != nil {
		t.Fatal(err)
	}

	if needTests(pkgs[0]) {
		t.Errorf("package noneedtests no need tests, but needTests() returned true")
	}
}

func BenchmarkHasNoTests(b *testing.B) {
	b.Chdir(path.Join("testdata", "notests"))
	for b.Loop() {
		main()
	}
}
