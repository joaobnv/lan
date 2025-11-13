// This command is a Git hook. It is intended to be used as the pre-commit hook.
package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"
)

// the version of Lan.
const version = "0.1.0"

func main() {
	workDir, err := defaultOS.getwd()
	if err != nil {
		fmt.Println(defaultOS.stdout, err)
		defaultOS.exit(1)
		return
	}
	denyGit, message := run(workDir)
	if denyGit {
		fmt.Fprintln(defaultOS.stdout, message)
		// I think that showing the version helps the user because Lan does not receive command line parameters
		// that make it show its version.
		fmt.Fprint(defaultOS.stdout, "\tlan version: "+version)
		defaultOS.exit(1)
	}
}

// run executes the operations.
func run(workDir string) (denyGit bool, message string) {
	ops := []operation{
		newTests(30*time.Second, workDir),
		newVet(workDir),
		newStaticcheck(workDir),
	}

	// operationResult is the result of the method run of a operation.
	type operationResult struct {
		message string
		// the position is used for allow the results to always appear in the same order.
		pos int
		err error
	}

	chOperationResult := make(chan operationResult)
	var wg sync.WaitGroup
	for pos, op := range ops {
		wg.Go(func() {
			message, err := op.run()
			chOperationResult <- operationResult{message: message, pos: pos, err: err}
		})
	}

	go func() {
		wg.Wait()
		close(chOperationResult)
	}()

	results := make([]operationResult, len(ops))

	for or := range chOperationResult {
		results[or.pos] = or
	}

	var messages []string
	for _, res := range results {
		if res.err != nil {
			return true, strings.TrimRightFunc(res.err.Error(), unicode.IsSpace)
		} else if res.message != "" {
			messages = append(messages, res.message)
		}
	}

	return len(messages) > 0, strings.Join(messages, "\n")
}

// operation is a operation that can deny a git from proceding.
type operation interface {
	// run executes a operation and returns the message, without white space at end. If the result of the
	// execution doesn't denies git then the empty string will be returned in message.
	run() (message string, err error)
}

// tests is a operation that executes the tests.
type tests struct {
	workDir               string
	timeoutForEachPackage time.Duration
	// packagesPath contains the path to be used for running the tests.
	packagesPath string
	// coverageRe is for determining the coverage of the tests of a package.
	coverageRe *regexp.Regexp
	tat        thereAreTests
	os         operatingSystem
}

// newTests creates a tests.
func newTests(timeoutForEachPackage time.Duration, workDir string) operation {
	return tests{
		workDir:               workDir,
		timeoutForEachPackage: timeoutForEachPackage,
		packagesPath:          "." + string(os.PathSeparator) + "...",
		coverageRe:            regexp.MustCompile(`^coverage: (\d{1,3}(?:\.\d)?)% of statements\n$`),
		tat:                   newThereAreTests(workDir),
		os:                    newOperatingSystem(),
	}
}

// run executes the tests of the package and their subpackages.
func (t tests) run() (message string, err error) {
	tatResCh := make(chan []thereAreTestsResult, 1)
	tatErrorCh := make(chan error, 1)
	go func() {
		res, err := t.tat.run()
		tatResCh <- res
		tatErrorCh <- err
	}()

	// even when the tests reports 100.0% of coverage some statements may not have been executed.
	// Then we use the coverprofile to verifiy whether all statements where executed.
	cpf, err := t.os.createTemp("", "coverprofile*.out")
	if err != nil {
		return
	}
	cpf.Close()                   // we dont need the file now.
	defer t.os.remove(cpf.Name()) // we assume that the file will not be removed automatically by the system.

	to := t.timeoutForEachPackage.String()
	stdout := new(bytes.Buffer)
	cmd := t.os.newCmd(
		t.workDir, stdout, nil,
		"go", "test", "-json", "-timeout="+to, "-vet=off", "-cover", "-failfast", "-coverprofile="+cpf.Name(), t.packagesPath,
	)

	err = cmd.Run()
	if exitError := new(exec.ExitError); err != nil && !errors.As(err, &exitError) {
		return "", fmt.Errorf("\tlan: from executing the tests\n%w", err)
	}

	resultsPerPkg, err := t.results(stdout)
	if err != nil {
		return "", fmt.Errorf("\tlan: from executing the tests\n%w", err)
	}

	for _, rs := range resultsPerPkg {
		if t.buildFailed(rs) {
			// we return before testing a error from thereAreTests because we dont want
			// the message from the packages.Load failure.
			return "", fmt.Errorf("\tlan: from executing the tests\nbuild failed")
		}
	}

	tatResults := <-tatResCh
	tatErr := <-tatErrorCh

	if tatErr != nil {
		return "", tatErr
	}

	return t.message(tatResults, resultsPerPkg, cpf.Name())
}

// results decodes r as a map from the package name to their test results, but only results whose kind
// is not testResultIgnore are returned.
func (t tests) results(r io.Reader) (map[string][]testResult, error) {
	m := make(map[string][]testResult)

	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if err := json.UnmarshalDecode(dec, &te); err == io.EOF {
			return m, nil
		} else if err != nil {
			return nil, err
		}
		result := t.result(te)
		if result.kind != testResultIgnore {
			m[result.pkg] = append(m[result.pkg], result)
		} else if result.pkg != "" {
			if _, ok := m[result.pkg]; !ok {
				m[result.pkg] = nil
			}
		}
	}
}

// next decodes an event and returns their meaning.
func (t tests) result(te testEvent) testResult {
	if te.Action == "fail" && te.Test != "" {
		return testResult{kind: testResultFail, pkg: te.Package, message: fmt.Sprintf("%s: %s failed\n", te.Package, te.Test)}
	}

	if te.Action == "output" && strings.HasPrefix(te.Output, "panic: test timed out after") {
		return testResult{kind: testResultTimeout, pkg: te.Package, message: fmt.Sprintf("%s: %s\n", te.Package, te.Output)}
	}

	if te.Action == "output" && t.coverageRe.MatchString(te.Output) {
		submatches := t.coverageRe.FindAllStringSubmatch(te.Output, -1)
		if submatches[0][1] == "100.0" {
			return testResult{kind: testResultIgnore, pkg: te.Package}
		}
		return testResult{kind: testResultCoverageNot100PerCent, pkg: te.Package,
			message: fmt.Sprintf("%s: test coverage is not 100.0%%\n", te.Package)}
	}

	if te.Action == "build-fail" && te.Test == "" && te.Package == "" {
		return testResult{kind: testResultBuildFail}
	}

	return testResult{kind: testResultIgnore, pkg: te.Package}
}

// message creates the message corresponding to the there are tests verification, the results of the execution of the tests,
// and the coverprofile generated.
func (t tests) message(tatResults []thereAreTestsResult, results map[string][]testResult, coverProfileName string) (string, error) {
	var messages []testMessage

	pkgNames := slices.Collect(maps.Keys(results))
	slices.Sort(pkgNames)

	for _, pkg := range pkgNames {
		var tatRes thereAreTestsResult
		ind := slices.IndexFunc(tatResults, func(e thereAreTestsResult) bool { return e.pkg == pkg })
		if ind >= 0 {
			tatRes = tatResults[ind]
		}
		if tatRes.pkg != "" && tatRes.need && !tatRes.has {
			messages = append(messages, testMessage{kind: testMessageThereAreTests, message: fmt.Sprintf("%s has no tests\n", pkg)})
			continue
		}

		rs := results[pkg]

		if t.allTestsPassed(rs) && t.coverageIs100Percent(rs) {
			ms, err := t.messageFromCoverProfile(pkg, coverProfileName)
			if err != nil {
				return "", err
			}
			messages = append(messages, ms...)
			continue
		}

		messages = append(messages, t.messageFromExecutingTheTests(rs)...)
	}

	var exec, tat, covProf []testMessage
	for _, m := range messages {
		switch m.kind {
		case testMessageExec, testMessageCover:
			exec = append(exec, m)
		case testMessageThereAreTests:
			tat = append(tat, m)
		case testMessageCoverProfile:
			covProf = append(covProf, m)
		}
	}

	var b strings.Builder
	if len(exec) > 0 {
		b.WriteString("\tlan: from executing the tests\n")
		for _, m := range exec {
			b.WriteString(m.message)
		}
	}
	if len(tat) > 0 {
		b.WriteString("\tlan: from checking if there are tests\n")
		for _, m := range tat {
			b.WriteString(m.message)
		}
	}
	if len(covProf) > 0 {
		b.WriteString("\tlan: from cover profile\n")
		for _, m := range covProf {
			b.WriteString(m.message)
		}
	}

	return strings.TrimRightFunc(b.String(), unicode.IsSpace), nil
}

// messageFromExecutingTheTests creates the messages corresponding to the execution of the tests of a package.
func (t tests) messageFromExecutingTheTests(pkgResults []testResult) (ms []testMessage) {
	// if the tests failed then the coverage can be wrong because of the -failfast option
	failed := slices.ContainsFunc(pkgResults, testResult.isFailure)
	if failed {
		for i := range pkgResults {
			if pkgResults[i].kind == testResultCoverageNot100PerCent {
				continue
			}
			ms = append(ms, testMessage{kind: testMessageExec, message: pkgResults[i].message})
		}
	} else {
		for i := range pkgResults {
			if pkgResults[i].kind == testResultCoverageNot100PerCent {
				ms = append(ms, testMessage{kind: testMessageCover, message: pkgResults[i].message})
			}
		}
	}
	return ms
}

// messageFromCoverProfile creates the messages corresponding to the coverprofile generated.
func (t tests) messageFromCoverProfile(pkg string, fileName string) (ms []testMessage, err error) {
	f, err := t.os.open(fileName)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		linePkg := path.Dir(strings.Split(line, ":")[0])
		if linePkg == pkg && strings.HasSuffix(line, "0") {
			// when we run "go clean -testcache" and then "go test -coverprofile=somefile ." then the content
			// of the cover profile may be repeated 3 times.
			if !slices.ContainsFunc(ms, func(e testMessage) bool { return e.message == line+"\n" }) {
				ms = append(ms, testMessage{kind: testMessageCoverProfile, message: line + "\n"})
			}
		}
	}

	err = sc.Err()
	return
}

// buildFailed reports whether the results in rs implies that the build failed.
func (t tests) buildFailed(rs []testResult) bool {
	return slices.ContainsFunc(rs, func(e testResult) bool {
		return e.kind == testResultBuildFail
	})
}

// allTestsPassed reports whether rs dont contains test failures.
func (t tests) allTestsPassed(rs []testResult) bool {
	return !slices.ContainsFunc(rs, testResult.isFailure)
}

// coverageIs100Percent reports whether the results in rs implies that the coverage is 100%.
func (t tests) coverageIs100Percent(rs []testResult) bool {
	return !slices.ContainsFunc(rs, func(e testResult) bool {
		return e.kind == testResultCoverageNot100PerCent
	})
}

// testEvent is a event generated by the test command.
type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

type testResultKind int

const (
	testResultIgnore testResultKind = iota
	testResultFail
	testResultTimeout
	testResultCoverageNot100PerCent
	testResultBuildFail
)

type testResult struct {
	pkg     string
	kind    testResultKind
	message string
}

// isFailure reports whether tr represents a test failure.
func (tr testResult) isFailure() bool {
	return tr.kind == testResultFail || tr.kind == testResultTimeout
}

type testMessageKind int

const (
	testMessageExec testMessageKind = iota
	testMessageThereAreTests
	testMessageCover
	testMessageCoverProfile
)

type testMessage struct {
	kind    testMessageKind
	message string
}

// thereAreTests check if the packages need and have tests.
type thereAreTests struct {
	workDir      string
	packagesPath string
}

func newThereAreTests(workDir string) thereAreTests {
	return thereAreTests{workDir: workDir, packagesPath: "." + string(os.PathSeparator) + "..."}
}

// run check if a package and your subpackages, need and has tests.
func (t thereAreTests) run() (results []thereAreTestsResult, err error) {
	cfg := &packages.Config{
		Dir:   t.workDir,
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, t.packagesPath)
	if err != nil {
		return results, fmt.Errorf("\tlan: from checking if there are tests\n%w", err)
	}

	pkgsByDir := make(map[string][]*packages.Package)
	var dirs []string
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return results, fmt.Errorf("\tlan: from checking if there are tests\n%w", t.joinErrors(pkg.Errors...))
		}
		pkgsByDir[pkg.Dir] = append(pkgsByDir[pkg.Dir], pkg)
		if !slices.Contains(dirs, pkg.Dir) {
			dirs = append(dirs, pkg.Dir)
		}
	}

	for _, dir := range dirs {
		results = append(results, thereAreTestsResult{
			pkg:  t.pkgPath(pkgsByDir[dir]),
			need: t.need(pkgsByDir[dir]),
			has:  t.has(pkgsByDir[dir]),
		})
	}

	return
}

// need reports wheter pkg need tests. A directory may have more than one package because of the tests (see the documentation
// of packages.Config.Tests).
func (t thereAreTests) need(pkgsOfDir []*packages.Package) bool {
	// We dont support folders with multiple non-test packages.
	for _, pkg := range pkgsOfDir {
		if strings.HasSuffix(pkg.PkgPath, ".test") {
			continue
		}
		for _, s := range pkg.Syntax {
			f := pkg.Fset.File(s.FileStart)
			if strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}

			for _, d := range s.Decls {
				if _, ok := d.(*ast.FuncDecl); ok {
					return true
				}
			}
		}
	}

	return false
}

// has reports wheter the directory has tests.
func (t thereAreTests) has(pkgsOfDir []*packages.Package) bool {
	for _, pkg := range pkgsOfDir {
		for _, s := range pkg.Syntax {
			f := pkg.Fset.File(s.FileStart)
			if !strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}

			for _, d := range s.Decls {
				f, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if t.isTestFunction(pkg.TypesInfo, f) {
					return true
				}
			}
		}
	}

	return false
}

// pkgPath return the path of the non test package in the directory. If there aren't such a package then it returns
// the path of the first package in pkgsByDir.
func (t thereAreTests) pkgPath(pkgsByDir []*packages.Package) string {
	for _, pkg := range pkgsByDir {
		if !strings.HasSuffix(pkg.PkgPath, ".test") && !strings.HasSuffix(pkg.PkgPath, "_test") {
			return pkg.PkgPath
		}
	}

	return pkgsByDir[0].PkgPath
}

// isTestFunction reports wheter f has the signature of a test function.
func (t thereAreTests) isTestFunction(ti *types.Info, f *ast.FuncDecl) bool {
	if t.isFuzzTestFunction(ti, f) {
		return true
	}

	if !strings.HasPrefix(f.Name.Name, "Test") {
		return false
	}

	if t.startWithLowerCaseLetter(strings.TrimPrefix(f.Name.Name, "Test")) {
		return false
	}

	sig := ti.Defs[f.Name].Type().(*types.Signature)
	if sig.Params().Len() != 1 {
		return false
	}

	paramPointer, ok := sig.Params().At(0).Type().(*types.Pointer)
	if !ok {
		return false
	}

	elem := types.Unalias(paramPointer.Elem())
	paramNamed, ok := elem.(*types.Named)
	if !ok {
		return false
	}

	if pkg := paramNamed.Obj().Pkg(); pkg == nil || pkg.Path() != "testing" {
		return false
	}

	if paramNamed.Obj().Name() != "T" {
		return false
	}

	return true
}

// isFuzzTestFunction reports wheter f has the signature of a fuzz test function.
func (t thereAreTests) isFuzzTestFunction(ti *types.Info, f *ast.FuncDecl) bool {
	if !strings.HasPrefix(f.Name.Name, "Fuzz") {
		return false
	}
	if t.startWithLowerCaseLetter(strings.TrimPrefix(f.Name.Name, "Fuzz")) {
		return false
	}

	sig := ti.Defs[f.Name].Type().(*types.Signature)
	if sig.Params().Len() != 1 {
		return false
	}

	paramPointer, ok := sig.Params().At(0).Type().(*types.Pointer)
	if !ok {
		return false
	}

	elem := types.Unalias(paramPointer.Elem())
	paramNamed, ok := elem.(*types.Named)
	if !ok {
		return false
	}

	if pkg := paramNamed.Obj().Pkg(); pkg == nil || pkg.Path() != "testing" {
		return false
	}

	if paramNamed.Obj().Name() != "F" {
		return false
	}

	return true
}

// startWithLowerCaseLetter reports if s start with a lower case letter.
func (t thereAreTests) startWithLowerCaseLetter(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLower(r)
}

// joinErrors concatenates the error messages of the errors in errs, with a new line between
// each message, and returns the error with it.
func (t thereAreTests) joinErrors(errs ...packages.Error) error {
	var messages []string

	for _, err := range errs {
		messages = append(messages, err.Error())
	}

	return errors.New(strings.Join(messages, "\n"))
}

type thereAreTestsResult struct {
	pkg  string
	need bool
	has  bool
}

// vet is a operation that executes "go vet ./...".
type vet struct {
	workDir string
	// packagesPath contains the path to be used for running the vet.
	packagesPath string
	os           operatingSystem
}

func newVet(workDir string) operation {
	return vet{workDir: workDir, packagesPath: "." + string(os.PathSeparator) + "...", os: newOperatingSystem()}
}

// run executes "go vet ./...".
func (v vet) run() (message string, err error) {
	buf := new(bytes.Buffer)
	buf.WriteString("\tlan: from go vet\n")

	cmd := v.os.newCmd(v.workDir, nil, buf, "go", "vet", v.packagesPath)
	err = cmd.Run()
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		return strings.TrimRightFunc(buf.String(), unicode.IsSpace), nil
	} else if err != nil {
		return "", fmt.Errorf("\tlan: from go vet\n%w", err)
	}

	return
}

// staticcheck is a operation that executes the staticcheck linter, if it is installed.
type staticcheck struct {
	workDir string
	// packagesPath contains the path to be used for running the vet.
	packagesPath string
	os           operatingSystem
}

// newStaticcheck creates a staticcheck.
func newStaticcheck(workDir string) operation {
	return staticcheck{workDir: workDir, packagesPath: "." + string(os.PathSeparator) + "...", os: newOperatingSystem()}
}

// run executes "staticcheck" and returns the message.
// If the staticcheck is not installed then run returns the empty string.
// It verifies if staticcheck was installed with "go get -tool" or with "go install".
// If staticcheck was installed with "go get -tool" then run executes "go tool staticcheck".
// Otherwise if installed with "go install" then run executes "staticcheck". If installed
// with both methods then run executes executes "go tool staticcheck".
func (s staticcheck) run() (message string, err error) {
	buf := new(bytes.Buffer)
	buf.WriteString("\tlan: from staticcheck\n")

	cmd, err := s.command()
	if err != nil {
		return "", fmt.Errorf("\tlan: from staticcheck\n%w", err)
	}

	type result struct {
		message   string
		err       error
		withTests bool
	}

	chResult := make(chan result)
	var wg sync.WaitGroup
	wg.Go(func() {
		var r result
		r.withTests = true
		r.message, r.err = s.withTests(cmd)
		chResult <- r
	})
	wg.Go(func() {
		var r result
		r.message, r.err = s.withoutTests(cmd)
		chResult <- r
	})

	go func() {
		wg.Wait()
		close(chResult)
	}()

	results := make([]result, 2)
	for r := range chResult {
		if r.withTests {
			results[0] = r
		} else {
			results[1] = r
		}
	}

	for i := range 2 {
		if results[i].err != nil {
			return "", fmt.Errorf("\tlan: from staticcheck\n%w", results[i].err)
		} else if results[i].message != "" {
			// withTests has precedence, so if there are a function not used by the test and the non-test code
			// then it wont be reported two times.
			buf.WriteString(results[i].message)
			return strings.TrimRightFunc(buf.String(), unicode.IsSpace), nil
		}
	}

	return
}

// command determines what command to use, "go tool staticcheck" or "staticcheck".
func (s staticcheck) command() (cmd []string, err error) {
	cmd, err = s.installedWithGoTool()
	if cmd != nil {
		return
	}

	cmd, errSys := s.installedInTheSystem()
	err = cmp.Or(err, errSys)
	return
}

func (s staticcheck) installedWithGoTool() (cmd []string, err error) {
	var stdout strings.Builder
	goToolCmd := s.os.newCmd(s.workDir, &stdout, nil, "go", "tool")
	if err = goToolCmd.Run(); err != nil {
		return
	}
	for line := range strings.Lines(stdout.String()) {
		if strings.TrimSpace(line) == "honnef.co/go/tools/cmd/staticcheck" {
			return []string{"go", "tool", "staticcheck"}, nil
		}
	}

	return
}

// maybe installed with "go install".
func (s staticcheck) installedInTheSystem() (cmd []string, err error) {
	if _, err = exec.LookPath("staticcheck"); errors.Is(err, exec.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return
	}

	return []string{"staticcheck"}, nil
}

// withTests executes staticcheck considering the tests.
func (s staticcheck) withTests(cmd []string) (message string, err error) {
	var b strings.Builder
	cmd = append(cmd, s.packagesPath)
	execCmd := s.os.newCmd(s.workDir, &b, nil, cmd[0], cmd[1:]...)

	err = execCmd.Run()
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		return b.String(), nil
	}

	return
}

// withoutTests executes "staticcheck -tests=false -checks=U1000 ./..." and returns the message.
//
// With the "-tests=false" parameter a function is not considered used if only tests call it.
// I think that only the U1000 check is afected by "-tests=false".
func (s staticcheck) withoutTests(cmd []string) (message string, err error) {
	var b strings.Builder
	cmd = append(cmd, "-tests=false", "-checks=U1000", s.packagesPath)
	execCmd := s.os.newCmd(s.workDir, &b, nil, cmd[0], cmd[1:]...)

	err = execCmd.Run()
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		return b.String(), nil
	}

	return
}

var defaultOS = newOperatingSystem()

// opSystem is for allowing the tests to change the behavior of the packages os and exec.
type operatingSystem struct {
	// stdout is for os.Stdout.
	stdout io.Writer
	// exit is for os.Exit.
	exit func(code int)
	// createTemp is for the os.CreatTemp function.
	createTemp func(dir string, pattern string) (*os.File, error)
	// remove is for the os.Remove function.
	remove func(name string) error
	// open is for the os.Open function.
	open func(name string) (io.ReadCloser, error)
	// newCmd is for exec.CommandContext
	newCmd func(worDir string, stdout, stderr io.Writer, name string, args ...string) command
	// getwd is for os.Getwd
	getwd func() (string, error)
}

func newOperatingSystem() operatingSystem {
	return operatingSystem{
		stdout:     os.Stdout,
		exit:       os.Exit,
		createTemp: os.CreateTemp,
		remove:     os.Remove,
		open: func(name string) (io.ReadCloser, error) {
			return os.Open(name)
		},
		newCmd: func(workDir string, stdout, stderr io.Writer, name string, args ...string) command {
			cmd := exec.Command(name, args...)
			cmd.Dir = workDir
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd
		},
		getwd: os.Getwd,
	}
}

// command is for allowing the tests to change the behavior of the exec.Cmd struct.
type command interface {
	Run() error
}
