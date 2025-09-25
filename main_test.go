package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bytes"
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

// tests whether the failure of tests is detected.
func TestMainTestsFail(t *testing.T) {
	t.Chdir(path.Join("testdata", "testfail"))
	var exitCode int

	defaultOS.stdout = new(bytes.Buffer)
	defaultOS.exit = func(code int) {
		exitCode = code
	}

	main()

	expectedMessage := "\tfrom executing the tests\ntestfail: TestSum failed\n\n"
	if r := defaultOS.stdout.(*bytes.Buffer).String(); r != expectedMessage {
		t.Errorf("output = %q, want %q", r, expectedMessage)
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

// tests whether syntax errors in the code are detected.
func TestMainTestsStderr(t *testing.T) {
	t.Chdir(path.Join("testdata", "stderr"))

	var exitCode int

	defaultOS.stdout = new(bytes.Buffer)
	defaultOS.exit = func(code int) {
		exitCode = code
	}

	main()

	if defaultOS.stdout.(*bytes.Buffer).Len() == 0 {
		t.Errorf("without stderr")
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

// tests the case where a call to cmd.Run() returns an error.
// To make it return an error we change the PATH environment variable to a directory that don't have
// the go command.
func TestCmdRunError(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	cases := []struct{ op operation }{
		{op: newVet()},
		{op: newTests(1 * time.Second)},
		{op: newThereAreTests()},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%T", c.op), func(t *testing.T) {
			if _, pos, err := c.op.run(10); err == nil {
				t.Errorf("run did not return a error")
			} else if pos != 10 {
				t.Errorf("position (%d) != pos (%d)", 10, pos)
			}
		})
	}
}

// tests the case where a timeout occurs in the tests.
func TestTestsTimeout(t *testing.T) {
	t.Chdir(path.Join("testdata", "timeout"))

	cleanTestCache(t)

	op := newTests(100 * time.Nanosecond)
	message, pos, err := op.run(12)
	if err != nil {
		t.Fatal(err)
	} else if pos != 12 {
		t.Errorf("position (%d) != pos (%d)", 12, pos)
	}

	expectedMessage := "\tfrom executing the tests\ntimeout: panic: test timed out after 100ns\n\n"
	if message != expectedMessage {
		t.Errorf("message = %q, want %q", message, expectedMessage)
	}
}

// tests the case where the path used by thereAreTests is invalid.
// In this case we expect that packages.Load returns an error.
func TestPackageLoadError(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	op := newThereAreTests().(thereAreTests) // I think \x00 is not allowed in Linux and Windows.
	op.packagesPath = "\x00<\\/>"
	if _, pos, err := op.run(22); err == nil {
		t.Errorf("err = nil, want an error explaining that the path is invalid")
	} else if pos != 22 {
		t.Errorf("position (%d) != pos (%d)", 22, pos)
	}
}

// tests the case where the createTemp method returns a error.
func TestCreateTempError(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	op := newTests(1 * time.Second).(tests)
	errorMsg := "CreateTemp: error"
	op.os.createTemp = func(dir, pattern string) (*os.File, error) {
		return nil, errors.New("CreateTemp: error")
	}
	op.os.remove = nil

	if _, pos, err := op.run(1); err == nil {
		t.Errorf("err = nil")
	} else if err.Error() != errorMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errorMsg)
	} else if pos != 1 {
		t.Errorf("position (%d) != pos (%d)", 1, pos)
	}
}

// tests the case where the "go test" command generates a result that is not JSON. To do this
// we replace the go command by another that dont generates JSON.
func TestCmdNoJson_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "nojson"))

	createExecutable(t, "go.go")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	op := newTests(1 * time.Millisecond)
	if _, pos, err := op.run(2); err == nil {
		t.Error("err = nil")
	} else if pos != 2 {
		t.Errorf("position (%d) != pos (%d)", 2, pos)
	}
}

// test the reading of the cover profile.
func TestCoverProfile_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	op := newTests(1 * time.Millisecond).(tests)
	op.os.open = func(name string) (io.ReadCloser, error) {
		return os.Open(path.Join("..", "coverprofile.txt"))
	}

	msg, pos, err := op.run(3)
	if err != nil {
		t.Fatal(err)
	} else if pos != 3 {
		t.Errorf("position (%d) != pos (%d)", 3, pos)
	}

	expectedMessage := "\tfrom cover profile\n0"
	if msg != expectedMessage {
		t.Errorf("message = %q, want %q", msg, expectedMessage)
	}
}

// test the case where the opening of the coverprofile returns an error.
func TestCoverProfileOpenError_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	op := newTests(1 * time.Millisecond).(tests)
	errMsg := "Open: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return nil, errors.New(errMsg)
	}

	_, pos, err := op.run(4)
	if err == nil {
		t.Error("err = nil")
	} else if err.Error() != errMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errMsg)
	} else if pos != 4 {
		t.Errorf("position (%d) != pos (%d)", 4, pos)
	}
}

type readerError func(p []byte) (n int, err error)

func (r readerError) Read(p []byte) (n int, err error) {
	return r(p)
}

// test the case where the reading of the cover profile returns an error.
func TestScannerError_tests(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

	op := newTests(1 * time.Millisecond).(tests)
	errMsg := "scanner: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return io.NopCloser(readerError(func(p []byte) (n int, err error) {
			return 0, errors.New(errMsg)
		})), nil
	}

	_, pos, err := op.run(5)
	if err == nil {
		t.Error("err = nil")
	} else if err.Error() != errMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errMsg)
	} else if pos != 5 {
		t.Errorf("position (%d) != pos (%d)", 5, pos)
	}
}

// test the case where the packages of dir are only of tests. To do this we run packages.Load
// and removes the packages that are not of test from the result of Load.
func TestPkgPath(t *testing.T) {
	t.Chdir(path.Join("testdata", "testok"))

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

	tat := newThereAreTests().(thereAreTests)
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

	tat := newThereAreTests().(thereAreTests)

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

func TestMessage(t *testing.T) {
	cases := []struct {
		name            string
		dir             string
		op              operation
		expectedMessage string
	}{
		{
			// the tests are ok.
			name:            "testsOk",
			dir:             path.Join("testdata", "testok"),
			op:              newTests(30 * time.Second),
			expectedMessage: "",
		}, {
			// coverage is not 100.0%.
			name:            "testsNoCoverage",
			dir:             path.Join("testdata", "nocoverage"),
			op:              newTests(30 * time.Second),
			expectedMessage: "\tfrom executing the tests\nnocoverage: test coverage is not 100.0%\n",
		}, {
			// the tests of the package are in the package pkg_test,
			// this is, in another package.
			name:            "ThereAreTestsWithTestsInAnotherPackageOk",
			dir:             path.Join("testdata", "testsinpkg"),
			op:              newThereAreTests(),
			expectedMessage: "",
		}, {
			// the package hasn't tests.
			name:            "WithoutTests",
			dir:             path.Join("testdata", "notests"),
			op:              newThereAreTests(),
			expectedMessage: "\tfrom checking if there are tests\nnotests has no tests\n",
		}, {
			// the package hasn't function declarations, so it not need tests.
			name:            "noneedTests",
			dir:             path.Join("testdata", "noneedtests"),
			op:              newThereAreTests(),
			expectedMessage: "",
		}, {
			// the package hasn't test functions, but has fuzz functions.
			name:            "fuzztest",
			dir:             path.Join("testdata", "testfuzz"),
			op:              newThereAreTests(),
			expectedMessage: "",
		},
	}

	position := 22
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(c.dir)

			message, pos, err := c.op.run(position)
			if err != nil {
				t.Fatal(err)
			} else if pos != position {
				t.Errorf("position (%d) != pos (%d)", position, pos)
			}

			if message != c.expectedMessage {
				t.Errorf("message = %q, want %q", message, c.expectedMessage)
			}
		})
	}
}

func TestNotOkIgnoringExactMessageContent(t *testing.T) {
	cases := []struct {
		name              string
		dir               string
		op                operation
		messageExplaining string
	}{
		{
			// the vet is called but the package has syntax errors.
			name:              "VetWithSyntaxError",
			dir:               path.Join("testdata", "syntaxerror"),
			op:                newVet(),
			messageExplaining: "the syntax error",
		}, {
			// the package has a suspicious construct that will be reported by the printf checker.
			name:              "VetFail",
			dir:               path.Join("testdata", "printfvet"),
			op:                newVet(),
			messageExplaining: "wrong arg type for %%d",
		},
	}

	position := 2
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(c.dir)

			message, pos, err := c.op.run(position)
			if err != nil {
				t.Fatal(err)
			} else if pos != position {
				t.Errorf("position (%d) != pos (%d)", position, pos)
			}

			if message == "" {
				t.Errorf("message = \"\", want a message explaining %s", c.messageExplaining)
			}
		})
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
		fileNameWithoutExtension := strings.TrimSuffix(goFileName, ".go")
		if fileExists(fileNameWithoutExtension) { // in Linux the executable hasn't extension.
			os.Remove(fileNameWithoutExtension)
		} else if fileExists(fileNameWithoutExtension + ".exe") { // in Windows the executable has the .exe extension.
			os.Remove(fileNameWithoutExtension + ".exe")
		}
	})
}

// fileExists reports whether a file, with the given name, exists in the current directory.
func fileExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return !errors.Is(err, os.ErrNotExist)
}

// cleanTestCache cleans the test cache of go.
func cleanTestCache(t *testing.T) {
	if err := exec.Command("go", "clean", "-testcache").Run(); err != nil {
		t.Fatal(err)
	}
}
