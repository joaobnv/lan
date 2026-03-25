package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
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

	opSys := newOperatingSystem()
	prevNewCmd := opSys.newCmd
	opSys.newCmd = func(ctx context.Context, workDir string, stdout, stderr io.Writer, name string, args ...string) command {
		if args[0] == "tool" { // running "go tool"
			return testCmd(func() error { return errors.New("go not found") })
		}
		return prevNewCmd(t.Context(), workDir, stdout, stderr, name, args...)
	}

	t.Setenv("PATH", wd)

	cases := []struct{ op operation }{
		{op: newBuild(wd, opSys)},
		{op: newTests(1*time.Second, wd, opSys)},
		{op: newVet(wd, opSys)},
		{op: newStaticcheck(wd, opSys)},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%T", c.op), func(t *testing.T) {
			if _, err := c.op.run(t.Context()); err == nil {
				t.Errorf("run did not return a error")
			}
		})
	}

	denyGit, message := run(runConfig{workDir: wd, os: opSys})
	if !denyGit || message == "" {
		t.Errorf("not deny git or message is empty")
	}
}

// test the case where a operation returns a error
func TestRunOpError(t *testing.T) {
	t.Chdir(path.Join("testdata", "fusptop"))

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	opSys := newOperatingSystem()
	prevNewCmd := opSys.newCmd
	opSys.newCmd = func(ctx context.Context, workDir string, stdout, stderr io.Writer, name string, args ...string) command {
		if args[0] == "tool" { // running "go tool"
			return testCmd(func() error { return errors.New("go not found") })
		}
		return prevNewCmd(t.Context(), workDir, stdout, stderr, name, args...)
	}

	denyGit, message := run(runConfig{workDir: wd, os: opSys})
	if !denyGit || message == "" {
		t.Errorf("not deny git or message is empty")
	}
}

func TestOpGroupError(t *testing.T) {
	t.Parallel()
	const N = 3

	var done []chan struct{}
	var msgs []string
	var errs []error
	var ops []op
	var numbers []int

	for n := range N {
		done = append(done, make(chan struct{}))
		msgs = append(msgs, "")
		errs = append(errs, nil)
		ops = append(ops, func(context.Context) (string, error) { <-done[n]; return msgs[n], errs[n] })
		numbers = append(numbers, n)
	}

	for errorsUntil := range N + 1 {
		for _, perm := range perms(numbers) {
			for n := range N {
				msgs[n] = ""
				errs[n] = nil
			}
			for _, n := range perm[:errorsUntil] {
				errs[n] = fmt.Errorf("error from op %d", n)
			}
			if errorsUntil > 0 {
				for n := 1 + slices.Min(perm[:errorsUntil]); n < N; n++ {
					msgs[n] = fmt.Sprintf("message from op %d", n)
				}
			}

			var wantedError error
			for n := range N {
				if errs[n] != nil {
					wantedError = errs[n]
					break
				}
			}

			eg := newOpGroup()
			for n := range N {
				eg.executeGo(t.Context(), ops[n])
			}

			go func() {
				for i, n := range perm {
					done[n] <- struct{}{}
					eg.done.L.Lock()
					for eg.numberEnded < i+1 {
						eg.done.Wait()
					}
					eg.done.L.Unlock()
				}
			}()

			_, err := eg.wait()
			if err != wantedError {
				t.Errorf("for {perm: %v, errorsUntil: %d}: got %q, want %q", perm, errorsUntil, err, wantedError)
			}

			// check for leaks. If has one, then this test will not end.
			eg.done.L.Lock()
			for eg.numberEnded != N {
				eg.done.Wait()
			}
			eg.done.L.Unlock()
		}
	}
}

func TestOpGroupMessages(t *testing.T) {
	t.Parallel()
	const N = 3

	var done []chan struct{}
	var msgs []string
	var errs []error
	var ops []op
	var numbers []int

	for n := range N {
		done = append(done, make(chan struct{}))
		msgs = append(msgs, "")
		errs = append(errs, nil)
		ops = append(ops, func(context.Context) (string, error) { <-done[n]; return msgs[n], errs[n] })
		numbers = append(numbers, n)
	}

	for messagesUntil := range N + 1 {
		for _, perm := range perms(numbers) {
			for n := range N {
				msgs[n] = ""
				errs[n] = nil
			}
			for _, n := range perm[:messagesUntil] {
				msgs[n] = fmt.Sprintf("message from op %d", n)
			}
			if messagesUntil > 0 {
				for n := 1 + slices.Min(perm[:messagesUntil]); n < N; n++ {
					errs[n] = fmt.Errorf("error from op %d", n)
				}
			}

			var wantedMessage string
			for n := range N {
				if msgs[n] != "" {
					wantedMessage = msgs[n]
					break
				}
			}

			eg := newOpGroup()
			for n := range N {
				eg.executeGo(t.Context(), ops[n])
			}

			go func() {
				for i, n := range perm {
					done[n] <- struct{}{}
					eg.done.L.Lock()
					for eg.numberEnded < i+1 {
						eg.done.Wait()
					}
					eg.done.L.Unlock()
				}
			}()

			msg, _ := eg.wait()
			if msg != wantedMessage {
				t.Errorf("for {perm: %v, errorsUntil: %d}: got %q, want %q", perm, messagesUntil, msg, wantedMessage)
			}

			// check for leaks. If has one, then this test will not end.
			eg.done.L.Lock()
			for eg.numberEnded != N {
				eg.done.Wait()
			}
			eg.done.L.Unlock()
		}
	}
}

func TestPkgOpGroupError(t *testing.T) {
	t.Parallel()
	const N = 3

	var done []chan struct{}
	var errs []error
	var ops []pkgOp
	var numbers []int

	for n := range N {
		done = append(done, make(chan struct{}))
		errs = append(errs, nil)
		ops = append(ops, func(context.Context, string) (string, error) { <-done[n]; return "", errs[n] })
		numbers = append(numbers, n)
	}

	for errorsUntil := range N + 1 {
		for _, doneOrder := range perms(numbers) {
			for n := range N {
				errs[n] = nil
			}
			for _, n := range doneOrder[:errorsUntil] {
				errs[n] = fmt.Errorf("error from op %d", n)
			}

			wantedError := errs[doneOrder[0]]

			eg := newPkgOpGroup()
			for n := range N {
				eg.executeGo(t.Context(), ops[n], "")
			}

			go func() {
				for i, n := range doneOrder {
					done[n] <- struct{}{}
					eg.done.L.Lock()
					for eg.numberEnded < i+1 {
						eg.done.Wait()
					}
					eg.done.L.Unlock()
				}
			}()

			_, err := eg.wait()
			if err != wantedError {
				t.Errorf("for {doneOrder: %v, errorsUntil: %d}: got %q, want %q", doneOrder, errorsUntil, err, wantedError)
			}

			// check for leaks. If has one, then this test will not end.
			eg.done.L.Lock()
			for eg.numberEnded != N {
				eg.done.Wait()
			}
			eg.done.L.Unlock()
		}
	}
}

func TestPkgOpGroupMessages(t *testing.T) {
	t.Parallel()
	const N = 3

	var done []chan struct{}
	var msgs []string
	var ops []pkgOp
	var numbers []int

	for n := range N {
		done = append(done, make(chan struct{}))
		msgs = append(msgs, "")
		ops = append(ops, func(context.Context, string) (string, error) { <-done[n]; return msgs[n], nil })
		numbers = append(numbers, n)
	}

	for messagesUntil := range N + 1 {
		for _, doneOrder := range perms(numbers) {
			for n := range N {
				msgs[n] = ""
			}
			for _, n := range doneOrder[:messagesUntil] {
				msgs[n] = fmt.Sprintf("message from op %d", n)
			}

			wantedMessage := msgs[doneOrder[0]]

			eg := newPkgOpGroup()
			for n := range N {
				eg.executeGo(t.Context(), ops[n], "")
			}

			go func() {
				for i, n := range doneOrder {
					done[n] <- struct{}{}
					eg.done.L.Lock()
					for eg.numberEnded < i+1 {
						eg.done.Wait()
					}
					eg.done.L.Unlock()
				}
			}()

			msg, _ := eg.wait()
			if msg != wantedMessage {
				t.Errorf("for {doneOrder: %v, messagesUntil: %d}: got %q, want %q", doneOrder, messagesUntil, msg, wantedMessage)
			}

			// check for leaks. If has one, then this test will not end.
			eg.done.L.Lock()
			for eg.numberEnded != N {
				eg.done.Wait()
			}
			eg.done.L.Unlock()
		}
	}
}

type op func(ctx context.Context) (message string, err error)

func (o op) run(ctx context.Context) (message string, err error) {
	return o(ctx)
}

type pkgOp func(ctx context.Context, pkg string) (message string, err error)

func (po pkgOp) run(ctx context.Context, pkg string) (message string, err error) {
	return po(ctx, pkg)
}

func perms(a []int) (ps [][]int) {
	if len(a) <= 1 {
		ps = append(ps, slices.Clone(a))
		return
	}

	n := len(a) - 1
	a = slices.Clone(a)
	for i := range a {
		a[n], a[i] = a[i], a[n]
		for _, p := range perms(a[:n]) {
			p = append(p, a[n])
			ps = append(ps, p)
		}
		a[n], a[i] = a[i], a[n]
	}

	slices.SortFunc(ps, slices.Compare)

	return ps
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

	op := newBuild(wd, newOperatingSystem())
	if _, err := op.run(t.Context()); err == nil {
		t.Error("err = nil")
	}
}

func TestBuildCmdError(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		output   string
		runError error
	}{
		{output: "<Language>Go</Language>"},
		{runError: io.EOF},
	}

	for _, c := range cases {
		b := newBuild(wd, newOperatingSystem()).(build)
		b.os.newCmd = func(ctx context.Context, worDir string, stdout, stderr io.Writer, name string, args ...string) command {
			return testCmd(func() error { io.WriteString(stdout, c.output); return c.runError })
		}

		if _, err = b.run(t.Context()); err == nil {
			t.Error("err == nil")
		}
	}
}

// tests the case where the path used by thereAreTests is invalid.
// In this case we expect that packages.Load returns an error.
func TestThereAreTestsPackageLoadError(t *testing.T) {
	t.Parallel()

	// I think \x00 is not allowed in Linux and Windows.
	op := newThereAreTests("\x00<\\/>", newOperatingSystem()).(thereAreTests)
	if _, err := op.run(t.Context()); err == nil {
		t.Errorf("err = nil, want an error explaining that the path is invalid")
	}
}

func TestThereAreTestsRunError(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	b := newThereAreTests(wd, newOperatingSystem()).(thereAreTests)
	b.os.newCmd = func(ctx context.Context, worDir string, stdout, stderr io.Writer, name string, args ...string) command {
		return testCmd(func() error { return io.EOF })
	}

	if _, err = b.run(t.Context()); err == nil {
		t.Error("err == nil")
	}
}

// tests the case where the createTemp method returns a error.
func TestCreateTempError(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	op := newTests(1*time.Second, wd, newOperatingSystem()).(tests)
	errorMsg := "CreateTemp: error"
	op.os.createTemp = func(dir, pattern string) (*os.File, error) {
		return nil, errors.New("CreateTemp: error")
	}
	op.os.remove = nil

	if _, err := op.run(t.Context()); err == nil {
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
	op := newTests(1*time.Millisecond, wd, newOperatingSystem())
	if _, err := op.run(t.Context()); err == nil {
		t.Error("err = nil")
	}
}

// tests the case where the "go test" command generates a result that is not JSON, in
// the operation thereAreTests. To do this we replace the go command by another that
// dont generates JSON.
func TestCmdNoJson_thereAreTests(t *testing.T) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(path.Join("testdata", "nojson"))

	// to handle the case where an error in a previous execution of the tests resulted
	// in the executable remaining in the folder.
	if err := exec.Command(goPath, "clean").Run(); err != nil {
		t.Fatal(err)
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

	pkgOp := newThereAreTests(wd, newOperatingSystem()).(thereAreTests)
	if _, err = pkgOp.has(t.Context()); err == nil {
		t.Error("err == nil")
	}
}

// test the case where the opening of the coverprofile returns an error.
func TestCoverProfileOpenError_tests(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wd = filepath.Join(wd, "testdata", "fusptop")

	op := newTests(1*time.Millisecond, wd, newOperatingSystem()).(tests)
	errMsg := "Open: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return nil, errors.New(errMsg)
	}

	if _, err = op.run(t.Context()); err == nil {
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
	wd = filepath.Join(wd, "testdata", "fusptop")

	op := newTests(1*time.Millisecond, wd, newOperatingSystem()).(tests)
	errMsg := "scanner: error"
	op.os.open = func(name string) (io.ReadCloser, error) {
		return io.NopCloser(iotest.ErrReader(errors.New(errMsg))), nil
	}

	if _, err := op.run(t.Context()); err == nil {
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

	op := newStaticcheck(filepath.Join(wd, "testdata", "fusptop"), newOperatingSystem()).(staticcheck)
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

	op := newStaticcheck(filepath.Join(wd, "testdata", "staticcheckcmd"), newOperatingSystem()).(staticcheck)
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
	dir = filepath.Join(dir, "testdata", "fusptop")

	t.Chdir(filepath.Join("testdata", "fusptop"))
	t.Setenv("PATH", filepath.Join(dir, "testdata", "testfail"))

	op := newStaticcheck(dir, newOperatingSystem())

	if _, err := op.run(t.Context()); err == nil {
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
	wd = filepath.Join(wd, "testdata", "fusptop")

	op := newStaticcheck(wd, newOperatingSystem()).(staticcheck)
	prevNewCmd := op.os.newCmd
	errorMsg := "failed to execute command"
	op.os.newCmd = func(ctx context.Context, workDir string, stdout, stderr io.Writer, name string, args ...string) command {
		if args[0] == "tool" { // running "go tool"
			return prevNewCmd(t.Context(), workDir, stdout, stderr, name, args...)
		}
		return testCmd(func() error { return errors.New(errorMsg) })
	}

	if _, err := op.run(t.Context()); err == nil {
		t.Errorf("err == nil")
	} else if err.Error() != "\tlan: from staticcheck\n"+errorMsg {
		t.Errorf("want error.Error() == %s, got %s", errorMsg, err.Error())
	}
}

type testCmd func() error

func (c testCmd) Run() error {
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
