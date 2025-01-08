package main

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
	wd := chdir(path.Join("testdata", "testfail"), t)
	defer chdir(wd, t)

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
	wd := chdir(path.Join("testdata", "stderr"), t)
	defer chdir(wd, t)

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

func TestMainCmdRunFail(t *testing.T) {
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

	wd := chdir(path.Join("testdata", "testfail"), t)
	defer chdir(wd, t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	p := setpath(wd, t)
	defer setpath(p, t)

	main()
}

func TestMainTimeout(t *testing.T) {
	wd := chdir(path.Join("testdata", "timeout"), t)
	defer chdir(wd, t)

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

	wd := chdir(path.Join("testdata", "testok"), t)
	defer chdir(wd, t)

	pp := checkHasTestsPackagesPath
	checkHasTestsPackagesPath = "\x00<\\/>" // I think \x00 is not allowed in Linux and Windows.
	defer func() { checkHasTestsPackagesPath = pp }()

	main()
}

func TestMainHasTestsWithoutTests(t *testing.T) {
	wd := chdir(path.Join("testdata", "withouttests"), t)
	defer chdir(wd, t)

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

func TestRunTestsFail(t *testing.T) {
	wd := chdir(path.Join("testdata", "testfail"), t)
	defer chdir(wd, t)

	stderr := new(bytes.Buffer)
	results := new(bytes.Buffer)

	ok, err := runTests(stderr, results)
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
	wd := chdir(path.Join("testdata", "testok"), t)
	defer chdir(wd, t)

	stderr := new(bytes.Buffer)
	results := new(bytes.Buffer)

	ok, err := runTests(stderr, results)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Errorf("ok = false, want true")
	}
}

func TestRunTestsCmdRunFail(t *testing.T) {
	wd := chdir(path.Join("testdata", "testfail"), t)
	defer chdir(wd, t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	p := setpath(wd, t)
	defer setpath(p, t)

	stderr := new(bytes.Buffer)
	results := new(bytes.Buffer)

	if _, err = runTests(stderr, results); err == nil {
		t.Errorf("not returned a error")
	}
}

func TestRunTestsCmdNoJson(t *testing.T) {
	pgo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

	wd := chdir(path.Join("testdata", "nojson"), t)
	defer chdir(wd, t)

	cmd := exec.Command(pgo, "build", "go.go")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if wd, err = os.Getwd(); err != nil {
		t.Fatal(err)
	}

	p := setpath(wd, t)
	defer setpath(p, t)

	stderr := new(bytes.Buffer)
	results := new(bytes.Buffer)

	if _, err = runTests(stderr, results); err == nil {
		t.Errorf("not returned a error")
	}
}

func TestRunTestsTimeout(t *testing.T) {
	wd := chdir(path.Join("testdata", "timeout"), t)
	defer chdir(wd, t)

	stderr := new(bytes.Buffer)
	results := new(bytes.Buffer)

	packageTestTimeout = 1 * time.Millisecond

	ok, err := runTests(stderr, results)
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Errorf("timeout exceeded but runtests returned ok = true")
	}
}

func TestCheckHasTests(t *testing.T) {
	wd := chdir(path.Join("testdata", "testok"), t)
	defer chdir(wd, t)

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
	wd := chdir(path.Join("testdata", "notests"), t)
	defer chdir(wd, t)

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
	wd := chdir(path.Join("testdata", "testok"), t)
	defer chdir(wd, t)
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
	wd := chdir(path.Join("testdata", "withouttests"), t)
	defer chdir(wd, t)

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
	wd := chdir(path.Join("testdata", "noneedtests"), t)
	defer chdir(wd, t)

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
		func TestSum(a, b int) {}
		func TestSum2(a int) {}
		func TestSum3(a *int) {}
		func TestSum4(a *A) {}
		func TestSum5(a *testing.B) {}
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
	wd := chdir(path.Join("testdata", "noneedtests"), t)
	defer chdir(wd, t)

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

// chdir changes the current working directory, it calls os.Chdir with new. It returns the working directory
// that was in effect before the change.
func chdir(new string, t *testing.T) (old string) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err = os.Chdir(new); err != nil {
		t.Fatal(err)
	}

	return
}

// setpath changes the PATH, it calls os.Setenv with new. It returns the PATH
// that was in effect before the change.
func setpath(new string, t *testing.T) (old string) {
	old = os.Getenv("PATH")

	if err := os.Setenv("PATH", new); err != nil {
		t.Fatal(err)
	}

	return
}
