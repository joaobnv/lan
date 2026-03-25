// This command is a Git hook. It is intended to be used as the pre-commit hook.
package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// the version of Lan.
const version = "0.1.0"

func main() {
	workDir, err := defaultOS.getwd()
	if err != nil {
		fmt.Fprint(defaultOS.stdout, err)
		defaultOS.exit(1)
		return
	}
	denyGit, message := run(runConfig{workDir: workDir, os: defaultOS})
	if denyGit {
		fmt.Fprint(defaultOS.stdout, message)
		// I think that showing the version helps the user because Lan does not receive command line parameters
		// that make it show its version.
		fmt.Fprint(defaultOS.stdout, "\n\tlan version: "+version+"\n")
		defaultOS.exit(1)
	}
}

type runConfig struct {
	workDir     string
	os          operatingSystem
	testTimeout time.Duration
}

// run executes the operations.
func run(cfg runConfig) (denyGit bool, message string) {
	pkgs, err := listPackages(cfg.workDir, cfg.os)
	if err != nil {
		return true, strings.TrimRightFunc(err.Error(), unicode.IsSpace)
	}

	if cfg.testTimeout == 0 {
		cfg.testTimeout = 120 * time.Second
	}

	og := newOpGroup()
	ctx := context.Background()

	og.executeGo(ctx, newBuild(cfg.workDir, cfg.os), pkgs)
	og.executeGo(ctx, newThereAreTests(cfg.workDir, cfg.os), pkgs)
	og.executeGo(ctx, newVet(cfg.workDir, cfg.os), pkgs)
	og.executeGo(ctx, newStaticcheck(cfg.workDir, cfg.os), pkgs)
	og.executeGo(ctx, newTests(cfg.testTimeout, cfg.workDir, cfg.os), pkgs)

	message, err = og.wait()
	if err != nil {
		return true, strings.TrimRightFunc(err.Error(), unicode.IsSpace)
	}

	return message != "", message
}

func listPackages(workDir string, opSys operatingSystem) (pkgs []string, err error) {
	stdout := new(bytes.Buffer)
	stderr := new(strings.Builder)
	cmd := opSys.newCmd(context.Background(), workDir, stdout, stderr, "go", "list", "."+string(os.PathSeparator)+"...")
	err = cmd.Run()
	if _, isExitError := errors.AsType[*exec.ExitError](err); err != nil && !isExitError {
		return
	}
	err = nil

	if stderr.Len() > 0 {
		return nil, errors.New("\tlan: error\n" + stderr.String())
	}

	for line := range bytes.Lines(stdout.Bytes()) {
		pkgs = append(pkgs, string(bytes.TrimSpace(line)))
	}

	return
}

// operation is a operation that can deny a git from proceding.
type operation interface {
	// run executes a operation and returns the message, without white space at end. If the result of the
	// execution doesn't denies git then the empty string will be returned in message.
	run(ctx context.Context, packages []string) (message string, err error)
}

// packageOperation is operates in only one packages.
type packageOperation interface {
	// run executes a operation and returns the message, without white space at end. If the result of the
	// execution doesn't denies git then the empty string will be returned in message.
	run(ctx context.Context, pkg string) (message string, err error)
}

// opGroup executes operations. The method wait returns the first failure, if any.
// That is, the first operation result where the message is not empty or the error is non nil.
// First with respect to the order of the calls to executeGo, that is, if multiple
// operations return failures then wait returns the failure of the operation in relation
// to which executeGo was called first.
//
// If there is no failure then wait returns the empty message and the nil error.
type opGroup struct {
	ops        []operation
	cancellers []context.CancelFunc
	rs         []opGroupOperationResult
	wg         sync.WaitGroup
	resultCh   chan opGroupOperationResult

	// is so that the tests can determine how many operations have already been completed.
	done        *sync.Cond
	numberEnded int
}

func newOpGroup() *opGroup {
	return &opGroup{resultCh: make(chan opGroupOperationResult), done: sync.NewCond(&sync.Mutex{})}
}

func (eg *opGroup) executeGo(ctx context.Context, op operation, pkgs []string) {
	eg.ops = append(eg.ops, op)
	eg.rs = append(eg.rs, opGroupOperationResult{})
	ctx, cancel := context.WithCancel(ctx)
	eg.cancellers = append(eg.cancellers, cancel)

	pos := len(eg.ops) - 1
	eg.wg.Go(func() {
		message, err := op.run(ctx, pkgs)
		eg.resultCh <- opGroupOperationResult{
			message: message, err: err, pos: pos, done: true,
		}

		eg.done.L.Lock()
		eg.numberEnded++
		eg.done.Signal()
		eg.done.L.Unlock()
	})
}

func (eg *opGroup) wait() (message string, err error) {
	go func() {
		eg.wg.Wait()
		close(eg.resultCh)
	}()

	for res := range eg.resultCh {
		eg.rs[res.pos] = res
		if res.message != "" || res.err != nil {
			for _, cancel := range eg.cancellers[res.pos:] {
				cancel()
			}
		}
	}

	for _, res := range eg.rs {
		if res.message != "" || res.err != nil {
			return res.message, res.err
		}
	}

	return "", nil
}

type opGroupOperationResult struct {
	message string
	pos     int
	err     error
	done    bool
}

// pkgpkgOpGroup executes package operations. The method wait returns the first failure, if any.
// That is, the first operation result where the message is not empty or the error is non nil.
// First not respect to the order of the calls to executeGo, that is, if multiple
// operations return failures then wait returns the failure of the first operation that returned.
//
// If there is no failure then wait returns the empty message and the nil error.
type pkgOpGroup struct {
	ops        []packageOperation
	cancellers []context.CancelFunc
	rs         []pkgOpGroupOperationResult
	wg         sync.WaitGroup
	resultCh   chan pkgOpGroupOperationResult

	// is so that the tests can determine how many package operations have already been completed.
	done        *sync.Cond
	numberEnded int
}

func newPkgOpGroup() *pkgOpGroup {
	return &pkgOpGroup{resultCh: make(chan pkgOpGroupOperationResult), done: sync.NewCond(&sync.Mutex{})}
}

func (eg *pkgOpGroup) executeGo(ctx context.Context, pkgOp packageOperation, pkg string) {
	eg.ops = append(eg.ops, pkgOp)
	eg.rs = append(eg.rs, pkgOpGroupOperationResult{})
	ctx, cancel := context.WithCancel(ctx)
	eg.cancellers = append(eg.cancellers, cancel)

	pos := len(eg.ops) - 1
	eg.wg.Go(func() {
		message, err := pkgOp.run(ctx, pkg)
		eg.resultCh <- pkgOpGroupOperationResult{
			message: message, err: err, pos: pos, done: true,
		}

		eg.done.L.Lock()
		eg.numberEnded++
		eg.done.Signal()
		eg.done.L.Unlock()
	})
}

func (eg *pkgOpGroup) wait() (message string, err error) {
	go func() {
		eg.wg.Wait()
		close(eg.resultCh)
	}()

	var resultRes *pkgOpGroupOperationResult
	for res := range eg.resultCh {
		eg.rs[res.pos] = res
		if res.message != "" || res.err != nil {
			if resultRes == nil {
				resultRes = &res
			}
			for _, cancel := range eg.cancellers {
				cancel()
			}
		}
	}

	if resultRes == nil {
		return "", nil
	}
	return resultRes.message, resultRes.err
}

type pkgOpGroupOperationResult struct {
	message string
	pos     int
	err     error
	done    bool
}

// build is a operation that executes the build for each package.
type build struct {
	workDir string
	os      operatingSystem
}

func newBuild(workDir string, opSys operatingSystem) operation {
	return build{workDir: workDir, os: opSys}
}

// run builds the packages.
func (b build) run(ctx context.Context, packages []string) (message string, err error) {
	g := newPkgOpGroup()

	for _, pkg := range packages {
		bp := newBuildPkg(b.workDir, b.os)
		g.executeGo(ctx, bp, pkg)
	}

	return g.wait()
}

// build is a operation that executes the build of a package.
type buildPkg struct {
	workDir string
	os      operatingSystem
}

func newBuildPkg(workDir string, opSys operatingSystem) packageOperation {
	return buildPkg{workDir: workDir, os: opSys}
}

// run builds the packages.
func (b buildPkg) run(ctx context.Context, pkg string) (message string, err error) {
	stdout := new(bytes.Buffer)

	cmd := b.os.newCmd(ctx, b.workDir, stdout, nil,
		"go", "test", "-c", "-json", "-vet=off", "-o="+os.DevNull, pkg)
	err = cmd.Run()
	if _, isExitError := errors.AsType[*exec.ExitError](err); err != nil && !isExitError {
		return message, fmt.Errorf("\tlan: from building\n%w", err)
	}

	buildOk, err := b.result(stdout)

	if err != nil {
		return "", fmt.Errorf("\tlan: from building\n%w", err)
	}

	if !buildOk {
		return "\tlan: from building\nbuild failed", nil
	}

	return
}

// result decodes r and reports whether the build was ok.
func (t buildPkg) result(r io.Reader) (buildOk bool, err error) {
	buildOk = true

	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if err = json.UnmarshalDecode(dec, &te); err == io.EOF {
			return buildOk, nil
		} else if err != nil {
			return false, err
		}
		if te.Action == "build-fail" {
			buildOk = false
		}
	}
}

// thereAreTests check if the packages need and have tests.
type thereAreTests struct {
	workDir string
	os      operatingSystem
}

func newThereAreTests(workDir string, opSys operatingSystem) operation {
	return thereAreTests{workDir: workDir, os: opSys}
}

// run checks if all packages that need tests have tests.
func (t thereAreTests) run(ctx context.Context, packages []string) (message string, err error) {
	g := newPkgOpGroup()

	for _, pkg := range packages {
		tp := newThereAreTestsPkg(t.workDir, t.os)
		g.executeGo(ctx, tp, pkg)
	}

	return g.wait()
}

// thereAreTestsPkg check if a package need and have tests.
type thereAreTestsPkg struct {
	workDir string
	os      operatingSystem
}

func newThereAreTestsPkg(workDir string, opSys operatingSystem) packageOperation {
	return thereAreTestsPkg{workDir: workDir, os: opSys}
}

// run checks if the package need and have tests.
func (t thereAreTestsPkg) run(ctx context.Context, pkg string) (message string, err error) {
	need, err := t.need(ctx, pkg)
	if err != nil {
		return
	}
	has, err := t.has(ctx, pkg)
	if err != nil {
		return
	}

	if need && !has {
		return fmt.Sprintf("\tlan: from checking if there are tests\n%s has no tests", pkg), nil
	}

	return "", nil

}

// need reports whether the package need tests.
func (t thereAreTestsPkg) need(ctx context.Context, pkgPath string) (bool, error) {
	// We dont support folders with multiple non-test packages.

	cfg := &packages.Config{
		Dir:     t.workDir,
		Context: ctx,
		Mode:    packages.NeedSyntax | packages.NeedFiles | packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return false, err
	}

	pkg := pkgs[0]

	funcDecl := func(e ast.Decl) bool { _, ok := e.(*ast.FuncDecl); return ok }

	if len(pkg.Errors) > 0 {
		return false, t.joinErrors(pkg.Errors...)
	}
	for _, s := range pkg.Syntax {
		if slices.ContainsFunc(s.Decls, funcDecl) {
			return true, nil
		}
	}

	return false, nil
}

// has reports wheter the package have tests.
func (t thereAreTestsPkg) has(ctx context.Context, pkgPath string) (bool, error) {
	stdout := new(bytes.Buffer)
	cmd := t.os.newCmd(ctx,
		t.workDir, stdout, nil,
		"go", "test", "-json", "-vet=off", "-list=(^Test)|(^Fuzz)", pkgPath,
	)

	err := cmd.Run()
	if _, isExitError := errors.AsType[*exec.ExitError](err); err != nil && !isExitError {
		return false, err
	}

	return t.decode(stdout)
}

// decode decodes r and reports whether the package has or not has tests.
func (t thereAreTestsPkg) decode(r io.Reader) (bool, error) {
	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if err := json.UnmarshalDecode(dec, &te); err == io.EOF {
			return false, nil
		} else if err != nil {
			return false, err
		}

		if t.isTest(te) {
			return true, nil
		}
		if t.isBuildFailure(te) {
			return false, errors.New("build fail")
		}
	}
}

// isTest reports whether the event represents a test listing.
func (t thereAreTestsPkg) isTest(te testEvent) bool {
	if te.Action != "output" || te.Package == "" {
		return false
	}
	return strings.HasPrefix(te.Output, "Test") || strings.HasPrefix(te.Output, "Fuzz")
}

// isBuildFailure reports whether the event represents a build failure.
func (t thereAreTestsPkg) isBuildFailure(te testEvent) bool {
	return te.Action == "build-fail" && te.Test == "" && te.Package == ""
}

// joinErrors concatenates the error messages of the errors in errs, with a new line between
// each message, and returns the error with it.
func (t thereAreTestsPkg) joinErrors(errs ...packages.Error) error {
	var messages []string

	for _, err := range errs {
		messages = append(messages, err.Error())
	}

	return errors.New(strings.Join(messages, "\n"))
}

// vet is a operation that executes "go vet" in each package.
type vet struct {
	workDir string
	os      operatingSystem
}

func newVet(workDir string, opSys operatingSystem) operation {
	return vet{workDir: workDir, os: opSys}
}

// run executes "go vet" on the packages.
func (v vet) run(ctx context.Context, packages []string) (message string, err error) {
	g := newPkgOpGroup()

	for _, pkg := range packages {
		tp := newVetPkg(v.workDir, v.os)
		g.executeGo(ctx, tp, pkg)
	}

	return g.wait()

}

// vet is a operation that executes "go vet" in the package.
type vetPkg struct {
	workDir string
	os      operatingSystem
}

func newVetPkg(workDir string, opSys operatingSystem) packageOperation {
	return vetPkg{workDir: workDir, os: opSys}
}

func (vp vetPkg) run(ctx context.Context, pkg string) (message string, err error) {
	stderr := new(bytes.Buffer)
	stderr.WriteString("\tlan: from go vet\n")

	cmd := vp.os.newCmd(ctx, vp.workDir, nil, stderr, "go", "vet", pkg)
	err = cmd.Run()
	if _, isExitError := errors.AsType[*exec.ExitError](err); err != nil && !isExitError {
		return "", fmt.Errorf("\tlan: from go vet\n%w", err)
	} else if err != nil {
		return strings.TrimRightFunc(stderr.String(), unicode.IsSpace), nil
	}

	return
}

// staticcheck is a operation that executes the staticcheck linter in the packages, if it is installed.
type staticcheck struct {
	workDir string
	os      operatingSystem
}

// newStaticcheck creates a staticcheck.
func newStaticcheck(workDir string, opSys operatingSystem) operation {
	return staticcheck{workDir: workDir, os: opSys}
}

func (s staticcheck) run(ctx context.Context, packages []string) (message string, err error) {
	g := newPkgOpGroup()

	for _, pkg := range packages {
		tp := newStaticcheckPkg(s.workDir, s.os)
		g.executeGo(ctx, tp, pkg)
	}

	return g.wait()
}

// staticcheckPkg is a operation that executes the staticcheck linter in a package, if it is installed.
type staticcheckPkg struct {
	workDir string
	os      operatingSystem
}

// newStaticcheck creates a staticcheck.
func newStaticcheckPkg(workDir string, opSys operatingSystem) packageOperation {
	return staticcheckPkg{workDir: workDir, os: opSys}
}

// run executes "staticcheck" and returns the message.
// If the staticcheck is not installed then run returns the empty string and nil in err.
// It verifies if staticcheck was installed with "go get -tool" or with "go install".
// If staticcheck was installed with "go get -tool" then run executes "go tool staticcheck".
// Otherwise, if installed with "go install" then run executes "staticcheck". If installed
// with both methods then run executes executes "go tool staticcheck".
func (sp staticcheckPkg) run(ctx context.Context, pkg string) (message string, err error) {
	buf := new(bytes.Buffer)
	buf.WriteString("\tlan: from staticcheck\n")

	cmd, err := sp.command(ctx)
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
		r.message, r.err = sp.withTests(ctx, pkg, cmd)
		chResult <- r
	})
	wg.Go(func() {
		var r result
		r.message, r.err = sp.withoutTests(ctx, pkg, cmd)
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
func (sp staticcheckPkg) command(ctx context.Context) (cmd []string, err error) {
	cmd, err = sp.installedWithGoTool(ctx)
	if cmd != nil {
		return
	}

	cmd, errSys := sp.installedInTheSystem()
	err = cmp.Or(err, errSys)
	return
}

func (sp staticcheckPkg) installedWithGoTool(ctx context.Context) (cmd []string, err error) {
	var stdout strings.Builder
	goToolCmd := sp.os.newCmd(ctx, sp.workDir, &stdout, nil, "go", "tool")
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
func (sp staticcheckPkg) installedInTheSystem() (cmd []string, err error) {
	if _, err = exec.LookPath("staticcheck"); errors.Is(err, exec.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return
	}

	return []string{"staticcheck"}, nil
}

// withTests executes staticcheck considering the tests.
func (sp staticcheckPkg) withTests(ctx context.Context, pkg string, cmd []string) (message string, err error) {
	var b strings.Builder
	cmd = append(cmd, pkg)
	execCmd := sp.os.newCmd(ctx, sp.workDir, &b, nil, cmd[0], cmd[1:]...)

	err = execCmd.Run()
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		return b.String(), nil
	}

	return
}

// withoutTests executes "staticcheck -tests=false -checks=U1000" and returns the message.
//
// With the "-tests=false" parameter a function is not considered used if only tests call it.
// I think that only the U1000 check is afected by "-tests=false".
func (sp staticcheckPkg) withoutTests(ctx context.Context, pkg string, cmd []string) (message string, err error) {
	var b strings.Builder
	cmd = append(cmd, "-tests=false", "-checks=U1000", pkg)
	execCmd := sp.os.newCmd(ctx, sp.workDir, &b, nil, cmd[0], cmd[1:]...)

	err = execCmd.Run()
	if exitError := new(exec.ExitError); errors.As(err, &exitError) {
		return b.String(), nil
	}

	return
}

// tests is a operation that executes the tests.
type tests struct {
	workDir               string
	timeoutForEachPackage time.Duration
	os                    operatingSystem
}

// newTests creates a tests.
func newTests(timeoutForEachPackage time.Duration, workDir string, opSys operatingSystem) operation {
	return tests{
		workDir:               workDir,
		timeoutForEachPackage: timeoutForEachPackage,
		os:                    opSys,
	}
}

func (t tests) run(ctx context.Context, packages []string) (message string, err error) {
	g := newPkgOpGroup()

	for _, pkg := range packages {
		tp := newTestsPkg(t.timeoutForEachPackage, t.workDir, t.os)
		g.executeGo(ctx, tp, pkg)
	}

	return g.wait()
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

// coverageRe is for determining the coverage of the tests of a package.
var coverageRe = regexp.MustCompile(`^coverage: (\d{1,3}(?:\.\d)?)% of statements\n$`)

// testsPkg is a operation that executes the tests.
type testsPkg struct {
	workDir string
	timeout time.Duration
	os      operatingSystem
}

// newTestsPkg creates a testsPkg.
func newTestsPkg(timeout time.Duration, workDir string, opSys operatingSystem) packageOperation {
	return testsPkg{
		workDir: workDir,
		timeout: timeout,
		os:      opSys,
	}
}

// run executes the tests of the package.
func (tp testsPkg) run(ctx context.Context, pkg string) (message string, err error) {
	// even when the tests reports 100.0% of coverage some statements may not have been executed.
	// Then we use the coverprofile to verifiy whether all statements where executed.
	cpf, err := tp.os.createTemp("", "coverprofile*.out")
	if err != nil {
		return
	}
	cpf.Close()                    // we dont need the file now.
	defer tp.os.remove(cpf.Name()) // we assume that the file will not be removed automatically by the system.

	stdout := new(bytes.Buffer)
	to := tp.timeout.String()
	cmd := tp.os.newCmd(ctx,
		tp.workDir, stdout, nil,
		"go", "test", "-json", "-timeout="+to, "-vet=off", "-cover", "-failfast", "-coverprofile="+cpf.Name(), pkg,
	)

	err = cmd.Run()
	if _, isExitError := errors.AsType[*exec.ExitError](err); err != nil && !isExitError {
		return message, fmt.Errorf("\tlan: from executing the tests\n%w", err)
	}

	res, err := tp.result(stdout)
	if err != nil {
		return "", fmt.Errorf("\tlan: from executing the tests\n%w", err)
	}

	return tp.message(res, cpf.Name())
}

// result decodes r into a result.
func (tp testsPkg) result(r io.Reader) (tr testResult, err error) {
	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if err = json.UnmarshalDecode(dec, &te); err == io.EOF {
			return tr, nil
		} else if err != nil {
			return
		}
		if tr = tp.toTestResult(te); tr.kind != testResultIgnore {
			return
		}
	}
}

// toTestResult decodes an event and returns their meaning.
func (tp testsPkg) toTestResult(te testEvent) testResult {
	if te.Action == "fail" && te.Test != "" {
		return testResult{kind: testResultFail, pkg: te.Package, message: fmt.Sprintf("%s: %s failed", te.Package, te.Test)}
	}

	if te.Action == "output" && strings.HasPrefix(te.Output, "panic: test timed out after") {
		return testResult{kind: testResultTimeout, pkg: te.Package, message: fmt.Sprintf("%s: %s", te.Package, te.Output)}
	}

	if te.Action == "output" && coverageRe.MatchString(te.Output) {
		submatches := coverageRe.FindAllStringSubmatch(te.Output, -1)
		if submatches[0][1] == "100.0" {
			return testResult{kind: testResultIgnore, pkg: te.Package}
		}
		return testResult{kind: testResultCoverageNot100PerCent, pkg: te.Package,
			message: fmt.Sprintf("%s: test coverage is not 100.0%%", te.Package)}
	}

	if te.Action == "build-fail" && te.Test == "" && te.Package == "" {
		return testResult{kind: testResultBuildFail, message: "build failed"}
	}

	return testResult{kind: testResultIgnore}
}

// message creates the message corresponding to the results of the execution of the tests,
// and the coverprofile generated.
func (tp testsPkg) message(tr testResult, coverProfileName string) (string, error) {
	if tr.kind == testResultIgnore {
		return tp.messageFromCoverProfile(coverProfileName)
	}
	return "\tlan: from executing the tests\n" + tr.message, nil
}

// messageFromCoverProfile creates the messages corresponding to the coverprofile generated.
func (tp testsPkg) messageFromCoverProfile(fileName string) (message string, err error) {
	f, err := tp.os.open(fileName)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasSuffix(line, "0") {
			return "\tlan: from cover profile\n" + line, nil
		}
	}

	return "", sc.Err()
}

var defaultOS = newOperatingSystem()

// opSystem is for allowing the tests to change the behavior of the packages os and exec.
type operatingSystem struct {
	pathSeparator rune
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
	newCmd func(ctx context.Context, worDir string, stdout, stderr io.Writer, name string, args ...string) command
	// getwd is for os.Getwd
	getwd func() (string, error)
}

func newOperatingSystem() operatingSystem {
	return operatingSystem{
		pathSeparator: os.PathSeparator,
		stdout:        os.Stdout,
		exit:          os.Exit,
		createTemp:    os.CreateTemp,
		remove:        os.Remove,
		open: func(name string) (io.ReadCloser, error) {
			return os.Open(name)
		},
		newCmd: func(ctx context.Context, workDir string, stdout, stderr io.Writer, name string, args ...string) command {
			cmd := exec.CommandContext(ctx, name, args...)
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
