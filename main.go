package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var ansi *regexp.Regexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func executeCommand(w http.ResponseWriter, tempDir string, command string, args ...string) (string, bool) {
	cmd := exec.Command(command, args...)
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

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

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.RemoveAll(tempDir)

	codePath := filepath.Join(tempDir, "main.jule")
	codeInput, _ := io.ReadAll(r.Body)
	os.WriteFile(codePath, codeInput, 0644)

	julecPath := os.Getenv("JULEC_PATH")
	if julecPath == "" {
		julecPath = "julec"
	}

	const programName = "program"
	_, ok := executeCommand(w, tempDir, julecPath, "build", "-o", programName, ".")
	if !ok {
		return
	}

	outputStr, ok := executeCommand(w, tempDir, filepath.Join(tempDir, programName))
	if ok {
		fmt.Fprint(w, outputStr)
	}
}

func main() {
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/playground/", http.StripPrefix("/playground/", fs))

	http.HandleFunc("/playground/compile", postHandler)
	fmt.Println("http://0.0.0.0:8080/playground/")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
