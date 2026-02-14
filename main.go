package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var ansi *regexp.Regexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func executeCommand(w http.ResponseWriter, command string, args ...string) (string, bool) {
	build := exec.Command(command, args...)
	output, err := build.CombinedOutput()

	// Jule error messages use ANSI color codes, so they must be filtered out for
	// web display.
	outputStr := ansi.ReplaceAllString(string(output), "")
	outputStr = strings.TrimSpace(outputStr)
	// For some reason in some error messages there is a null character
	// which is displayed as a square.
	outputStr = strings.ReplaceAll(outputStr, "\x00", "")

	if err == nil {
		return outputStr, true
	}

	if outputStr != "" {
		fmt.Println(outputStr)
		http.Error(w, outputStr, 500)
	} else {
		fmt.Println("Error: ", err.Error())
		http.Error(w, err.Error(), 500)
	}

	return outputStr, false
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	codeInput, _ := io.ReadAll(r.Body)
	os.WriteFile("main.jule", codeInput, 0644)

	julecPath := os.Getenv("JULEC_PATH")
	if julecPath == "" {
		julecPath = "julec"
	}

	const programName = "program"
	_, ok := executeCommand(w, julecPath, "build", "-o", programName, ".")
	if !ok {
		return
	}

	outputStr, ok := executeCommand(w, "./"+programName)
	if ok {
		fmt.Fprint(w, outputStr)
	}
}

func main() {
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	http.HandleFunc("/send", postHandler)
	fmt.Println("http://0.0.0.0:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
