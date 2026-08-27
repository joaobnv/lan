package main

// Copyright (c) 2026, João Breno. See the license.

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"
)

var modelLoadErrorTestStart = flag.Int("model.loaderrortest.start", -1, "start of models")
var modelLoadErrorTestN = flag.Int("model.loaderrortest.n", -1, "number of models")

func TestModelLoadErrorTest(t *testing.T) {
	t.Parallel()
	testModel(t, "loaderrortest", and{loadErrorTest, formulaTest}, modelLoadErrorTestStart, modelLoadErrorTestN)
}

var modelTestStart = flag.Int("model.test.start", -1, "start of models")
var modelTestN = flag.Int("model.test.n", -1, "number of models")

func TestModelTest(t *testing.T) {
	t.Parallel()
	testModel(t, "test", and{notLoadErrorTest, formulaTest}, modelTestStart, modelTestN)
}

var modelLoadErrorFuzzStart = flag.Int("model.loaderrorfuzz.start", -1, "start of models")
var modelLoadErrorFuzzN = flag.Int("model.loaderrorfuzz.n", -1, "number of models")

func TestModelLoadErrorFuzz(t *testing.T) {
	t.Parallel()
	testModel(t, "loaderrorfuzz", and{loadErrorFuzz, formulaFuzz}, modelLoadErrorFuzzStart, modelLoadErrorFuzzN)
}

var modelFuzzStart = flag.Int("model.fuzz.start", -1, "start of models")
var modelFuzzN = flag.Int("model.fuzz.n", -1, "number of models")

func TestModelFuzz(t *testing.T) {
	t.Parallel()
	testModel(t, "fuzz", and{notLoadErrorFuzz, formulaFuzz}, modelFuzzStart, modelFuzzN)
}

var modelFlag = flag.Bool("model", false, "test models")

func testModel(t *testing.T, dir string, f formula, startFlag, nFlag *int) {
	if !*modelFlag {
		return
	}
	var dirExists = true
	_, err := os.Stat(filepath.Join("testdata", "gen", dir))
	if errors.Is(err, fs.ErrNotExist) {
		dirExists = false
	} else if err != nil {
		t.Fatal(err)
	}

	if dirExists {
		if testModelDirExists(t, dir) {
			return
		}
	}

	cases, modelEnumerationDuration := createCases(f, dir)

	start := rand.N(len(cases))
	if *startFlag >= 0 {
		if *startFlag >= len(cases) {
			t.Fatalf("invalid start %d, must be in [0, %d)", *startFlag, len(cases))
		}
		start = *startFlag
	}
	n := rand.N(len(cases)-start) + 1
	if *nFlag > 0 {
		n = min(*nFlag, len(cases)-start)
	}

	file, err := os.Create(filepath.Join("testdata", "gen", dir+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = fmt.Fprintf(file, "number of cases: %d\n", len(cases)); err != nil {
		t.Fatal(err)
	}
	if _, err = fmt.Fprintf(file, "model enumeration duration: %s\n", modelEnumerationDuration); err != nil {
		t.Fatal(err)
	}
	if _, err = fmt.Fprintf(file, "start = %d, n = %d\n", start, n); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases[start : start+n] {
		t.Run(tc.name, tc.run)
	}
}

func testModelDirExists(t *testing.T, dir string) (executed bool) {
	tcs := loadCases(t, dir)
	if len(tcs) == 0 {
		return false
	}

	for _, tc := range tcs {
		t.Run(tc.name, tc.run)
	}

	var lines [][]byte
	fileExists := true

	_, err := os.Stat(filepath.Join("testdata", "gen", dir+".txt"))
	if errors.Is(err, fs.ErrNotExist) {
		fileExists = false
	} else if err != nil {
		t.Fatal(err)
	}

	if fileExists {
		data, err := os.ReadFile(filepath.Join("testdata", "gen", dir+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		lines = bytes.SplitN(data, []byte("\n"), 3)
		lines = lines[0:2]
	}

	var line bytes.Buffer
	line.WriteString("cases executed: ")
	for i := range tcs {
		if i > 0 {
			line.WriteString(", ")
		}
		line.WriteString(tcs[i].name)
	}
	line.WriteRune('\n')

	lines = append(lines, line.Bytes())

	file, err := os.Create(filepath.Join("testdata", "gen", dir+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if _, err = file.Write(bytes.Join(lines, []byte("\n"))); err != nil {
		t.Fatal(err)
	}

	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	return true
}

func createCases(f formula, dir string) (tcs []*testCase, modelEnumerationDuration time.Duration) {
	startTime := time.Now()
	ms := slices.Collect(modelsThatSatisfies(f))
	modelEnumerationDuration = time.Since(startTime)

	slices.SortStableFunc(ms, func(a, b model) int {
		return cmp.Compare(a.numberOfAtomsTrue(), b.numberOfAtomsTrue())
	})

	tmpl := template.Must(template.ParseGlob(filepath.Join("testdata", "tmpl", "*.tmpl")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("testdata", "tmpl", "test", "*.tmpl")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("testdata", "tmpl", "fuzz", "*.tmpl")))

	for i := range ms {
		tcs = append(tcs, newTestCase(i, dir, ms[i], tmpl))
	}

	return
}

func loadCases(t *testing.T, dir string) (tcs []*testCase) {
	dirFS := os.DirFS(filepath.Join("testdata", "gen", dir))

	entries, err := fs.ReadDir(dirFS, ".")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) == 0 {
		return
	}

	cases := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Fatalf("loadCases: file %s is not a directory", entry.Name())
		}

		cases = append(cases, entry.Name())
	}

	tmpl := template.Must(template.ParseGlob(filepath.Join("testdata", "tmpl", "*.tmpl")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("testdata", "tmpl", "test", "*.tmpl")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("testdata", "tmpl", "fuzz", "*.tmpl")))

	for _, c := range cases {
		text, err := os.ReadFile(filepath.Join("testdata", "gen", dir, c, "model.txt"))
		if err != nil {
			t.Fatal(err)
		}

		td := &templateData{}
		td.UnmarshalText(text)
		m := newModelFromTemplateData(td)

		n, err := strconv.ParseInt(strings.TrimPrefix(c, "case"), 10, 64)
		if err != nil {
			t.Fatalf("%s: %s", c, err.Error())
		}

		tcs = append(tcs, newTestCase(int(n), dir, m, tmpl))
	}

	return
}

type testCase struct {
	name         string
	workDir      string
	packagesPath string
	errors       []*regexp.Regexp
	tmpl         *template.Template
	td           *templateData
}

func newTestCase(number int, dir string, m model, tmpl *template.Template) *testCase {
	pkgName := "case" + strconv.Itoa(number)
	pkgPath := "github.com/joaobnv/lan/testdata/gen/" + dir + "/" + pkgName
	tc := &testCase{
		name:         pkgName,
		workDir:      filepath.Join("testdata", "gen", dir, pkgName),
		packagesPath: "." + string(os.PathSeparator) + "...",
		tmpl:         tmpl,
		td:           newTemplateData(pkgPath, m),
	}

	notUseTestInStructLiteral := and{aliasToTestingTNamedT, aliasToTestingTPointer, paramTIsPointer,
		not{withoutTestPrefix}, not{lowercaseAfterTest}}
	notUseFuzzInStructLiteral := and{aliasToTestingFNamedF, aliasToTestingFPointer, paramFIsPointer,
		not{withoutFuzzPrefix}, not{lowercaseAfterFuzz}}
	if notUseTestInStructLiteral.value(m) {
		r := regexp.MustCompile(`cannot use _\w*\.TestAdd \(value of type func\(t \*".*?hastest/testdata/gen/loaderrortest/case\d*/test/testing"\.T\)\) as func\(\*"testing"\.T\) value in struct literal`)
		if hasProductionPkg.value(m) {
			tc.errors = append(tc.errors, r)
		}
		if hasTestPkg.value(m) {
			tc.errors = append(tc.errors, r)
		}
	} else if notUseFuzzInStructLiteral.value(m) {
		r := regexp.MustCompile(`cannot use _\w*.FuzzAdd \(value of type func\(f \*".*?hastest/testdata/gen/loaderrorfuzz/case\d*/fuzz/testing"\.F\)\) as func\(f \*"testing"\.F\) value in struct literal`)
		if hasProductionPkg.value(m) {
			tc.errors = append(tc.errors, r)
		}
		if hasTestPkg.value(m) {
			tc.errors = append(tc.errors, r)
		}
	} else if loadErrorTest.value(m) {
		tc.errors = append(tc.errors, regexp.MustCompile(`wrong signature for TestAdd, must be: func TestAdd\(t \*testing\.T\)$`))
	} else if loadErrorFuzz.value(m) {
		tc.errors = append(tc.errors, regexp.MustCompile(`wrong signature for FuzzAdd, must be: func FuzzAdd\(f \*testing\.F\)$`))
	}

	return tc
}

func (tc *testCase) run(t *testing.T) {
	t.Parallel()

	tc.genCode(t)
	tc.writeModel(t)

	expected, err := goTestList{}.has(tc.workDir, tc.packagesPath)
	if err != nil && tc.errors == nil {
		t.Fatal(err)
	}

	var log []string

	tat := newThereAreTests(tc.workDir, newOperatingSystem()).(thereAreTests)

	res, err := tat.has()
	if result := tc.checkErrors(err); result != nil {
		log = append(log, result.Error())
		t.Error(result)
	}

	if tc.errors == nil && err == nil {
		for k, v := range res {
			if expected[k] != v {
				msg := fmt.Sprintf("Has[%s] = %v, want %v", k, v, expected[k])
				log = append(log, msg)
				t.Error(msg)
			}
		}
	}

	if t.Failed() {
		f, err := os.Create(filepath.Join(tc.workDir, "log.txt"))
		if err != nil {
			return
		}
		defer f.Close()

		io.WriteString(f, strings.Join(log, "\n"))
	} else {
		os.RemoveAll(tc.workDir)
	}
}

func (tc *testCase) checkErrors(errGot error) error {
	if tc.errors == nil && errGot == nil {
		return nil
	}
	if tc.errors == nil && errGot != nil {
		return fmt.Errorf("unexpected error:\n%w", errGot)
	}
	if tc.errors != nil && errGot == nil {
		var msg strings.Builder
		msg.WriteString("expecting these errors:\n")
		for i := range tc.errors {
			if i > 0 {
				msg.WriteString("\n")
			}
			msg.WriteString(tc.errors[i].String())
		}
		return errors.New(msg.String())
	}

	lines := strings.Split(errGot.Error(), "\n")
	if len(lines) != len(tc.errors) {
		var msg strings.Builder
		fmt.Fprintf(&msg, "number of lines of error: got %d, want %d", len(lines), len(tc.errors))
		if len(tc.errors) > 0 {
			msg.WriteString("\n   regexps:\n")
			for i := range tc.errors {
				if i > 0 {
					msg.WriteString("\n")
				}
				msg.WriteString(tc.errors[i].String())
			}
		}
		if len(lines) > 0 {
			msg.WriteString("\n   error:\n")
			for i := range lines {
				if i > 0 {
					msg.WriteString("\n")
				}
				msg.WriteString(lines[i])
			}
		}
		return errors.New(msg.String())
	}
	for i, line := range lines {
		if !tc.errors[i].MatchString(line) {
			return fmt.Errorf("regexp not matched\nregexp: %s\nline: %s", tc.errors[i], line)
		}
	}
	return nil
}

func (tc *testCase) genCode(t *testing.T) {
	tc.createDir(t)

	if tc.td.HasProductionPkg {
		tc.createProductionFile(t)
		if tc.td.ConsiderTest {
			tc.createProductionTestFile(t)
		}
		if tc.td.ConsiderFuzz {
			tc.createProductionFuzzFile(t)
		}
	}

	if tc.td.HasTestPkg {
		if tc.td.ConsiderTest {
			tc.createTestPkgFile(t)
		}
		if tc.td.ConsiderFuzz {
			tc.createFuzzPkgFile(t)
		}
	}

	if tc.td.ImportOtherTesting {
		if tc.td.ConsiderTest {
			tc.createTestTestingPkg(t)
		}
		if tc.td.ConsiderFuzz {
			tc.createFuzzTestingPkg(t)
		}
	}
}

func (tc *testCase) createDir(t *testing.T) {
	if err := os.MkdirAll(tc.workDir, os.ModeDir); err != nil {
		t.Fatal(err)
	}
}

func (tc *testCase) createProductionFile(t *testing.T) {
	tc.createFile(t, tc.workDir, "a.go", "production_main.tmpl")
}

func (tc *testCase) createProductionTestFile(t *testing.T) {
	tc.createFile(t, tc.workDir, "a_test_test.go", "test_same_pkg.tmpl")
}

func (tc *testCase) createTestPkgFile(t *testing.T) {
	tc.createFile(t, tc.workDir, "o_test_test.go", "test_other_pkg.tmpl")
}

func (tc *testCase) createTestTestingPkg(t *testing.T) {
	dirPath := filepath.Join(tc.workDir, "test", "testing")
	if err := os.MkdirAll(dirPath, os.ModeDir); err != nil {
		t.Fatal(err)
	}
	tc.createFile(t, dirPath, "testing.go", "test_testing.tmpl")
}

func (tc *testCase) createProductionFuzzFile(t *testing.T) {
	tc.createFile(t, tc.workDir, "a_fuzz_test.go", "fuzz_same_pkg.tmpl")
}

func (tc *testCase) createFuzzPkgFile(t *testing.T) {
	tc.createFile(t, tc.workDir, "o_fuzz_test.go", "fuzz_other_pkg.tmpl")
}

func (tc *testCase) createFuzzTestingPkg(t *testing.T) {
	dirPath := filepath.Join(tc.workDir, "fuzz", "testing")
	if err := os.MkdirAll(dirPath, os.ModeDir); err != nil {
		t.Fatal(err)
	}
	tc.createFile(t, dirPath, "testing.go", "fuzz_testing.tmpl")
}

func (tc *testCase) createFile(t *testing.T, dirPath, fileName, templateName string) {
	f, err := os.Create(filepath.Join(dirPath, fileName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err = tc.tmpl.ExecuteTemplate(f, templateName, tc.td); err != nil {
		t.Fatal(err)
	}

	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func (tc *testCase) writeModel(t *testing.T) {
	f, err := os.Create(filepath.Join(tc.workDir, "model.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	d, _ := tc.td.MarshalText()
	if _, err = f.Write(d); err != nil {
		t.Fatal(err)
	}

	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

type templateData struct {
	PkgPath string

	HasProductionPkg   bool
	HasTestPkg         bool
	ImportDot          bool
	ImportRename       bool
	ImportOtherTesting bool

	ConsiderTest           bool
	HasTestInProductionPkg bool
	HasTestInTestPkg       bool
	WithoutTestPrefix      bool
	LowercaseAfterTest     bool
	AliasToTestingT        bool
	AliasToTestingTNamedT  bool
	AliasToTestingTPointer bool
	ParamTIsPointer        bool

	ConsiderFuzz           bool
	HasFuzzInProductionPkg bool
	HasFuzzInTestPkg       bool
	WithoutFuzzPrefix      bool
	LowercaseAfterFuzz     bool
	AliasToTestingF        bool
	AliasToTestingFNamedF  bool
	AliasToTestingFPointer bool
	ParamFIsPointer        bool
}

func newTemplateData(pkgPath string, m model) *templateData {
	return &templateData{
		PkgPath:                pkgPath,
		HasProductionPkg:       m[hasProductionPkg],
		HasTestPkg:             m[hasTestPkg],
		ImportDot:              m[importDot],
		ImportRename:           m[importRename],
		ImportOtherTesting:     m[importOtherTesting],
		ConsiderTest:           m[considerTest],
		HasTestInProductionPkg: m[hasTestInProductionPkg],
		HasTestInTestPkg:       m[hasTestInTestPkg],
		WithoutTestPrefix:      m[withoutTestPrefix],
		LowercaseAfterTest:     m[lowercaseAfterTest],
		AliasToTestingT:        m[aliasToTestingT],
		AliasToTestingTNamedT:  m[aliasToTestingTNamedT],
		AliasToTestingTPointer: m[aliasToTestingTPointer],
		ConsiderFuzz:           m[considerFuzz],
		HasFuzzInProductionPkg: m[hasFuzzInProductionPkg],
		HasFuzzInTestPkg:       m[hasFuzzInTestPkg],
		WithoutFuzzPrefix:      m[withoutFuzzPrefix],
		LowercaseAfterFuzz:     m[lowercaseAfterFuzz],
		AliasToTestingF:        m[aliasToTestingF],
		AliasToTestingFNamedF:  m[aliasToTestingFNamedF],
		AliasToTestingFPointer: m[aliasToTestingFPointer],
		ParamTIsPointer:        m[paramTIsPointer],
		ParamFIsPointer:        m[paramFIsPointer],
	}
}

func (td *templateData) MarshalText() (text []byte, err error) {
	var buf bytes.Buffer

	v := reflect.ValueOf(td)
	v = v.Elem()
	for f, b := range v.Fields() {
		if b.Kind() == reflect.Bool && b.Bool() {
			buf.WriteString(f.Name)
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes(), nil
}

func (td *templateData) UnmarshalText(text []byte) error {
	atoms := make(map[string]bool)
	for line := range bytes.Lines(text) {
		line = bytes.TrimSpace(line)
		atoms[string(line)] = true
	}

	v := reflect.ValueOf(td)
	v = v.Elem()
	for f, b := range v.Fields() {
		if f.Type.Kind() != reflect.Bool {
			continue
		}

		b.SetBool(atoms[f.Name])
	}

	return nil
}

func modelsThatSatisfies(f formula) iter.Seq[model] {
	return func(yield func(model) bool) {
		m := make(model)
		atoms := f.atoms()
		for nextModel(m, atoms) {
			if f.value(m) {
				if !yield(maps.Clone(m)) {
					return
				}
			}
		}
	}
}

func nextModel(m model, atoms []atom) (ok bool) {
	if len(atoms) == 0 {
		return false
	}
	if len(m) == 0 {
		for _, a := range atoms {
			m[a] = false
		}
		return true
	}
	for _, a := range atoms {
		if m[a] {
			m[a] = false
		} else {
			m[a] = true
			return true
		}
	}
	return false
}

type model map[atom]bool

func (m model) numberOfAtomsTrue() (n int) {
	for _, v := range m {
		if v {
			n++
		}
	}
	return
}

func newModelFromTemplateData(td *templateData) model {
	m := make(model)
	m[hasProductionPkg] = td.HasProductionPkg
	m[hasTestPkg] = td.HasTestPkg
	m[importDot] = td.ImportDot
	m[importRename] = td.ImportRename
	m[importOtherTesting] = td.ImportOtherTesting
	m[considerTest] = td.ConsiderTest
	m[hasTestInProductionPkg] = td.HasTestInProductionPkg
	m[hasTestInTestPkg] = td.HasTestInTestPkg
	m[withoutTestPrefix] = td.WithoutTestPrefix
	m[lowercaseAfterTest] = td.LowercaseAfterTest
	m[aliasToTestingT] = td.AliasToTestingT
	m[aliasToTestingTNamedT] = td.AliasToTestingTNamedT
	m[aliasToTestingTPointer] = td.AliasToTestingTPointer
	m[considerFuzz] = td.ConsiderFuzz
	m[hasFuzzInProductionPkg] = td.HasFuzzInProductionPkg
	m[hasFuzzInTestPkg] = td.HasFuzzInTestPkg
	m[withoutFuzzPrefix] = td.WithoutFuzzPrefix
	m[lowercaseAfterFuzz] = td.LowercaseAfterFuzz
	m[aliasToTestingF] = td.AliasToTestingF
	m[aliasToTestingFNamedF] = td.AliasToTestingFNamedF
	m[aliasToTestingFPointer] = td.AliasToTestingFPointer
	m[paramTIsPointer] = td.ParamTIsPointer
	m[paramFIsPointer] = td.ParamFIsPointer
	return m
}

type formula interface {
	value(m model) bool
	atoms() []atom
}

func implies(hipothesis, conclusion formula) formula {
	return or{not{hipothesis}, conclusion}
}

type or []formula

func (o or) value(m model) bool {
	for _, f := range o {
		if f.value(m) {
			return true
		}
	}
	return false
}

func (o or) atoms() []atom {
	return atoms(o...)
}

type and []formula

func (a and) value(m model) bool {
	for _, f := range a {
		if !f.value(m) {
			return false
		}
	}
	return true
}

func (a and) atoms() []atom {
	return atoms(a...)
}

type not struct {
	f formula
}

func (n not) value(m model) bool {
	return !n.f.value(m)
}

func (n not) atoms() []atom {
	return n.f.atoms()
}

type atom int32

func (a atom) value(m model) bool {
	return m[a]
}

func (a atom) atoms() []atom {
	return []atom{a}
}

func atoms(fs ...formula) (result []atom) {
	if len(fs) == 0 {
		return
	}
	result = fs[0].atoms()
	for _, f := range fs[1:] {
		for _, a := range f.atoms() {
			if !slices.Contains(result, a) {
				result = append(result, a)
			}
		}
	}
	return result
}

func atMostOne(fs ...formula) formula {
	f := and{}
	for i := range fs {
		notOthers := and{}
		for j := range fs {
			if i != j {
				notOthers = append(notOthers, not{fs[j]})
			}
		}
		f = append(f, implies(fs[i], notOthers))
	}
	return f
}

func onlyOne(fs ...formula) formula {
	return and{atMostOne(fs...), append(or{}, fs...)}
}

const (
	hasProductionPkg atom = iota
	hasTestPkg
	importDot
	importRename
	importOtherTesting
	considerTest
	hasTestInProductionPkg
	hasTestInTestPkg
	withoutTestPrefix
	lowercaseAfterTest
	aliasToTestingT
	aliasToTestingTNamedT
	aliasToTestingTPointer
	paramTIsPointer
	considerFuzz
	hasFuzzInProductionPkg
	hasFuzzInTestPkg
	withoutFuzzPrefix
	lowercaseAfterFuzz
	aliasToTestingF
	aliasToTestingFNamedF
	aliasToTestingFPointer
	paramFIsPointer
)

var (
	loadErrorTest = and{considerTest, or{hasProductionPkg, hasTestPkg}, not{withoutTestPrefix}, not{lowercaseAfterTest},
		onlyOne(
			and{not{paramTIsPointer}, not{aliasToTestingT}},
			and{not{paramTIsPointer}, aliasToTestingT, not{aliasToTestingTPointer}},
			and{not{paramTIsPointer}, aliasToTestingT, aliasToTestingTPointer},
			and{paramTIsPointer, aliasToTestingTPointer},
			and{paramTIsPointer, aliasToTestingT, not{aliasToTestingTNamedT}},
		)}
	notLoadErrorTest = not{and{considerTest, or{hasProductionPkg, hasTestPkg}, not{withoutTestPrefix}, not{lowercaseAfterTest},
		or{
			and{not{paramTIsPointer}, not{aliasToTestingT}},
			and{not{paramTIsPointer}, aliasToTestingT, not{aliasToTestingTPointer}},
			and{not{paramTIsPointer}, aliasToTestingT, aliasToTestingTPointer},
			and{paramTIsPointer, aliasToTestingTPointer},
			and{paramTIsPointer, aliasToTestingT, not{aliasToTestingTNamedT}},
		}}}
	loadErrorFuzz = and{considerFuzz, or{hasProductionPkg, hasTestPkg}, not{withoutFuzzPrefix}, not{lowercaseAfterFuzz},
		onlyOne(
			and{not{paramFIsPointer}, not{aliasToTestingF}},
			and{not{paramFIsPointer}, aliasToTestingF, not{aliasToTestingFPointer}},
			and{not{paramFIsPointer}, aliasToTestingF, aliasToTestingFPointer},
			and{paramFIsPointer, aliasToTestingFPointer},
			and{paramFIsPointer, aliasToTestingF, not{aliasToTestingFNamedF}},
		)}
	notLoadErrorFuzz = not{and{considerFuzz, or{hasProductionPkg, hasTestPkg}, not{withoutFuzzPrefix}, not{lowercaseAfterFuzz},
		or{
			and{not{paramFIsPointer}, not{aliasToTestingF}},
			and{not{paramFIsPointer}, aliasToTestingF, not{aliasToTestingFPointer}},
			and{not{paramFIsPointer}, aliasToTestingF, aliasToTestingFPointer},
			and{paramFIsPointer, aliasToTestingFPointer},
			and{paramFIsPointer, aliasToTestingF, not{aliasToTestingFNamedF}},
		}}}
	validTestSignature = and{not{withoutTestPrefix}, not{lowercaseAfterTest}}
	validFuzzSignature = and{not{withoutFuzzPrefix}, not{lowercaseAfterFuzz}}
	hasNormalTest      = and{considerTest, or{hasTestInProductionPkg, hasTestInTestPkg}}
	hasFuzzTest        = and{considerFuzz, or{hasFuzzInProductionPkg, hasFuzzInTestPkg}}
	formulaTest        = and{
		considerTest,
		implies(hasTestInProductionPkg, hasProductionPkg),
		implies(hasTestInTestPkg, hasTestPkg),
		implies(and{hasTestInProductionPkg, hasTestInTestPkg}, validTestSignature),
		implies(and{hasTestInProductionPkg, not{validTestSignature}}, hasTestPkg),
		implies(and{hasTestInTestPkg, not{validTestSignature}}, hasProductionPkg),
		atMostOne(withoutTestPrefix, lowercaseAfterTest),
		implies(or{aliasToTestingTNamedT, aliasToTestingTPointer}, aliasToTestingT),
		implies(aliasToTestingT, importOtherTesting),
		implies(importOtherTesting, aliasToTestingT),
		implies(or{importDot, importRename, importOtherTesting}, or{hasNormalTest, not{validTestSignature}}),
		atMostOne(importDot, importRename),
	}
	formulaFuzz = and{
		considerFuzz,
		implies(hasFuzzInProductionPkg, hasProductionPkg),
		implies(hasFuzzInTestPkg, hasTestPkg),
		implies(and{hasFuzzInProductionPkg, hasFuzzInTestPkg}, validFuzzSignature),
		implies(and{hasFuzzInProductionPkg, not{validFuzzSignature}}, hasTestPkg),
		implies(and{hasFuzzInTestPkg, not{validFuzzSignature}}, hasProductionPkg),
		atMostOne(withoutFuzzPrefix, lowercaseAfterFuzz),
		implies(or{aliasToTestingFNamedF, aliasToTestingFPointer}, aliasToTestingF),
		implies(aliasToTestingF, importOtherTesting),
		implies(importOtherTesting, aliasToTestingF),
		implies(or{importDot, importRename, importOtherTesting}, or{hasFuzzTest, not{validFuzzSignature}}),
		atMostOne(importDot, importRename),
	}
	formulaTestAndFuzz = and{formulaTest, formulaFuzz}
)

type goTestList struct{}

func (gtl goTestList) has(workDir, packagesPath string) (result map[string]bool, err error) {
	cmd := exec.Command("go", "test", "-json", "-vet=off", "-list=(^Test)|(^Fuzz)", packagesPath)
	stdout := new(bytes.Buffer)
	cmd.Dir = workDir
	cmd.Stdout = stdout

	err = cmd.Run()
	if _, isExitError := errors.AsType[*exec.ExitError](err); err != nil && !isExitError {
		return nil, err
	}

	pkgs, err := gtl.decode(stdout)
	if err != nil {
		return
	}

	result = make(map[string]bool, len(pkgs))
	for i := range pkgs {
		result[pkgs[i]] = true
	}
	return
}

// decode decodes r and returns the packages that have tests.
func (gtl goTestList) decode(r io.Reader) ([]string, error) {
	var pkgs []string

	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if err := json.UnmarshalDecode(dec, &te); err == io.EOF {
			return pkgs, nil
		} else if err != nil {
			return pkgs, err
		}

		if gtl.isTest(te) {
			if !slices.Contains(pkgs, te.Package) {
				pkgs = append(pkgs, te.Package)
			}
		}
	}
}

// isTest reports whether the event represents a test listing.
func (gtl goTestList) isTest(te testEvent) bool {
	if te.Action != "output" || te.Package == "" {
		return false
	}
	return strings.HasPrefix(te.Output, "Test") || strings.HasPrefix(te.Output, "Fuzz")
}
