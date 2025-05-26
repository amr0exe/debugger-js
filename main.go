package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

var declarationRegex = regexp.MustCompile(`^\s*(let|const|var)\s+([^=;]+(?:=[^,;]*)?(?:\s*,\s*[^=;]+(?:=[^,;]*)?)*)\s*;?`)

var (
	forLoopRegex   = regexp.MustCompile(`^\s*for\s*\(`)
	whileLoopRegex = regexp.MustCompile(`^\s*while\s*\(`)
	doWhileRegex   = regexp.MustCompile(`^\s*do\s*\{`)
)

type LoopInfo struct {
	Type      string
	Variables []string
}

func detectLoopType(line string) string {
	line = strings.TrimSpace(line)

	if forLoopRegex.MatchString(line) {
		return "for"
	}
	if whileLoopRegex.MatchString(line) {
		return "while"
	}
	if doWhileRegex.MatchString(line) {
		return "do-while"
	}
	return ""
}

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

// Utility: writes current state to output.txt
func writeDebugInfoToFile(debugInfo map[string]any, label string) {
	file, err := os.Create("output.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create output.txt: %v\n", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	fmt.Fprintf(writer, "=== %s ===\n", label)
	for k, v := range debugInfo {
		fmt.Fprintf(writer, "%s: %v\n", k, v)
	}
	writer.Flush()
}

// Function to write loop information to loops.txt
func writeLoopInfoToFile(loopInfos []LoopInfo, allVariables map[string]any) {
	file, err := os.Create("loops.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create loops.txt: %v\n", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	fmt.Fprintf(writer, "=== LOOP ANALYSIS ===\n\n")

	for i, loop := range loopInfos {
		fmt.Fprintf(writer, "Loop %d:\n", i+1)
		fmt.Fprintf(writer, "Type: %s\n", loop.Type)
		fmt.Fprintf(writer, "Variables in scope: {\n")

		// Write only variables that are inside this loop block
		for _, varName := range loop.Variables {
			if value, exists := allVariables[varName]; exists {
				fmt.Fprintf(writer, "  [%s, %v],\n", varName, value)
			}
		}

		fmt.Fprintf(writer, "}\n\n")
	}

	writer.Flush()
}

func setupJsRuntime(vm *goja.Runtime) {
	registry := require.NewRegistry(require.WithGlobalFolders("."))
	registry.Enable(vm)
	console.Enable(vm)
}

func configDebugFunctions(vm *goja.Runtime, debugInfo map[string]any) {
	vm.Set("debug", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		value := call.Argument(1).Export()
		debugInfo[name] = value
		return goja.Undefined()
	})

	vm.Set("__breakpoint", func(call goja.FunctionCall) goja.Value {
		fmt.Println("\n|_| Breakpoint hit! Current variables:")
		for k, v := range debugInfo {
			fmt.Printf("  %s: %v\n", k, v)
		}
		writeDebugInfoToFile(debugInfo, "BREAKPOINT SNAPSHOT")

		fmt.Print("\n|>  Press ENTER to continue...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return goja.Undefined()
	})
}

func instrumentCode(script string) (string, []LoopInfo) {
	lines := strings.Split(script, "\n")
	var instrumented strings.Builder

	var detectedLoops []LoopInfo
	currentLoopIndex := -1
	braceLevel := 0
	inLoop := false

	for _, line := range lines {
		loopType := detectLoopType(line)
		if loopType != "" {
			fmt.Printf("|+| Detected %s loop \n", loopType)
			detectedLoops = append(detectedLoops, LoopInfo{Type: loopType, Variables: []string{}})

			currentLoopIndex = len(detectedLoops) - 1
			inLoop = true
			braceLevel = 0
		}

		if inLoop {
			braceLevel += strings.Count(line, "{")
			braceLevel -= strings.Count(line, "}")

			if braceLevel <= 0 {
				inLoop = false
				currentLoopIndex = -1
			}
		}

		vars := extractVariablesFromLine(line)
		if inLoop && currentLoopIndex >= 0 && len(vars) > 0 {
			detectedLoops[currentLoopIndex].Variables = append(detectedLoops[currentLoopIndex].Variables, vars...)
		}

		if len(vars) > 0 {
			instrumented.WriteString(line)
			for _, v := range vars {
				instrumented.WriteString(fmt.Sprintf("; debug(\"%s\", %s)", v, v))
			}
			instrumented.WriteString("\n")
		} else {
			instrumented.WriteString(line + "\n")
		}
	}

	fmt.Println("\n|||> Instrumented JS code:")
	fmt.Println(instrumented.String())

	return instrumented.String(), detectedLoops
}

func executeAndAnalyze(vm *goja.Runtime, instrumentCode string, debugInfo map[string]any, detectedLoops []LoopInfo) {
	_, err := vm.RunString(instrumentCode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JS Execution Error: %v\n", err)
		os.Exit(1)
	}

	writeDebugInfoToFile(debugInfo, "FINAL SNAPSHOT")

	if len(detectedLoops) > 0 {
		writeLoopInfoToFile(detectedLoops, debugInfo)
		fmt.Printf("\n Detected %d Loop. Loop analysis saved to loops.txt \n", len(detectedLoops))
	}

	fmt.Println("\n |> Final Snapshot: ")
	for k, v := range debugInfo {
		fmt.Printf("   %s: %v \n", k, v)
	}

	fmt.Println("Finished execution... see output.txt file...")
}


func main() {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	loop.RunOnLoop(func(vm *goja.Runtime) {
		setupJsRuntime(vm)

		debugInfo := make(map[string]any)
		var detectedLoops []LoopInfo

		configDebugFunctions(vm, debugInfo)

		scriptContent, err :=  os.ReadFile("script.js")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read script.js: %v\n", err)
			os.Exit(1)
		}

		instrumented, detectedLoops := instrumentCode(string(scriptContent))

		executeAndAnalyze(vm, instrumented, debugInfo, detectedLoops)
	})

}
