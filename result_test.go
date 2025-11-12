package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestResults(t *testing.T) {
	cases := []string{
		"ntf", "tfntf", "sptnt", "t", "futo", "pt", "tpt",
		"sne", "ps", "tws", "fws", "oif", "fusptop",
	}
	for _, dir := range cases {
		t.Run(dir, func(t *testing.T) {
			t.Chdir(path.Join("testdata", dir))
			var exitCode int

			defaultOS.stdout = new(bytes.Buffer)
			defaultOS.exit = func(code int) {
				exitCode = code
			}

			main()

			expected, err := os.ReadFile("result.txt")
			if err != nil {
				t.Fatal(err)
			}
			res := checkResults(exitCode, string(expected), defaultOS.stdout.(*bytes.Buffer).String())
			if res != "" {
				t.Error(res)
			}
		})
	}
}

func checkResults(exitCode int, expected, got string) string {
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
		if exitCode != 0 {
			fmt.Fprintf(&message, "exit code = %d, want 0\n", exitCode)
		}
	} else if len(parts) == 0 && len(gotLines) > 0 {
		if exitCode != 1 {
			fmt.Fprintf(&message, "exit code = %d, want 1\n", exitCode)
		}
		fmt.Fprintf(&message, "unexpected: %s\n", strings.Join(gotLines, "\n"))
	} else if len(parts) > 0 && len(gotLines) == 0 {
		if exitCode != 0 {
			fmt.Fprintf(&message, "exit code = %d, want 0\n", exitCode)
		}
		fmt.Fprint(&message, "lan didn't report anything\n")
	} else {
		if exitCode != 1 {
			fmt.Fprintf(&message, "exit code = %d, want 1\n", exitCode)
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
