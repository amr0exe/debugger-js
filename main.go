package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// 
var declarationRegex = regexp.MustCompile(`^\s*(let|const|var)\s+([^=;]+(?:=[^,;]*)?(?:\s*,\s*[^=;]+(?:=[^,;]*)?)*)\s*;?`)

// Extracts variable names from a valid `let/const/var` line
func extractVariablesFromLine(line string) []string {
	matches := declarationRegex.FindStringSubmatch(line)
	if len(matches) < 3 {
		return nil
	}

	rawVars := matches[2]
	parts := strings.Split(rawVars, ",")
	var result []string

	for _, part := range parts {
		seg := strings.SplitN(part, "=", 2)[0]
		name := strings.TrimSpace(seg)

		if valid, _ := regexp.MatchString(`^[a-zA-Z_$][a-zA-Z0-9_$]*$`, name); valid {
			result = append(result, name)
		}
	}
	return result
}

func main() {
	vm := goja.New()

	// debug variable store
	debugInfo := make(map[string]any)

	// Define debug function inside JS
	vm.Set("debug", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		value := call.Argument(1).Export()
		debugInfo[name] = value
		return goja.Undefined()
	})

	// Load JS
	scriptBytes, err := os.ReadFile("script.js")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read script.js: %v\n", err)
		os.Exit(1)
	}
	lines := strings.Split(string(scriptBytes), "\n")

	var instrumentedCode strings.Builder

	for _, line := range lines {
		vars := extractVariablesFromLine(line)

		if len(vars) > 0 {
			instrumentedCode.WriteString(line)
			for _, v := range vars {
				instrumentedCode.WriteString(fmt.Sprintf("; debug(\"%s\", %s)", v, v))
			}
			instrumentedCode.WriteString("\n")
		} else {
			instrumentedCode.WriteString(line + "\n")
		}
	}

	// print what code is being executed
	fmt.Println(instrumentedCode.String())

	_, err = vm.RunString(instrumentedCode.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "JS Error: %v\n", err)
		os.Exit(1)
	}

	// Write debug info to file
	file, err := os.Create("output.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create output.txt: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for k, v := range debugInfo {
		fmt.Fprintf(writer, "%s: %v\n", k, v)
	}
	writer.Flush()

	fmt.Println("Finished. See output.txt.")
}
