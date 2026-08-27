package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"
)

func TestDirsMainPT(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dirPath := filepath.Join(wd, "testdata", "pt")

	d, err := unmarshalDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(dirPath)
	defaultOS.stdout = &bytes.Buffer{}
	t.Cleanup(func() { defaultOS.stdout = os.Stdout })
	var exitCode int
	defaultOS.exit = func(code int) { exitCode = code }
	t.Cleanup(func() { defaultOS.exit = os.Exit })

	main()

	denyGit := exitCode != 0
	message := defaultOS.stdout.(*bytes.Buffer).String()

	message = strings.TrimSuffix(message, "\tlan version: "+version+"\n")
	message = strings.TrimRightFunc(message, unicode.IsSpace)

	if err := d.check(denyGit, message); err != nil {
		t.Error(err)
	}
}

func TestDirsMainSE(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dirPath := filepath.Join(wd, "testdata", "se")

	d, err := unmarshalDir(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Chdir(dirPath)
	defaultOS.stdout = &bytes.Buffer{}
	t.Cleanup(func() { defaultOS.stdout = os.Stdout })
	var exitCode int
	defaultOS.exit = func(code int) { exitCode = code }
	t.Cleanup(func() { defaultOS.exit = os.Exit })

	main()

	denyGit := exitCode != 0
	message := defaultOS.stdout.(*bytes.Buffer).String()

	message = strings.TrimSuffix(message, "\tlan version: "+version+"\n")
	message = strings.TrimRightFunc(message, unicode.IsSpace)

	if err := d.check(denyGit, message); err != nil {
		t.Error(err)
	}
}

func TestDirs(t *testing.T) {
	t.Parallel()

	for _, dirPath := range dirsForTest(t) {
		d, err := unmarshalDir(dirPath)
		if err != nil {
			t.Fatal(err)
		}

		t.Run(filepath.Base(d.path), d.testRun)
		t.Run(filepath.Base(d.path)+"/op", d.testOperations)
	}
}

func dirsForTest(t *testing.T) (result []string) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	testdataFS := os.DirFS(filepath.Join(wd, "testdata"))
	dirs, err := fs.ReadDir(testdataFS, ".")
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		subDirs, err := fs.ReadDir(testdataFS, d.Name())
		if err != nil {
			t.Fatal(err)
		}

		if slices.ContainsFunc(subDirs, func(e fs.DirEntry) bool { return e.Name() == "res" }) {
			result = append(result, filepath.Join(wd, "testdata", d.Name()))
		}
	}

	return
}

type fileTestDir struct {
	path          string
	fileForPrefix map[string]*file
	fileForName   map[string]*file
}

func unmarshalDir(path string) (d *fileTestDir, err error) {
	d = &fileTestDir{
		path:          path,
		fileForPrefix: map[string]*file{},
		fileForName:   map[string]*file{},
	}

	fileNames := []string{
		"building.txt", "cover_profile.txt", "staticcheck.txt",
		"tests.txt", "there_are_tests.txt", "vet.txt",
	}

	for _, fileName := range fileNames {
		filePath := filepath.Join(path, "res", fileName)
		if !fileExists(filePath) {
			continue
		}

		f, err := unmarshalFile(path, fileName)
		if err != nil {
			return nil, err
		}

		d.fileForPrefix[f.head] = f
		d.fileForName[fileName] = f
	}

	return
}

func (d *fileTestDir) testRun(t *testing.T) {
	t.Parallel()

	denyGit, message := run(runConfig{workDir: d.path, os: newOperatingSystem(), testTimeout: d.testTimeout()})
	if strings.TrimRightFunc(message, unicode.IsSpace) != message {
		t.Errorf("whitespace at end\n%q", message)
	}
	if err := d.check(denyGit, message); err != nil {
		t.Error(err)
	}
}

func (d *fileTestDir) check(denyGit bool, message string) error {
	wantDenyGit := d.wantDenyGit()
	if !denyGit {
		if wantDenyGit {
			return fmt.Errorf("deny git == %v, want %v", denyGit, wantDenyGit)
		}
		return nil
	}

	ind := strings.IndexRune(message, '\n')
	if ind == -1 {
		return fmt.Errorf("unexpected %q", message)
	}

	prefix := strings.TrimSpace(message[:ind])
	f := d.fileForPrefix[prefix]
	if f == nil {
		return fmt.Errorf("unknown prefix: %q\n%s", prefix, message)
	}

	return f.check(message)
}

func (d *fileTestDir) testOperations(t *testing.T) {
	t.Parallel()

	for _, f := range d.fileForName {
		testName, _, _ := strings.Cut(f.fileName, ".")
		t.Run(testName, f.testOp)
	}
}

func (d *fileTestDir) wantDenyGit() bool {
	for _, f := range d.fileForName {
		if len(f.regs) > 0 {
			return true
		}
	}
	return false
}

func (d *fileTestDir) testTimeout() (dur time.Duration) {
	f := d.fileForName["tests.txt"]
	if f != nil && f.testTimeout != dur {
		return f.testTimeout
	}

	f = d.fileForName["cover_profile.txt"]
	if f != nil && f.testTimeout != dur {
		return f.testTimeout
	}

	return
}

type file struct {
	dirPath  string
	fileName string

	head             string
	lines            []string
	regs             []*regexp.Regexp
	all              bool
	ordered          bool
	regexpQuantifier string
	testTimeout      time.Duration
}

func unmarshalFile(dirPath, fileName string) (f *file, err error) {
	filePath := filepath.Join(dirPath, "res", fileName)
	if !fileExists(filePath) {
		return nil, nil
	}

	f = &file{
		dirPath:          dirPath,
		fileName:         fileName,
		all:              true,
		ordered:          true,
		regexpQuantifier: "all",
	}

	if err = f.setHead(fileName); err != nil {
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	lines := slices.Collect(bytes.Lines(data))
	if len(lines) == 0 {
		return
	}

	trimmed := bytes.TrimSpace(lines[0])
	if bytes.HasPrefix(trimmed, []byte("(")) && bytes.HasSuffix(trimmed, []byte(")")) {
		if err = f.unmarshallOptions(trimmed); err != nil {
			return
		}
	} else {
		f.lines = append(f.lines, string(bytes.TrimRight(lines[0], "\r\t\n")))
	}

	for _, line := range lines[1:] {
		f.lines = append(f.lines, string(bytes.TrimRight(line, "\r\t\n")))
	}

	for _, line := range f.lines {
		reg, err := regexp.Compile(line)
		if err != nil {
			return nil, err
		}

		f.regs = append(f.regs, reg)
	}

	return
}

func (f *file) setHead(fileName string) error {
	switch fileName {
	case "building.txt":
		f.head = "lan: from building"
	case "error.txt":
		f.head = "lan: error"
	case "cover_profile.txt":
		f.head = "lan: from cover profile"
	case "staticcheck.txt":
		f.head = "lan: from staticcheck"
	case "tests.txt":
		f.head = "lan: from executing the tests"
	case "there_are_tests.txt":
		f.head = "lan: from checking if there are tests"
	case "vet.txt":
		f.head = "lan: from go vet"
	default:
		return fmt.Errorf("unknown file: %s", fileName)
	}
	return nil
}

func (f *file) unmarshallOptions(line []byte) error {
	line = line[utf8.RuneLen('(') : len(line)-utf8.RuneLen(')')]
	if len(line) == 0 {
		return nil
	}

	notAll := []byte("not all")
	unordered := []byte("unordered")
	someRegexp := []byte("some regexp")

	options := bytes.Split(line, []byte(","))
	for i := range options {
		option := bytes.TrimSpace(options[i])
		if bytes.Equal(option, notAll) {
			f.all = false
		} else if bytes.Equal(option, unordered) {
			f.ordered = false
		} else if bytes.Equal(option, someRegexp) {
			f.regexpQuantifier = "exists"
		} else if bytes.HasPrefix(option, []byte("timeout=")) {
			_, d, _ := bytes.Cut(option, []byte("="))
			timeout, err := time.ParseDuration(string(d))
			if err != nil {
				return fmt.Errorf("%s: %w", string(option), err)
			}
			f.testTimeout = timeout
		} else {
			return fmt.Errorf("unkonwn option %q, near %q", string(option), string(line))
		}
	}

	return nil
}

func (f *file) testOp(t *testing.T) {
	t.Parallel()

	message, err := f.op().run(t.Context())
	if err != nil {
		return
	}

	if strings.TrimRightFunc(message, unicode.IsSpace) != message {
		t.Errorf("whitespace at end\n%q", message)
	}
	if err = f.check(message); err != nil {
		t.Error(err)
	}
}

func (f *file) op() operation {
	switch f.fileName {
	case "building.txt":
		return newBuild(f.dirPath, newOperatingSystem())
	case "there_are_tests.txt":
		return newThereAreTests(f.dirPath, newOperatingSystem())
	case "vet.txt":
		return newVet(f.dirPath, newOperatingSystem())
	case "staticcheck.txt":
		return newStaticcheck(f.dirPath, newOperatingSystem())
	case "tests.txt":
		return newTests(f.testTimeout, f.dirPath, newOperatingSystem())
	case "cover_profile.txt":
		return newTests(f.testTimeout, f.dirPath, newOperatingSystem())
	default:
		panic(fmt.Errorf("unknown file: %s", f.fileName))
	}
}

func (f *file) check(message string) error {
	gotLines := slices.Collect(strings.Lines(message))
	if len(gotLines) == 0 {
		if len(f.regs) == 0 {
			return nil
		}
		return f.createError("expecting at least 1 line", gotLines)
	}

	if strings.TrimSpace(gotLines[0]) != f.head {
		if len(f.regs) == 0 {
			return nil
		}
		return f.createError("invalid head", gotLines)
	}

	if len(f.regs) == 0 {
		if len(gotLines) > 0 {
			return f.createError("expecting 0 lines", gotLines)
		}
		return nil
	}

	if len(gotLines) == 1 {
		return f.createError("expecting at least 2 lines", gotLines)
	}

	body := gotLines[1:]
	if err := f.checkRegexps(body); err != nil {
		return f.createError(err.Error(), gotLines)
	}
	return nil
}

func (f *file) createError(msg string, gotLines []string) error {
	expected := []string{"\t" + f.head}
	expected = append(expected, f.lines...)

	b := new(bytes.Buffer)
	b.WriteString(msg + "\n")
	f.writeExpectedAndGot(b, expected, gotLines)

	return errors.New(b.String())
}

func (f *file) checkRegexps(bodyLines []string) error {
	var regexpsMatched []int
	var linesMatched []int

	for regIndex, reg := range f.regs {
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
	allRegexps := len(regexpsMatched) == len(f.regs)
	someRegexp := len(regexpsMatched) > 0

	var msgs []string
	if f.ordered && !ordered {
		msgs = append(msgs, "not ordered")
	}
	if f.all && !allLines {
		msgs = append(msgs, "not all lines")
	}
	if f.regexpQuantifier == "all" && !allRegexps {
		msgs = append(msgs, "not all regexps")
	}
	if f.regexpQuantifier == "exists" && !someRegexp {
		msgs = append(msgs, "no regexp matched")
	}
	if len(msgs) > 0 {
		return errors.New(strings.Join(msgs, ", "))
	}

	return nil
}

func (f *file) writeExpectedAndGot(b *bytes.Buffer, expected, got []string) {
	tw := tabwriter.NewWriter(b, 4, 2, 1, ' ', 0)

	toShow := func(line string) string {
		line = strings.TrimRight(line, "\t\r\n")
		return strings.ReplaceAll(line, "\t", strings.Repeat(" ", 4))
	}

	fmt.Fprintf(tw, "EXPECTED\t\tGOT\n")
	for i := range min(len(expected), len(got)) {
		expectedLine := toShow(expected[i])
		gotLine := toShow(got[i])
		fmt.Fprintf(tw, "%s\t\t%s\n", expectedLine, gotLine)
	}

	if len(expected) > len(got) {
		for i := len(got); i < len(expected); i++ {
			expectedLine := toShow(expected[i])
			fmt.Fprintf(tw, "%s\t\t\n", expectedLine)
		}
	}
	if len(got) > len(expected) {
		for i := len(expected); i < len(got); i++ {
			gotLine := toShow(got[i])
			fmt.Fprintf(tw, "\t\t%s\n", gotLine)
		}
	}

	tw.Flush()
}

// fileExists reports whether a file exists.
func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !errors.Is(err, os.ErrNotExist)
}
