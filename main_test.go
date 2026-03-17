package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// Copyright (c) 2025, João Breno. See the license.

func TestGetcwdError(t *testing.T) {
	t.Cleanup(func() { defaultOS = newOperatingSystem() })
	t.Chdir(filepath.Join("testdata", "fusptop"))

	errMsg := "getwd error"
	defaultOS.stdout = new(strings.Builder)
	defaultOS.getwd = func() (string, error) {
		return "", errors.New(errMsg)
	}
	var exitCode int
	defaultOS.exit = func(code int) {
		exitCode = code
	}

	main()

	message := defaultOS.stdout.(*strings.Builder).String()
	if message != errMsg {
		t.Errorf("message = %q, want %q", message, errMsg)
	}

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

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
		{op: newBuild(wd)},
		{op: newTests(1*time.Second, wd)},
		{op: newVet(wd)},
		{op: newStaticcheck(wd)},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%T", c.op), func(t *testing.T) {
			if _, err := c.op.run(); err == nil {
				t.Errorf("run did not return a error")
			}
		})
	}

	denyGit, message := run(wd)
	if !denyGit || message == "" {
		t.Errorf("not deny git or message is empty")
	}
}

// tests the case where the "go test" command generates a result that is not JSON in the operation build.
// To do this we replace the go command by another that dont generates JSON.
func TestCmdNoJson_build(t *testing.T) {
	t.Chdir(path.Join("testdata", "nojson"))

	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

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

	op := newBuild(wd)
	if _, err := op.run(); err == nil {
		t.Error("err = nil")
	}
}

// tests the case where the path used by thereAreTests is invalid.
// In this case we expect that packages.Load returns an error.
func TestPackageLoadError(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	op := newTests(30*time.Second, filepath.Join(wd, "testdata", "fusptop")).(tests)
	// I think \x00 is not allowed in Linux and Windows.
	op.tat.packagesPath = "\x00<\\/>"
	if _, err := op.run(); err == nil {
		t.Errorf("err = nil, want an error explaining that the path is invalid")
	}
}

// tests the case where the createTemp method returns a error.
func TestCreateTempError(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	op := newTests(1*time.Second, filepath.Join(wd, "testdata", "t")).(tests)
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

// tests the case where the "go test" command generates a result that is not JSON in the operations tests.
// To do this we replace the go command by another that dont generates JSON.
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

	op := newTests(1*time.Millisecond, wd)
	if _, err := op.run(); err == nil {
		t.Error("err = nil")
	}
}

// test the case where the opening of the coverprofile returns an error.
func TestCoverProfileOpenError_tests(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	op := newTests(1*time.Millisecond, filepath.Join(wd, "testdata", "fusptop")).(tests)
	errMsg := "Open: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return nil, errors.New(errMsg)
	}

	if _, err = op.run(); err == nil {
		t.Error("err = nil")
	} else if err.Error() != errMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errMsg)
	}
}

// test the case where the reading of the cover profile returns an error.
func TestScannerError_tests(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	op := newTests(1*time.Millisecond, filepath.Join(wd, "testdata", "fusptop")).(tests)
	errMsg := "scanner: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return io.NopCloser(iotest.ErrReader(errors.New(errMsg))), nil
	}

	if _, err := op.run(); err == nil {
		t.Error("err = nil")
	} else if err.Error() != errMsg {
		t.Errorf("err.Error() = %q, want %q", err.Error(), errMsg)
	}
}

// TestStaticcheckNotFoundInTheSystem tests the case where the staticcheck was not installed
// in the system. For making this happens we change the PATH environment
// variable to a directory that hasn't the staticcheck command.
func TestStaticcheckNotFoundInTheSystem(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", wd)

	op := newStaticcheck(filepath.Join(wd, "testdata", "fusptop")).(staticcheck)
	if cmd, err := op.installedInTheSystem(); cmd != nil || err != nil {
		t.Errorf("cmd = %v and err == %q, want cmd = <nil> and error = <nil>", cmd, err)
	}
}

// TestStaticcheckLookPathError tests the case where the exec.LookPath("staticcheck") returns a error
// that isn't exec.ErrNotFound. For making this happens we create a staticcheck command in the current
// directory and change the PATH variable to not find the correct staticcheck command. With this exec.LookPath will
// return exec.ErrDot.
func TestStaticcheckLookPathError(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// change the current dir to where the fake staticcheck will be.
	t.Chdir(filepath.Join(wd, "testdata", "staticcheckcmd"))

	// create a fake staticcheck command.
	createExecutable(t, "staticcheck.go")

	// note that we dont include testdata/staticcheckcmd in the PATH because if we do that then LookPath will not
	// return ErrDot.
	t.Setenv("PATH", "."+string(os.PathListSeparator)+path.Join(wd, "testdata"))

	op := newStaticcheck(filepath.Join(wd, "testdata", "staticcheckcmd")).(staticcheck)
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

	op := newStaticcheck(filepath.Join(dir, "testdata", "fusptop"))
	if _, err := op.run(); err == nil {
		t.Error("want err != nil, found nil")
	}
}

// TestStaticcheckWithoutTestsError tests the case where run returns a error due to
// withTest or withoutTest returning a error.
func TestStaticcheckError(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	op := newStaticcheck(filepath.Join(wd, "testdata", "fusptop")).(staticcheck)
	prevNewCmd := op.os.newCmd
	errorMsg := "failed to execute command"
	op.os.newCmd = func(workDir string, stdout, stderr io.Writer, name string, args ...string) command {
		if args[0] == "tool" { // running "go tool"
			return prevNewCmd(workDir, stdout, stderr, name, args...)
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
