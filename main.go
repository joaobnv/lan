// This command is a Git hook. It is intended to be used as the pre-commit hook.
package main

// Copyright (c) 2025, João Breno. See the license.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"log"
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

var exit = os.Exit

// the version of Lan.
const version = "0.1.0"

func main() {
	denyGit, message := run(runConfig{waitFunc: (*opGroup).waitFirst})
	if denyGit {
		fmt.Fprint(os.Stdout, message)
		// I think that showing the version helps the user because Lan does not receive command line parameters
		// that make it show its version.
		fmt.Fprint(os.Stdout, "\n\tlan version: "+version+"\n")
		exit(1)
	}
}

type runConfig struct {
	workDir     string
	testTimeout time.Duration
	tmpDir      string
	waitFunc    func(*opGroup) (string, error)
}

// run executes the operations.
func run(cfg runConfig) (denyGit bool, message string) {
	if cfg.testTimeout == 0 {
		cfg.testTimeout = 240 * time.Second
	}

	og := newOpGroup()
	ctx := context.Background()

	buildOp := newBuild(cfg.workDir)
	tat := newThereAreTests(cfg.workDir, buildOp)
	vet := newVet(cfg.workDir, buildOp)
	sc := newStaticcheck(cfg.workDir, buildOp)
	ts := newTests(cfg.testTimeout, cfg.workDir, cfg.tmpDir, linters{tat, vet, sc})

	og.executeGo(ctx, buildOp)
	og.executeGo(ctx, tat)
	og.executeGo(ctx, vet)
	og.executeGo(ctx, sc)
	og.executeGo(ctx, ts)

	message, err := cfg.waitFunc(og)
	if message != "" {
		return true, message
	}
	if err != nil {
		return true, strings.TrimRightFunc(err.Error(), unicode.IsSpace)
	}
	return
}

// operation is a operation that can deny a git from proceding.
type operation interface {
	// run executes a operation and returns the message, without white space at end. If the result of the
	// execution doesn't denies git then the empty string will be returned in message.
	run(ctx context.Context) (message string, err error)
}

// opGroup executes operations.
type opGroup struct {
	ops        []operation
	cancellers []context.CancelFunc
	rs         []opGroupOperationResult
	wg         sync.WaitGroup
	resultCh   chan opGroupOperationResult
}

func newOpGroup() *opGroup {
	return &opGroup{resultCh: make(chan opGroupOperationResult)}
}

func (eg *opGroup) executeGo(ctx context.Context, op operation) {
	eg.ops = append(eg.ops, op)
	eg.rs = append(eg.rs, opGroupOperationResult{})
	ctx, cancel := context.WithCancel(ctx)
	eg.cancellers = append(eg.cancellers, cancel)

	pos := len(eg.ops) - 1
	eg.wg.Go(func() {
		message, err := op.run(ctx)
		eg.resultCh <- opGroupOperationResult{
			message: message, err: err, pos: pos,
		}
	})
}

// waitFirst returns the first failure, if any. That is, the first operation result where the message is
// not empty or the error is non nil. First with respect to the order of the calls to executeGo, that is,
// if multiple operations return failures then wait returns the failure of the operation in relation
// to which executeGo was called first.
//
// If there is no failure then waitFirst returns the empty message and the nil error.
func (eg *opGroup) waitFirst() (message string, err error) {
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

// waitAll returns the results of all operations. It respect the order of the calls to executeGo.
// waitAll joins the messages with '\n'. It also join the errors with '\n'.
//
//lint:ignore U1000 to be used only by tests.
func (eg *opGroup) waitAll() (message string, err error) {
	go func() {
		eg.wg.Wait()
		close(eg.resultCh)
	}()

	for res := range eg.resultCh {
		eg.rs[res.pos] = res
	}

	messages := make([]string, 0, len(eg.rs))
	errs := make([]error, 0, len(eg.rs))

	for _, r := range eg.rs {
		if r.message != "" {
			messages = append(messages, r.message)
		}
		if r.err != nil {
			errs = append(errs, r.err)
		}
	}

	return strings.Join(messages, "\n"), errors.Join(errs...)
}

type opGroupOperationResult struct {
	pos     int
	message string
	err     error
}

// build is a operation that executes the build for each package.
type build struct {
	workDir    string
	chOk       chan struct{}
	chComplete chan struct{}
}

func newBuild(workDir string) *build {
	return &build{workDir: workDir, chOk: make(chan struct{}), chComplete: make(chan struct{})}
}

// run builds the packages.
func (b *build) run(ctx context.Context) (message string, err error) {
	defer func() {
		if message == "" && err == nil {
			close(b.chOk)
		}
		close(b.chComplete)
	}()

	stdout, stderr, _ := execCommand(ctx, b.workDir, "go test -c -json -vet=off -o=%s .%c...", os.DevNull, os.PathSeparator)

	if b.noPackages(stderr) {
		return "\tlan: from building\nno Go packages", nil
	}

	if stderr.Len() > 0 {
		msg := strings.TrimRightFunc(stderr.String(), unicode.IsSpace)
		return fmt.Sprintf("\tlan: from building\n%s", msg), nil
	}

	if !b.buildOk(stdout) {
		return "\tlan: from building\nbuild failed", nil
	}

	return
}

func (b *build) wait() bool {
	<-b.chComplete

	select {
	case <-b.chOk:
		return true
	default:
		return false
	}
}

func (b *build) noPackages(stderr *bytes.Buffer) bool {
	for line := range strings.Lines(stderr.String()) {
		if strings.TrimSpace(line) == "no packages to test" {
			return true
		}
	}
	return false
}

// buildOk decodes r and reports whether the build was ok.
func (b *build) buildOk(stdout io.Reader) bool {
	dec := jsontext.NewDecoder(stdout)
	for {
		var te testEvent
		if mustOrEOF(json.UnmarshalDecode(dec, &te)) {
			return true
		}
		if te.Action == "build-fail" {
			return false
		}
	}
}

type linters []interface{ wait() bool }

func (ls linters) wait() bool {
	for _, l := range ls {
		if !l.wait() {
			return false
		}
	}
	return true
}

// thereAreTests check if the packages need and have tests.
type thereAreTests struct {
	workDir    string
	build      *build
	chOk       chan struct{}
	chComplete chan struct{}
}

func newThereAreTests(workDir string, build *build) *thereAreTests {
	return &thereAreTests{workDir: workDir, build: build, chOk: make(chan struct{}), chComplete: make(chan struct{})}
}

// run checks if the packages need and have tests.
func (t *thereAreTests) run(ctx context.Context) (message string, err error) {
	if !t.build.wait() {
		close(t.chComplete)
		return
	}
	defer func() {
		if message == "" && err == nil {
			close(t.chOk)
		}
		close(t.chComplete)
	}()

	for _, d := range t.dirs(ctx) {
		if d.needTests && !d.hasTests {
			return fmt.Sprintf("\tlan: from checking if there are tests\n%s has no tests", d.pkgPath), nil
		}
	}

	return
}

func (t *thereAreTests) wait() bool {
	<-t.chComplete

	select {
	case <-t.chOk:
		return true
	default:
		return false
	}
}

type dir struct {
	needTests     bool
	hasTests      bool
	pkgPath       string
	productionPkg *packages.Package
	testPkg       *packages.Package
}

func (t *thereAreTests) dirs(ctx context.Context) (dirs []*dir) {
	// We dont support folders with multiple non-test packages.
	cfg := &packages.Config{
		Dir:     t.workDir,
		Context: ctx,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypesInfo,
		Tests:   true,
	}
	pkgs := must2(packages.Load(cfg, "."+string(os.PathSeparator)+"..."))
	mustOk(len(pkgs) > 0)

	dirsMap := make(map[string]*dir)
	for _, pkg := range pkgs {
		mustOk(len(pkg.Errors) == 0)

		if strings.HasSuffix(pkg.PkgPath, ".test") {
			continue
		}

		d, ok := dirsMap[pkg.Dir]
		if !ok {
			d = &dir{pkgPath: t.pkgPath(pkg)}
			dirs = append(dirs, d)
			dirsMap[pkg.Dir] = d
		}
		if strings.HasSuffix(pkg.PkgPath, "_test") {
			mustOk(d.testPkg == nil)
			d.testPkg = pkg
		} else {
			if d.productionPkg == nil || slices.ContainsFunc(pkg.GoFiles, func(name string) bool { return strings.HasSuffix(name, "_test.go") }) {
				d.productionPkg = pkg
			}
		}
	}

	for _, d := range dirs {
		d.hasTests = t.hasTests(d)
		d.needTests = t.needTests(d)
	}
	return dirs
}

// needTests reports whether d need tests.
func (t *thereAreTests) needTests(d *dir) bool {
	funcDecl := func(e ast.Decl) bool { _, ok := e.(*ast.FuncDecl); return ok }

	for _, s := range d.productionPkg.Syntax {
		f := d.productionPkg.Fset.File(s.FileStart)
		if strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		if slices.ContainsFunc(s.Decls, funcDecl) {
			return true
		}
	}

	return false
}

func (t *thereAreTests) pkgPath(pkg *packages.Package) string {
	return strings.TrimSuffix(pkg.PkgPath, "_test")
}

func (t *thereAreTests) hasTests(d *dir) bool {
	return t.hasTestInProductionPkg(d.productionPkg) || t.hasTestInTestPkg(d.testPkg)
}

func (t *thereAreTests) hasTestInProductionPkg(pkg *packages.Package) bool {
	var funcDecls []*ast.FuncDecl

	for _, s := range pkg.Syntax {
		f := pkg.Fset.File(s.FileStart)
		if !strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}

		for _, d := range s.Decls {
			if f, ok := d.(*ast.FuncDecl); ok {
				funcDecls = append(funcDecls, f)
			}
		}
	}

	for _, f := range funcDecls {
		if t.isTestFunction(pkg.TypesInfo, f) {
			return true
		}
	}

	return false
}

func (t *thereAreTests) hasTestInTestPkg(pkg *packages.Package) bool {
	if pkg == nil {
		return false
	}

	var funcDecls []*ast.FuncDecl
	for _, s := range pkg.Syntax {
		for _, d := range s.Decls {
			if f, ok := d.(*ast.FuncDecl); ok {
				funcDecls = append(funcDecls, f)
			}
		}
	}

	for _, f := range funcDecls {
		if t.isTestFunction(pkg.TypesInfo, f) {
			return true
		}
	}

	return false
}

// isTestFunction reports wheter f has the signature of a test function.
func (t *thereAreTests) isTestFunction(ti *types.Info, f *ast.FuncDecl) bool {
	return t.hasTestSignature(ti, f, "Test", "T") || t.isFuzzTestFunction(ti, f)
}

// isFuzzTestFunction reports wheter f has the signature of a fuzz test function.
func (t *thereAreTests) isFuzzTestFunction(ti *types.Info, f *ast.FuncDecl) bool {
	return t.hasTestSignature(ti, f, "Fuzz", "F")
}

func (t *thereAreTests) hasTestSignature(ti *types.Info, f *ast.FuncDecl, prefix string, paramTypeName string) bool {
	if !strings.HasPrefix(f.Name.Name, prefix) {
		return false
	}

	if t.startWithLowerCaseLetter(strings.TrimPrefix(f.Name.Name, prefix)) {
		return false
	}

	sig := ti.Defs[f.Name].Type().(*types.Signature)
	mustOk(sig.Params().Len() == 1)

	typePtr := mustOkType[*types.Pointer](sig.Params().At(0).Type())
	var name string
	switch elem := typePtr.Elem().(type) {
	case *types.Named:
		name = elem.Obj().Name()
	case *types.Alias:
		name = elem.Obj().Name()
	}
	mustOk(name == paramTypeName)

	elem := types.Unalias(typePtr.Elem())
	typeNamed := mustOkType[*types.Named](elem)
	mustOk(typeNamed.Obj().Name() == paramTypeName)

	obj := typeNamed.Obj()
	mustOk(obj.Pkg() != nil && obj.Pkg().Path() == "testing")

	return true
}

// startWithLowerCaseLetter reports if s start with a lower case letter.
func (t *thereAreTests) startWithLowerCaseLetter(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLower(r)
}

// vet is a operation that executes "go vet" in each package.
type vet struct {
	workDir    string
	build      *build
	chOk       chan struct{}
	chComplete chan struct{}
}

func newVet(workDir string, build *build) *vet {
	return &vet{workDir: workDir, build: build, chOk: make(chan struct{}), chComplete: make(chan struct{})}
}

// run executes "go vet ./...".
func (v *vet) run(ctx context.Context) (message string, err error) {
	if !v.build.wait() {
		close(v.chComplete)
		return
	}
	defer func() {
		if message == "" && err == nil {
			close(v.chOk)
		}
		close(v.chComplete)
	}()

	_, stderr, exitError := execCommand(ctx, v.workDir, "go vet .%c...", os.PathSeparator)
	if exitError {
		message = strings.TrimRightFunc("\tlan: from go vet\n"+stderr.String(), unicode.IsSpace)
	}

	return
}

func (v *vet) wait() bool {
	<-v.chComplete

	select {
	case <-v.chOk:
		return true
	default:
		return false
	}
}

// staticcheck is a operation that executes the staticcheck linter in the packages, if it is installed.
type staticcheck struct {
	workDir    string
	build      *build
	chOk       chan struct{}
	chComplete chan struct{}
}

// newStaticcheck creates a staticcheck.
func newStaticcheck(workDir string, build *build) *staticcheck {
	return &staticcheck{workDir: workDir, build: build, chOk: make(chan struct{}), chComplete: make(chan struct{})}
}

// run executes "staticcheck" and returns the message.
// If the staticcheck is not installed then run returns the empty string and nil in err.
// It verifies if staticcheck was installed with "go get -tool" or with "go install".
// If staticcheck was installed with "go get -tool" then run executes "go tool staticcheck".
// Otherwise, if installed with "go install" then run executes "staticcheck". If installed
// with both methods then run executes executes "go tool staticcheck".
func (s *staticcheck) run(ctx context.Context) (message string, err error) {
	if !s.build.wait() {
		close(s.chComplete)
		return
	}
	defer func() {
		if message == "" && err == nil {
			close(s.chOk)
		}
		close(s.chComplete)
	}()

	format, args := s.command(ctx)
	if format == "" {
		return
	}

	type result struct {
		message   string
		withTests bool
	}

	chResult := make(chan result)
	var wg sync.WaitGroup
	wg.Go(func() {
		var r result
		r.withTests = true
		r.message = s.withTests(ctx, format, args...)
		chResult <- r
	})
	wg.Go(func() {
		var r result
		r.message = s.withoutTests(ctx, format, args...)
		chResult <- r
	})

	go func() {
		wg.Wait()
		close(chResult)
	}()

	results := make([]result, 2)
	for r := range chResult {
		if r.withTests {
			// withTests has precedence, so if there are a function not used by the test and the non-test code
			// then it wont be reported two times.
			results[0] = r
		} else {
			results[1] = r
		}
	}

	for i := range 2 {
		if results[i].message != "" {
			return "\tlan: from staticcheck\n" + strings.TrimRightFunc(results[i].message, unicode.IsSpace), nil
		}
	}

	return
}

func (s *staticcheck) wait() bool {
	<-s.chComplete

	select {
	case <-s.chOk:
		return true
	default:
		return false
	}
}

// command determines what command to use, "go tool staticcheck" or "staticcheck".
func (s *staticcheck) command(ctx context.Context) (format string, args []any) {
	if format, args = s.installedWithGoTool(ctx); format != "" {
		return
	}
	return s.installedInTheSystem()
}

func (s *staticcheck) installedWithGoTool(ctx context.Context) (format string, args []any) {
	stdout, _, exitError := execCommand(ctx, s.workDir, "go tool")
	mustOk(!exitError)

	for line := range strings.Lines(stdout.String()) {
		if strings.TrimSpace(line) == "staticcheck (honnef.co/go/tools/cmd/staticcheck)" {
			return "go tool staticcheck", nil
		}
	}

	return
}

// maybe installed with "go install".
func (s *staticcheck) installedInTheSystem() (format string, args []any) {
	var err error
	if _, err = exec.LookPath("staticcheck"); errors.Is(err, exec.ErrNotFound) {
		return
	}
	must(err)
	return "staticcheck", nil
}

// withTests executes staticcheck considering the tests.
func (s *staticcheck) withTests(ctx context.Context, format string, args ...any) (message string) {
	args = append(args, os.PathSeparator)
	stdout, _, _ := execCommand(ctx, s.workDir, format+" .%c...", args...)
	return stdout.String()
}

// withoutTests executes "staticcheck -tests=false -checks=U1000" and returns the message.
//
// With the "-tests=false" parameter a function is not considered used if only tests call it.
// I think that only the U1000 check is afected by "-tests=false".
func (s *staticcheck) withoutTests(ctx context.Context, format string, args ...any) (message string) {
	args = append(args, os.PathSeparator)
	stdout, _, _ := execCommand(ctx, s.workDir, format+" -tests=false -checks=U1000 .%c...", args...)
	return stdout.String()
}

// packageOperation is operates in only one packages.
type packageOperation interface {
	// run executes a operation and returns the message, without white space at end. If the result of the
	// execution doesn't denies git then the empty string will be returned in message.
	run(ctx context.Context, pkg string) (message string, err error)
}

// pkgpkgOpGroup executes package operations.
type pkgOpGroup struct {
	ops        []packageOperation
	cancellers []context.CancelFunc
	rs         []pkgOpGroupOperationResult
	wg         sync.WaitGroup
	resultCh   chan pkgOpGroupOperationResult
}

func newPkgOpGroup() *pkgOpGroup {
	return &pkgOpGroup{resultCh: make(chan pkgOpGroupOperationResult)}
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
			message: message, err: err, pos: pos,
		}
	})
}

// wait returns the first failure, if any. That is, the first operation result where the message
// is not empty or the error is non nil. Wait not respect to the order of the calls to executeGo,
// that is, if multiple operations return failures then wait returns the failure of the first operation
// that returned.
//
// If there is no failure then wait returns the empty message and the nil error.
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
	pos     int
	message string
	err     error
}

// tests is a operation that executes the tests.
type tests struct {
	workDir               string
	timeoutForEachPackage time.Duration
	tmpDir                string
	linters               linters
}

// newTests creates a tests.
func newTests(timeoutForEachPackage time.Duration, workDir, tmpDir string, linters linters) *tests {
	return &tests{
		workDir:               workDir,
		timeoutForEachPackage: timeoutForEachPackage,
		tmpDir:                tmpDir,
		linters:               linters,
	}
}

func (t *tests) run(ctx context.Context) (message string, err error) {
	if !t.linters.wait() {
		return
	}

	pkgs := t.listPackages(ctx)
	mustOk(len(pkgs) > 0)

	g := newPkgOpGroup()
	for _, pkg := range pkgs {
		tp := newTestsPkg(t.timeoutForEachPackage, t.workDir, t.tmpDir)
		g.executeGo(ctx, tp, pkg)
	}

	return g.wait()
}

func (t *tests) listPackages(ctx context.Context) (pkgs []string) {
	stdout, stderr, _ := execCommand(ctx, t.workDir, "go list .%c...", os.PathSeparator)
	mustOk(stderr.Len() == 0)

	for line := range strings.Lines(stdout.String()) {
		pkgs = append(pkgs, strings.TrimSpace(line))
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

// note that they are in the order of the sorting, (except the testResultIgnore).
const (
	testResultIgnore testResultKind = iota
	testResultFail
	testResultTimeout
	testResultCoverageNot100PerCent
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
	tmpDir  string
}

// newTestsPkg creates a testsPkg.
func newTestsPkg(timeout time.Duration, workDir, tmpDir string) *testsPkg {
	return &testsPkg{
		workDir: workDir,
		tmpDir:  tmpDir,
		timeout: timeout,
	}
}

// run executes the tests of the package.
func (tp *testsPkg) run(ctx context.Context, pkg string) (message string, err error) {
	// even when the tests reports 100.0% of coverage some statements may not have been executed.
	// Then we use the coverprofile to verifiy whether all statements were executed.
	cpf, err := os.CreateTemp(tp.tmpDir, "coverprofile*.out")
	if err != nil {
		return "", fmt.Errorf("\tlan: from executing the tests\n%w", err)
	}
	cpf.Close()                 // we dont need the file now.
	defer os.Remove(cpf.Name()) // we assume that the file will not be removed automatically by the system.

	stdout, _, _ := execCommand(ctx, tp.workDir, "go test -json -timeout=%s -vet=off -cover -failfast -coverprofile=%s %s", tp.timeout, cpf.Name(), pkg)

	res := tp.result(stdout)
	return tp.message(res, cpf.Name())
}

// result decodes r into a result.
func (tp *testsPkg) result(r io.Reader) (tr testResult) {
	dec := jsontext.NewDecoder(r)
	for {
		var te testEvent
		if mustOrEOF(json.UnmarshalDecode(dec, &te)) {
			return tr
		}
		c := tp.toTestResult(te)
		if c.kind != testResultIgnore && (tr.kind == testResultIgnore || c.kind < tr.kind) {
			tr = c
		}
	}
}

// toTestResult decodes an event and returns their meaning.
func (tp *testsPkg) toTestResult(te testEvent) testResult {
	if te.Action == "fail" && te.Test != "" {
		return testResult{kind: testResultFail, pkg: te.Package, message: fmt.Sprintf("%s: %s failed", te.Package, te.Test)}
	}

	if te.Action == "output" && strings.HasPrefix(te.Output, "panic: test timed out after") {
		return testResult{kind: testResultTimeout, pkg: te.Package, message: fmt.Sprintf("%s: %s", te.Package, strings.TrimSpace(te.Output))}
	}

	if te.Action == "output" && coverageRe.MatchString(te.Output) {
		submatches := coverageRe.FindAllStringSubmatch(te.Output, -1)
		if submatches[0][1] == "100.0" {
			return testResult{kind: testResultIgnore, pkg: te.Package}
		}
		return testResult{kind: testResultCoverageNot100PerCent, pkg: te.Package,
			message: fmt.Sprintf("%s: test coverage is not 100.0%%", te.Package)}
	}

	mustOk(!(te.Action == "build-fail" && te.Test == "" && te.Package == ""))

	return testResult{kind: testResultIgnore}
}

// message creates the message corresponding to the result of the execution of the tests,
// and the coverprofile generated.
func (tp *testsPkg) message(tr testResult, coverProfileName string) (string, error) {
	if tr.kind == testResultIgnore {
		return tp.messageFromCoverProfile(coverProfileName)
	}
	return "\tlan: from executing the tests\n" + tr.message, nil
}

// messageFromCoverProfile creates the messages corresponding to the coverprofile generated.
func (tp *testsPkg) messageFromCoverProfile(fileName string) (message string, err error) {
	f, err := os.Open(fileName)
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
	must(sc.Err())

	return "", nil
}

func execCommand(ctx context.Context, dir string, format string, args ...any) (stdout, stderr *bytes.Buffer, exitError bool) {
	s := fmt.Sprintf(format, args...)
	parts := strings.Split(s, " ")

	name := parts[0]
	arguments := parts[1:]

	stdout, stderr = new(bytes.Buffer), new(bytes.Buffer)

	cmd := exec.CommandContext(ctx, name, arguments...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitError = mustOrExitError(cmd.Run())

	return
}

func mustOrEOF(err error) (eof bool) {
	if err == io.EOF {
		return true
	}
	must(err)
	return false
}

func mustOrExitError(err error) bool {
	_, isExitError := errors.AsType[*exec.ExitError](err)
	if err != nil && isExitError {
		return true
	}
	must(err)
	return false
}

func must2[T any](v T, err error) T {
	must(err)
	return v
}

func must(err error) {
	mustLogger(err, log.Default())
}

func mustOkType[T any](v any) T {
	vt, ok := v.(T)
	mustOk(ok)
	return vt
}

func mustOk(ok bool) {
	mustOkLogger(ok, log.Default())
}

func mustLogger(err error, logger interface{ Fatalf(string, ...any) }) {
	if err != nil {
		logger.Fatalf(
			"\tlan: internal error:\n%s\n"+
				"please, consider reporting this as a Github issue on the Lan repository\n"+
				"\tlan version: %s", err.Error(), version,
		)
	}
}

func mustOkLogger(ok bool, logger interface{ Fatalf(string, ...any) }) {
	if !ok {
		logger.Fatalf(
			"\tlan: internal error\n"+
				"please, consider reporting this as a Github issue on the Lan repository\n"+
				"\tlan version: %s", version,
		)
	}
}
