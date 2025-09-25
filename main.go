// This command is a Git hook. It is intended to be used as the pre-commit hook.
package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"
)

func main() {
	denyGit, message := run()
	if denyGit {
		fmt.Fprintln(defaultOS.stdout, message)
		defaultOS.exit(1)
	}
}

// run executes the operations.
func run() (denyGit bool, message string) {
	ops := []operation{
		newVet(),
		newTests(30 * time.Second),
		newThereAreTests(),
	}

	// operationResult is the result of the method run of a operation.
	type operationResult struct {
		message  string
		position int
		err      error
	}

	chOperationResult := make(chan operationResult, len(ops))
	var wg sync.WaitGroup
	for pos, op := range ops {
		wg.Go(func() {
			message, pos, err := op.run(pos)
			chOperationResult <- operationResult{message: message, position: pos, err: err}
		})
	}

	go func() {
		wg.Wait()
		close(chOperationResult)
	}()

	results := make([]operationResult, len(ops))

	for or := range chOperationResult {
		// the position is used for allow the results to always appear in the same order.
		results[or.position] = or
	}

	var mb strings.Builder
	for _, res := range results {
		if res.err != nil {
			mb.WriteString(res.err.Error())
			denyGit = true
		} else if res.message != "" {
			mb.WriteString(res.message)
			denyGit = true
		}
	}

	return denyGit, mb.String()
}

// operation is a operation that can deny a git from proceding.
type operation interface {
	// run executes a operation and returns the message. If the result of the execution doesn't
	// denies git then the empty string will be returned in message. pos will be equals position.
	// We use pos to always display the messages in the same order.
	run(position int) (message string, pos int, err error)
}

// vet is a operation that executes "go vet ./...".
type vet struct {
	// packagesPath contains the path to be used for running the vet.
	packagesPath string
	os           operatingSystem
}

func newVet() operation {
	return vet{packagesPath: "." + string(os.PathSeparator) + "...", os: defaultOS}
}

// run executes "go vet ./...".
func (v vet) run(position int) (message string, pos int, err error) {
	pos = position
	buf := new(bytes.Buffer)
	buf.WriteString("\tfrom go vet\n")

	cmd := v.os.newCmd(nil, buf, "go", "vet", v.packagesPath)
	err = cmd.Run()
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		return buf.String(), pos, nil
	}

	return
}

// tests is a operation that executes the tests.
type tests struct {
	timeoutForEachPackage time.Duration
	// packagesPath contains the path to be used for running the tests.
	packagesPath string
	// coverageRe is for determining the coverage of the tests of a package.
	coverageRe *regexp.Regexp
	os         operatingSystem
}

// newTests creates a tests.
func newTests(timeoutForEachPackage time.Duration) operation {
	return tests{
		timeoutForEachPackage: timeoutForEachPackage,
		packagesPath:          "." + string(os.PathSeparator) + "...",
		coverageRe:            regexp.MustCompile(`^coverage: (\d{1,3}(?:\.\d)?)% of statements\n$`),
		os:                    defaultOS,
	}
}

// run executes the tests of the package and their subpackages.
func (t tests) run(position int) (message string, pos int, err error) {
	// even when the tests reports 100.0% of coverage some statements may not have been executed.
	// Then we use the coverprofile to verifiy whether all statements where executed.
	pos = position
	cpf, err := t.os.createTemp("", "coverprofile*.out")
	if err != nil {
		return
	}
	cpf.Close()                   // we dont need the file now.
	defer t.os.remove(cpf.Name()) // we assume that the file will not be removed automatically by the system.

	to := t.timeoutForEachPackage.String()
	stdout := new(bytes.Buffer)
	cmd := t.os.newCmd(
		stdout, nil,
		"go", "test", "-json", "-timeout="+to, "-vet=off", "-cover", "-failfast", "-coverprofile="+cpf.Name(), t.packagesPath,
	)

	err = cmd.Run()
	if exitError := new(exec.ExitError); err != nil && !errors.As(err, &exitError) {
		return
	}

	results, err := t.results(stdout)
	if err != nil {
		return
	}

	message, err = t.message(results, cpf.Name())
	return
}

// results decodes r as a []testResult, but only results whose kind is not testResultPass are returned.
func (t tests) results(r io.Reader) ([]testResult, error) {
	var rs []testResult
	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if err := json.UnmarshalDecode(dec, &te); err == io.EOF {
			return rs, nil
		} else if err != nil {
			return nil, err
		}
		result := t.result(te)
		if result.kind == testResultPass {
			continue
		}
		rs = append(rs, result)
	}
}

// next decodes an event and returns their meaning
func (t tests) result(te testEvent) (result testResult) {
	if te.Action == "fail" && te.Test != "" {
		result = testResult{kind: testResultFail, message: fmt.Sprintf("%s: %s failed\n", te.Package, te.Test)}
		return
	}

	if te.Action == "output" && strings.HasPrefix(te.Output, "panic: test timed out after") {
		result = testResult{kind: testResultTimeout, message: fmt.Sprintf("%s: %s\n", te.Package, te.Output)}
		return
	}

	if te.Action == "output" && t.coverageRe.MatchString(te.Output) {
		submatches := t.coverageRe.FindAllStringSubmatch(te.Output, -1)
		if submatches[0][1] == "100.0" {
			return
		}
		result = testResult{kind: testResultCoverageNot100PerCent, message: fmt.Sprintf("%s: test coverage is not 100.0%%\n", te.Package)}
		return
	}

	return
}

// message creates the message corresponding to the results of the execution of the tests and the coverprofile generated.
func (t tests) message(rs []testResult, coverProfileName string) (string, error) {
	if len(rs) == 0 { // all tests passed and 100.0% of coverage
		return t.messageFromCoverProfile(coverProfileName)
	}

	buf := new(bytes.Buffer)
	buf.WriteString("\tfrom executing the tests\n")

	// if the tests failed then the coverage can be wrong because of the -failfast option
	failed := slices.ContainsFunc(rs, func(r testResult) bool { return r.kind == testResultFail || r.kind == testResultTimeout })
	if failed {
		for i := range rs {
			if rs[i].kind == testResultCoverageNot100PerCent {
				continue
			}
			buf.WriteString(rs[i].message)
		}
	} else {
		for i := range rs {
			if rs[i].kind == testResultCoverageNot100PerCent {
				buf.WriteString(rs[i].message)
			}
		}
	}

	return buf.String(), nil
}

// messageFromCoverProfile creates the message corresponding to the coverprofile generated.
func (t tests) messageFromCoverProfile(fileName string) (m string, err error) {
	buf := new(bytes.Buffer)
	buf.WriteString("\tfrom cover profile\n")

	f, err := t.os.open(fileName)
	if err != nil {
		return
	}
	defer f.Close()

	var hasStmtNotExecuted bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasSuffix(line, "0") {
			hasStmtNotExecuted = true
			buf.WriteString(line)
		}
	}

	if err = sc.Err(); err != nil {
		return
	}

	if hasStmtNotExecuted {
		return buf.String(), nil
	}
	return
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
	testResultPass testResultKind = iota
	testResultFail
	testResultTimeout
	testResultCoverageNot100PerCent
)

type testResult struct {
	kind    testResultKind
	message string
}

// thereAreTests is a operation that check if there are tests. But if the package dont
// need tests then a message is not generated for it.
type thereAreTests struct {
	// packagesPath contains the path to be used for running the vet.
	packagesPath string
}

func newThereAreTests() operation {
	return thereAreTests{packagesPath: "." + string(os.PathSeparator) + "..."}
}

// run check if a packagem and your subpackages, need and has tests.
func (t thereAreTests) run(position int) (message string, pos int, err error) {
	pos = position
	buf := new(bytes.Buffer)
	buf.WriteString("\tfrom checking if there are tests\n")

	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, t.packagesPath)
	if err != nil {
		return
	}

	pkgsByDir := make(map[string][]*packages.Package)
	var dirs []string
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return "", pos, t.joinErrors(pkg.Errors...)
		}
		pkgsByDir[pkg.Dir] = append(pkgsByDir[pkg.Dir], pkg)
		if !slices.Contains(dirs, pkg.Dir) {
			dirs = append(dirs, pkg.Dir)
		}
	}

	var denyGit bool
	for _, dir := range dirs {
		if t.need(pkgsByDir[dir]) && !t.has(pkgsByDir[dir]) {
			fmt.Fprintf(buf, "%s has no tests\n", t.pkgPath(pkgsByDir[dir]))
			denyGit = true
		}
	}

	if denyGit {
		return buf.String(), pos, nil
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

	paramNamed, ok := paramPointer.Elem().(*types.Named)
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

	paramNamed, ok := paramPointer.Elem().(*types.Named)
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

var defaultOS = operatingSystem{
	stdout:     os.Stdout,
	exit:       os.Exit,
	createTemp: os.CreateTemp,
	remove:     os.Remove,
	open: func(name string) (io.ReadCloser, error) {
		return os.Open(name)
	},
	newCmd: func(stdout, stderr io.Writer, name string, args ...string) command {
		cmd := exec.Command(name, args...)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd
	},
}

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
	newCmd func(stdout, stderr io.Writer, name string, args ...string) command
}

// command is for allowing the tests to change the behavior of the exec.Cmd struct.
type command interface {
	Run() error
}
