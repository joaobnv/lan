package main

import (
	"bytes"
	"os"
	"os/exec"
	"path"
	"testing"
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

func TestTestsFail(t *testing.T) {
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

func TestTestsOk(t *testing.T) {
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

func TestCmdRunFail(t *testing.T) {
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

func TestCmdNoJson(t *testing.T) {
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
