package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestResultMain(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"ntf", "fusptop",
	}
	for _, dir := range cases {
		t.Run(dir, func(t *testing.T) {
			t.Chdir(filepath.Join("testdata", dir))

			defaultOS.stdout = new(strings.Builder)
			var exitCode int
			defaultOS.exit = func(code int) {
				exitCode = code
			}

			main()

			denyGit := exitCode != 0
			message := defaultOS.stdout.(*strings.Builder).String()

			expected, err := os.ReadFile(filepath.Join(wd, "testdata", dir, "result.txt"))
			if err != nil {
				t.Fatal(err)
			}
			res := checkResults(denyGit, string(expected), message)
			if res != "" {
				t.Error(res)
			}
		})
	}
}

func TestResults(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"ntf", "tfntf", "sptnt", "t", "futo", "pt", "tpt", "sne",
		"ps", "tws", "fws", "oif", "fusptop", "se", "pmntp",
	}
	for _, dir := range cases {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			denyGit, message := run(filepath.Join(wd, "testdata", dir))

			expected, err := os.ReadFile(filepath.Join(wd, "testdata", dir, "result.txt"))
			if err != nil {
				t.Fatal(err)
			}
			res := checkResults(denyGit, string(expected), message)
			if res != "" {
				t.Error(res)
			}
		})
	}
}

func TestTests(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{"to"}
	for _, dir := range cases {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			testsFilePath := filepath.Join(wd, "testdata", dir, "tests.txt")
			if !fileExists(testsFilePath) {
				return
			}

			data, err := os.ReadFile(testsFilePath)
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(data, []byte("\n"))
			timeout := time.Duration(-1)
			var remove int
			for i := range lines {
				if bytes.HasPrefix(lines[i], []byte("timeout")) {
					_, d, ok := bytes.Cut(lines[i], []byte("="))
					if !ok {
						t.Fatalf("syntax error: missing '=' in %q", string(bytes.TrimSpace(lines[i])))
					}
					to, err := time.ParseDuration(string(bytes.TrimSpace(d)))
					if err != nil {
						t.Fatalf("syntax error: %s", err)
					}
					if timeout != -1 {
						t.Fatal("syntax error: timeout redeclared")
					}
					timeout = to
					remove = i + 1
				} else if bytes.HasPrefix(lines[i], []byte("\tlan: ")) {
					break
				}
			}

			expected := bytes.Join(lines[remove:], []byte("\n"))

			if timeout == -1 {
				timeout = 30 * time.Second
			}

			op := newTests(timeout, filepath.Join(wd, "testdata", dir))
			message, err := op.run()
			denyGit := message != "" || err != nil
			if err != nil {
				message = strings.TrimRightFunc(err.Error(), unicode.IsSpace)
			}

			res := checkResults(denyGit, string(expected), message)
			if res != "" {
				t.Error(res)
			}
		})
	}
}

// fileExists reports whether a file exists.
func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !errors.Is(err, os.ErrNotExist)
}

func checkResults(denyGit bool, expected, got string) string {
	expLines := slices.Collect(strings.Lines(expected))
	gotLines := slices.Collect(strings.Lines(got))

	type part struct {
		head    string
		regs    []*regexp.Regexp
		all     bool
		ordered bool
	}

	var parts []*part
	var current *part
	for _, expLine := range expLines {
		if strings.HasPrefix(expLine, "\tlan: ") {
			current = &part{all: true, ordered: true}

			expLineParts := strings.SplitN(expLine, "(", 2)
			if len(expLineParts) == 2 {
				expLine = expLineParts[0]
				optionsStr := expLineParts[1]
				optionsStr = strings.TrimSpace(optionsStr)
				optionsStr = strings.TrimSuffix(optionsStr, ")")
				optionsStrParts := strings.Split(optionsStr, ",")
				for _, p := range optionsStrParts {
					switch p := strings.TrimSpace(p); p {
					case "not all":
						current.all = false
					case "unordered":
						current.ordered = false
					default:
						return fmt.Sprintf("unkonwn option near %q", p)
					}
				}
			}
			parts = append(parts, current)
			current.head = strings.TrimSpace(expLine)
		} else if !unicode.IsSpace([]rune(expLine)[0]) {
			if current == nil {
				return fmt.Sprintf("syntax error: unexpected %q\n", expLine)
			}
			expLine = strings.TrimSpace(expLine)
			reg, err := regexp.Compile(expLine)
			if err != nil {
				return err.Error()
			}
			current.regs = append(current.regs, reg)
		} else {
			return fmt.Sprintf("syntax error: %q", expLine)
		}
	}

	var message strings.Builder
	if len(parts) == 0 && len(gotLines) == 0 {
		if denyGit {
			message.WriteString("denyGit = true, want false\n")
		}
	} else if len(parts) == 0 && len(gotLines) > 0 {
		if denyGit {
			message.WriteString("denyGit = true, want false\n")
		}
		fmt.Fprintf(&message, "unexpected: %s\n", strings.Join(gotLines, "\n"))
	} else if len(parts) > 0 && len(gotLines) == 0 {
		if !denyGit {
			message.WriteString("denyGit = false, want true\n")
		}
		fmt.Fprint(&message, "lan didn't report anything\n")
	} else {
		if !denyGit {
			message.WriteString("denyGit = false, want true\n")
		}
	}

	var gotIndex, partIndex int
	var executingRegs bool
	for gotIndex < len(gotLines) && partIndex < len(parts) {
		gotLine := gotLines[gotIndex]
		if strings.HasPrefix(gotLine, "\tlan: ") {
			if executingRegs {
				executingRegs = false
				p := parts[partIndex]
				for _, reg := range p.regs {
					fmt.Fprintf(&message, "regexp not matched: %q\n", reg.String())
				}
				partIndex++
				if partIndex >= len(parts) {
					break
				}
			}
			if parts[partIndex].head == strings.TrimSpace(gotLine) {
				gotIndex++
			} else {
				fmt.Fprintf(&message, "%q != %q\n", strings.TrimSpace(gotLine), parts[partIndex].head)
				partIndex++
			}
		} else if strings.HasPrefix(gotLine, "\tlan version: ") {
			if executingRegs {
				executingRegs = false
				p := parts[partIndex]
				for _, reg := range p.regs {
					fmt.Fprintf(&message, "regexp not matched: %q\n", reg.String())
				}
				partIndex++
			}
			gotIndex++
		} else {
			executingRegs = true
			p := parts[partIndex]
			gotLine = strings.TrimSpace(gotLine)

			if len(p.regs) == 0 {
				if p.all {
					fmt.Fprintf(&message, "unexpected %q\n", gotLine)
				}
				gotIndex++
				continue
			}

			if p.ordered {
				if p.all {
					if !p.regs[0].MatchString(gotLine) {
						fmt.Fprintf(&message, "unexpected %q\n", gotLine)
					}
					p.regs = slices.Delete(p.regs, 0, 1)
				} else if p.regs[0].MatchString(gotLine) {
					p.regs = slices.Delete(p.regs, 0, 1)
				}
			} else {
				if p.all {
					var matched bool
					for i, reg := range p.regs {
						if reg.MatchString(gotLine) {
							p.regs = slices.Delete(p.regs, i, i+1)
							matched = true
							break
						}
					}
					if !matched {
						fmt.Fprintf(&message, "unexpected %q\n", gotLine)
					}
				} else {
					for i, reg := range p.regs {
						if reg.MatchString(gotLine) {
							p.regs = slices.Delete(p.regs, i, i+1)
							break
						}
					}
				}
			}
			gotIndex++
		}
	}
	return message.String()
}
