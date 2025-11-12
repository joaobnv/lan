package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"
)

// Copyright (c) 2025, João Breno. See the license.

// tests the case where a call to cmd.Run() returns an error.
// To make it return an error we change the PATH environment variable to a directory that don't have
// the go command.
func TestCmdRunError(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	cases := []struct{ op operation }{
		{op: newVet()},
		{op: newStaticcheck()},
		{op: newTests(1 * time.Second)},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%T", c.op), func(t *testing.T) {
			if _, err := c.op.run(); err == nil {
				t.Errorf("run did not return a error")
			}
		})
	}
}

// tests the case where a timeout occurs in the tests.
func TestTestsTimeout(t *testing.T) {
	t.Chdir(path.Join("testdata", "to"))

	cleanTestCache(t)

	op := newTests(100 * time.Nanosecond)
	message, err := op.run()
	if err != nil {
		t.Fatal(err)
	}

	expectedMessage := "\tlan: from executing the tests\nto: panic: test timed out after 100ns"
	if message != expectedMessage {
		t.Errorf("message = %q, want %q", message, expectedMessage)
	}
}

// tests the case where the path used by thereAreTests is invalid.
// In this case we expect that packages.Load returns an error.
func TestPackageLoadError(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	op := newTests(30 * time.Second).(tests)
	// I think \x00 is not allowed in Linux and Windows.
	op.tat.packagesPath = "\x00<\\/>"
	if _, err := op.run(); err == nil {
		t.Errorf("err = nil, want an error explaining that the path is invalid")
	}
}

// tests the case where the createTemp method returns a error.
func TestCreateTempError(t *testing.T) {
	t.Chdir(path.Join("testdata", "t"))

	op := newTests(1 * time.Second).(tests)
	errorMsg := "CreateTemp: error"
	op.os.createTemp = func(dir, pattern string) (*os.File, error) {
		return nil, errors.New("CreateTemp: error")
	}
	op.os.remove = nil

	if _, err := op.run(); err == nil {
		t.Errorf("err = nil")
	} else if err.Error() != errorMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errorMsg)
	}
}

// tests the case where the "go test" command generates a result that is not JSON. To do this
// we replace the go command by another that dont generates JSON.
func TestCmdNoJson_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "nojson"))

	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

	// to handle the case where an error in a previous execution of the tests resulted
	// in the executable remaining in the folder.
	if err := exec.Command(goPath, "clean").Run(); err != nil {
		panic(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	if err := exec.Command(goPath, "build", "go.go").Run(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := exec.Command(goPath, "clean").Run(); err != nil {
			panic(err)
		}
	})

	op := newTests(1 * time.Millisecond)
	if _, err := op.run(); err == nil {
		t.Error("err = nil")
	}
}

// test the case where the opening of the coverprofile returns an error.
func TestCoverProfileOpenError_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	op := newTests(1 * time.Millisecond).(tests)
	errMsg := "Open: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return nil, errors.New(errMsg)
	}

	_, err := op.run()
	if err == nil {
		t.Error("err = nil")
	} else if err.Error() != errMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errMsg)
	}
}

type readerError func(p []byte) (n int, err error)

func (r readerError) Read(p []byte) (n int, err error) {
	return r(p)
}

// test the case where the reading of the cover profile returns an error.
func TestScannerError_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	op := newTests(1 * time.Millisecond).(tests)
	errMsg := "scanner: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return io.NopCloser(readerError(func(p []byte) (n int, err error) {
			return 0, errors.New(errMsg)
		})), nil
	}

	_, err := op.run()
	if err == nil {
		t.Error("err = nil")
	} else if err.Error() != errMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errMsg)
	}
}

// test the case where the packages of dir are only of tests. To do this we run packages.Load
// and removes the packages that are not of test from the result of Load.
func TestPkgPath(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	cfg := &packages.Config{
		Context: t.Context(),
		Mode:    packages.NeedName | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests:   true,
	}
	pkgs, err := packages.Load(cfg, "."+string(os.PathSeparator)+"...")
	if err != nil {
		return
	}

	var testOnly []*packages.Package
	for i := range pkgs {
		if strings.HasSuffix(pkgs[i].PkgPath, "_test") || strings.HasSuffix(pkgs[i].PkgPath, ".test") {
			testOnly = append(testOnly, pkgs[i])
		}
	}

	tat := newThereAreTests()
	pp := tat.pkgPath(testOnly)
	if pp != testOnly[0].PkgPath {
		t.Errorf("pkg path = %q, want %q", pp, testOnly[0].PkgPath)
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

	tat := newThereAreTests()

	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if tat.isTestFunction(info, fd) {
			t.Errorf("got %s is a test function, want that it is not", fd.Name.Name)
		}
	}
}

// TestStaticcheckNotFoundInTheSystem tests the case where the staticcheck was not installed
// in the system. For making this happens we change the PATH environment
// variable to a directory that hasn't the staticcheck command.
func TestStaticcheckNotFoundInTheSystem(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	op := newStaticcheck().(staticcheck)
	if cmd, err := op.installedInTheSystem(); cmd != nil || err != nil {
		t.Errorf("cmd = %v and err == %q, want cmd = <nil> and error = <nil>", cmd, err)
	}
}

// TestStaticcheckLookPathError tests the case where the exec.LookPath("staticcheck") returns a error
// that isn't exec.ErrNotFound. For making this happens we create a staticcheck command in the current
// directory and change the PATH variable to not find the correct staticcheck command. With this exec.LookPath will
// return exec.ErrDot.
func TestStaticcheckLookPathError(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// change the current dir to where the fake staticcheck will be.
	t.Chdir(path.Join("testdata", "staticcheckcmd"))

	// create a fake staticcheck command.
	createExecutable(t, "staticcheck.go")

	// note that we dont include testdata/staticcheckcmd in the PATH because if we do that then LookPath will not
	// return ErrDot.
	t.Setenv("PATH", path.Join(dir, "testdata"))

	op := newStaticcheck().(staticcheck)
	if _, err := op.installedInTheSystem(); !errors.Is(err, exec.ErrDot) {
		t.Errorf("want err = exec.ErrDot, found %v", err)
	}
}

// TestStaticcheckCommandError tests the case where s.command returns a error. For making this happens
// we change PATH to point to a directory that not contains the commands go and staticcheck.
func TestStaticcheckCommandError(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(path.Join("testdata", "fusptop"))
	t.Setenv("PATH", path.Join(dir, "testdata", "testfail"))

	op := newStaticcheck()
	if _, err := op.run(); err == nil {
		t.Error("want err != nil, found nil")
	}
}

// TestStaticcheckWithoutTestsError tests the case where run returns a error due to
// withTest or withoutTest returning a error.
func TestStaticcheckError(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	op := newStaticcheck().(staticcheck)
	prevNewCmd := op.os.newCmd
	errorMsg := "failed to execute command"
	op.os.newCmd = func(stdout, stderr io.Writer, name string, args ...string) command {
		if args[0] == "tool" { // running "go tool"
			return prevNewCmd(stdout, stderr, name, args...)
		}
		return cmd(func() error { return errors.New(errorMsg) })
	}
	if _, err := op.run(); err == nil {
		t.Errorf("err == nil")
	} else if err.Error() != "\tlan: from staticcheck\n"+errorMsg {
		t.Errorf("want error.Error() == %s, got %s", errorMsg, err.Error())
	}
}

type cmd func() error

func (c cmd) Run() error {
	return c()
}

// createExecutable run "go build "+goFileName to create a executable in the current directory and uses Cleanup
// to remove it.
func createExecutable(t *testing.T, goFileName string) {
	cmd := exec.Command("go", "build", goFileName)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := exec.Command("go", "clean").Run(); err != nil {
			panic(err)
		}
	})
}

// cleanTestCache cleans the test cache of go.
func cleanTestCache(t *testing.T) {
	if err := exec.Command("go", "clean", "-testcache").Run(); err != nil {
		t.Fatal(err)
	}
}
