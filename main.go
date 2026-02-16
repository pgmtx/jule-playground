package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxConcurrentCompilations = 2

var semaphore = make(chan struct{}, maxConcurrentCompilations)
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func compileAndRunCode(w http.ResponseWriter, tempDir string) (string, bool) {
	// Useful to handle infinite loops
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const programName = "program"
	juleCommand := fmt.Sprintf("/jule/bin/julec build -o %s . && ./%s", programName, programName)
	containerName := filepath.Base(tempDir)

	cmd := exec.CommandContext(
		ctx,
		"docker", "run",
		"--rm",
		"--name", containerName,
		"--network=none",
		"--memory=512m",
		"--cpus=1",
		"--pids-limit=50", // to avoid fork bombs
		"-u", "1000:1000", // to avoid root access
		"--read-only",
		"--tmpfs=/tmp:size=128m",
		"--tmpfs=/root:size=16m",
		"-v", tempDir+":/sandbox",
		"--workdir=/sandbox",
		"jule-clang",
		"sh", "-c", juleCommand,
	)
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		killCmd := exec.Command("docker", "kill", containerName)
		_ = killCmd.Run()
		http.Error(w, "Compilation and execution timed out after 30s. Check for infinite loops or blocking calls.", 500)
		return "", false
	}

	// Jule error messages use ANSI color codes, so they must be filtered out for
	// web display.
	outputMessage := ansi.ReplaceAllString(string(output), "")
	outputMessage = strings.TrimSpace(outputMessage)
	// For some reason in some error messages there is a null character
	// which is displayed as a square.
	outputMessage = strings.ReplaceAll(outputMessage, "\x00", "")

	if err == nil {
		return outputMessage, true
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 139 {
		http.Error(w, "Segmentation fault", 500)
	} else {
		http.Error(w, outputMessage, 500)
	}

	return "", false
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	semaphore <- struct{}{}        // acquire
	defer func() { <-semaphore }() // release

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.RemoveAll(tempDir)

	os.Chmod(tempDir, 0777)
	codePath := filepath.Join(tempDir, "main.jule")
	codeInput, _ := io.ReadAll(r.Body)
	os.WriteFile(codePath, codeInput, 0644)

	if outputMessage, ok := compileAndRunCode(w, tempDir); ok {
		fmt.Fprint(w, outputMessage)
	}
}

func main() {
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/playground/", http.StripPrefix("/playground/", fs))

	http.HandleFunc("/playground/compile", postHandler)
	fmt.Println("http://0.0.0.0:8080/playground/")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
