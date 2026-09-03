package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestMainFunc(t *testing.T) {
	discard, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer discard.Close()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	t.Cleanup(func() { os.Stdout = stdout })
	os.Stdout = discard

	t.Run("deny", func(t *testing.T) {
		dirPath := filepath.Join(wd, "testdata", "cases", "build", "case00")
		t.Chdir(dirPath)

		var exitCode int
		exit = func(code int) {
			exitCode = code
		}

		main()
		if exitCode != 1 {
			t.Errorf("exit code = %d", exitCode)
		}
	})
	t.Run("allow", func(t *testing.T) {
		dirPath := filepath.Join(wd, "testdata", "cases", "tests", "case00")
		t.Chdir(dirPath)

		var exitCode int
		exit = func(code int) {
			exitCode = code
		}

		main()
		if exitCode != 0 {
			t.Errorf("exit code = %d", exitCode)
		}
	})
}

func TestOsOpen(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dirPath := filepath.Join(wd, "testdata", "cases", "build", "case00")

	tp := newTestsPkg(0, wd, "")
	_, err = tp.messageFromCoverProfile(filepath.Join(dirPath, "covnonexists.txt"))
	if err == nil {
		t.Error("err == nil")
	}
}

func TestMust(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		var l logger
		err := errors.New("err")
		mustLogger(err, &l)
		want := fmt.Sprintf("\tlan: internal error:\n%s\n"+
			"please, consider reporting this as a Github issue on the Lan repository\n"+
			"\tlan version: %s", err.Error(), version)
		if string(l) != want {
			t.Errorf("got %q", l)
		}
	})

	t.Run("ok", func(t *testing.T) {
		var l logger
		mustOkLogger(false, &l)
		want := fmt.Sprintf("\tlan: internal error\n"+
			"please, consider reporting this as a Github issue on the Lan repository\n"+
			"\tlan version: %s", version)
		if string(l) != want {
			t.Errorf("got %q", l)
		}
	})
}

type logger string

func (l *logger) Fatalf(format string, v ...any) {
	*l = logger(fmt.Sprintf(format, v...))
}

func TestDirs(t *testing.T) {
	var dirs []*testDir
	for _, dirPath := range dirsForTest(t) {
		dirs = append(dirs, unmarshalDir(t, dirPath))
	}
	for i := range dirs {
		_, testName, _ := strings.CutLast(dirs[i].path, "testdata")
		t.Run(testName, dirs[i].run)
	}
}

func dirsForTest(t *testing.T) (result []string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	testdataFS := os.DirFS(filepath.Join(wd, "testdata"))
	containsRes := func(path string) (bool, error) {
		subDirs, err := fs.ReadDir(testdataFS, path)
		if err != nil {
			return false, err
		}

		isRes := func(d fs.DirEntry) bool {
			return !d.IsDir() && d.Name() == "res.txt"
		}

		return slices.ContainsFunc(subDirs, isRes), nil
	}

	fs.WalkDir(testdataFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if contains, err := containsRes(path); !contains || err != nil {
			return err
		}
		result = append(result, filepath.Join(wd, "testdata", path))
		return fs.SkipDir
	})

	return
}

type testDir struct {
	path                   string
	ops                    []*opTest
	pathWithoutStaticcheck bool
}

func unmarshalDir(t *testing.T, path string) *testDir {
	d := &testDir{
		path: path,
	}
	options, dataOps := readRes(t, filepath.Join(path, "res.txt"))
	if options != "" {
		unmarshalOptions(t, d, options)
	}
	for i := range dataOps {
		d.ops = append(d.ops, unmarshallOpTest(t, path, dataOps[i]))
	}
	return d
}

func readRes(t *testing.T, path string) (string, []string) {
	var b strings.Builder
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = io.Copy(&b, f); err != nil {
		t.Fatal(err)
	}
	data := b.String()

	var options string
	if strings.HasPrefix(data, "(") {
		ind := strings.IndexRune(data, ')')
		if ind < 0 {
			t.Fatal("missing ')'")
		}
		options = data[:ind+1]
		data = data[ind+1:]
	}
	var dataOps []string
	for _, dataOp := range strings.Split(data, "lan: ") {
		dataOp = strings.TrimRight(dataOp, " \t")
		if strings.TrimSpace(dataOp) == "" {
			continue
		}
		dataOps = append(dataOps, "\tlan: "+dataOp)
	}

	return options, dataOps
}

func unmarshalOptions(t *testing.T, d *testDir, line string) {
	options := strings.TrimPrefix(line, "(")
	options = strings.TrimSuffix(options, ")")

	for _, option := range strings.Split(options, ",") {
		option = strings.TrimSpace(option)
		switch option {
		case "path without staticcheck":
			d.pathWithoutStaticcheck = true
		default:
			t.Fatalf("unknown option %q, near %q", option, line)
		}
	}
}

func (d *testDir) run(t *testing.T) {
	if d.pathWithoutStaticcheck {
		d.removeStaticcheckFromPATH(t)
	} else {
		t.Parallel()
	}

	cfg := runConfig{workDir: d.path, testTimeout: d.testTimeout(), waitFunc: (*opGroup).waitAll}
	if d.tempDirDoesNotExists() {
		cfg.tmpDir = filepath.Join(d.path, "tmpdir")
	}
	denyGit, message := run(cfg)
	if strings.TrimRightFunc(message, unicode.IsSpace) != message {
		t.Errorf("whitespace at end\n%q", message)
	}
	if err := d.check(denyGit, message); err != nil {
		t.Error(err)
	}
}

func (d *testDir) removeStaticcheckFromPATH(t *testing.T) {
	p := os.Getenv("PATH")

	staticcheckPath, err := exec.LookPath("staticcheck")
	if err != nil {
		t.Fatal(err)
	}

	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

	staticcheckPath = filepath.Dir(staticcheckPath)
	goPath = filepath.Dir(goPath)

	if staticcheckPath == goPath {
		t.Fatal("staticcheck path = Go path")
	}

	p = strings.Replace(p, staticcheckPath+string(os.PathListSeparator), "", 1)
	p = strings.TrimSuffix(p, staticcheckPath+string(os.PathSeparator))
	p = strings.TrimSuffix(p, staticcheckPath)

	t.Setenv("PATH", p)
}

func (d *testDir) check(denyGit bool, message string) error {
	wantDenyGit := d.wantDenyGit()
	if !denyGit && wantDenyGit {
		return fmt.Errorf("deny git == %v, want %v", denyGit, wantDenyGit)
	}

	if !denyGit {
		return nil
	}

	parts := strings.Split(message, "\tlan: ")
	parts = slices.DeleteFunc(parts, func(s string) bool { return s == "" })
	for i, op := range d.ops {
		if i >= len(parts) {
			break
		}
		message := "\tlan: " + parts[i]
		message = strings.TrimRight(message, "\n")
		if err := op.check(message); err != nil {
			return err
		}
	}

	if len(parts) > len(d.ops) {
		var m string
		for _, p := range parts[len(d.ops):] {
			m += "\tlan: " + p
		}
		return errors.New("unexpected:\n" + m)
	}

	if len(parts) < len(d.ops) {
		return d.ops[len(parts)].check("\tlan: " + parts[len(parts)-1])
	}

	return nil
}

func (d *testDir) wantDenyGit() bool {
	for _, ot := range d.ops {
		if len(ot.regs) > 0 {
			return true
		}
	}
	return false
}

func (d *testDir) testTimeout() (dur time.Duration) {
	for _, ot := range d.ops {
		if ot.testTimeout != 0 {
			return ot.testTimeout
		}
	}
	return
}

func (d *testDir) tempDirDoesNotExists() bool {
	for _, ot := range d.ops {
		if ot.tempDirDoesNotExists {
			return true
		}
	}
	return false
}

type opTest struct {
	dir   string
	head  string
	lines []string
	regs  []*regexp.Regexp

	all                  bool
	ordered              bool
	regexpQuantifier     string
	testTimeout          time.Duration
	tempDirDoesNotExists bool
}

func unmarshallOpTest(t *testing.T, dir string, data string) *opTest {
	ot := &opTest{dir: dir}
	firstLine, data, _ := strings.Cut(data, "\n")
	ot.unmarshalFirstLine(t, firstLine)
	ot.unmarshalRegexps(t, data)
	return ot

}

func (op *opTest) unmarshalFirstLine(t *testing.T, line string) {
	op.all = true
	op.ordered = true
	op.regexpQuantifier = "all"

	line = strings.TrimRight(line, " \t\r")

	ind := strings.IndexRune(line, '(')
	if ind == -1 {
		op.setHead(t, line)
		return
	}
	op.setHead(t, line[:ind])

	options, after, _ := strings.CutLast(line[ind+len("("):], ")")
	if after != "" {
		t.Fatalf("invalid head: text after ')': %q", after)
	}

	for _, option := range strings.Split(options, ",") {
		option = strings.TrimSpace(option)
		switch option {
		case "temp dir does not exists":
			op.tempDirDoesNotExists = true
		default:
			if strings.HasPrefix(option, "timeout=") {
				_, d, _ := strings.Cut(option, "=")
				timeout, err := time.ParseDuration(d)
				if err != nil {
					t.Fatalf("%s: %s", option, err.Error())
				}
				op.testTimeout = timeout
			} else {
				t.Fatalf("unknown option %q, near %q", option, line)
			}
		}
	}
}

func (op *opTest) setHead(t *testing.T, h string) {
	if !validHead(h) {
		t.Fatalf("invalid head: %q", h)
	}
	op.head = h
}

// they are ordered from the first the can ocurr.
var heads = []string{
	"\tlan: from building",
	"\tlan: error",
	"\tlan: from cover profile",
	"\tlan: from staticcheck",
	"\tlan: from executing the tests",
	"\tlan: from checking if there are tests",
	"\tlan: from go vet",
}

func validHead(head string) bool {
	return slices.Contains(heads, head)
}

func (ot *opTest) unmarshalRegexps(t *testing.T, data string) {
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		ot.lines = append(ot.lines, line)

		reg, err := regexp.Compile(line)
		if err != nil {
			t.Fatal(err)
		}

		ot.regs = append(ot.regs, reg)
	}
}

func (ot *opTest) check(message string) error {
	var lines []string
	for line := range strings.Lines(message) {
		lines = append(lines, strings.TrimRight(line, "\n"))
	}

	if len(lines) == 0 && len(ot.regs) == 0 {
		return nil
	}

	if len(lines) == 0 && len(ot.regs) > 0 {
		return ot.createError("expecting at least 1 line", lines)
	}

	if len(ot.regs) == 0 {
		return ot.createError("expecting 0 lines", lines)
	}

	head := strings.TrimRight(lines[0], " \t\r")
	body := lines[1:]

	if head != ot.head {
		return ot.createError("invalid head", lines)
	}

	if len(body) != len(ot.regs) {
		return ot.createError("expecting at least 2 body", lines)
	}

	if err := ot.checkRegexps(body); err != nil {
		return ot.createError(err.Error(), lines)
	}

	return nil
}

func (ot *opTest) createError(msg string, gotLines []string) error {
	expected := []string{ot.head}
	expected = append(expected, ot.lines...)

	b := new(bytes.Buffer)
	b.WriteString(msg + "\n")
	ot.writeExpectedAndGot(b, expected, gotLines)

	return errors.New(b.String())
}

func (ot *opTest) checkRegexps(bodyLines []string) error {
	var regexpsMatched []int
	var linesMatched []int

	for regIndex, reg := range ot.regs {
		if slices.Contains(regexpsMatched, regIndex) {
			continue
		}
		for lineIndex, line := range bodyLines {
			if slices.Contains(linesMatched, lineIndex) {
				continue
			}
			line = strings.TrimSpace(line)
			if reg.MatchString(line) {
				regexpsMatched = append(regexpsMatched, regIndex)
				linesMatched = append(linesMatched, lineIndex)
				break
			}
		}
	}

	ordered := slices.IsSorted(linesMatched)
	allLines := len(linesMatched) == len(bodyLines)
	allRegexps := len(regexpsMatched) == len(ot.regs)
	someRegexp := len(regexpsMatched) > 0

	var msgs []string
	if ot.ordered && !ordered {
		msgs = append(msgs, "not ordered")
	}
	if ot.all && !allLines {
		msgs = append(msgs, "not all lines")
	}
	if ot.regexpQuantifier == "all" && !allRegexps {
		msgs = append(msgs, "not all regexps")
	}
	if ot.regexpQuantifier == "exists" && !someRegexp {
		msgs = append(msgs, "no regexp matched")
	}
	if len(msgs) > 0 {
		return errors.New(strings.Join(msgs, ", "))
	}

	return nil
}

func (ot *opTest) writeExpectedAndGot(b *bytes.Buffer, expected, got []string) {
	fmt.Fprintf(b, "EXPECTED\n%s\n", strings.Join(expected, "\n"))
	fmt.Fprintf(b, "GOT\n%s", strings.Join(got, "\n"))
}
