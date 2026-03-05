package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
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
		http.Error(w, "Compilation and execution timed out after 30s. Check for infinite loops or blocking calls.", http.StatusInternalServerError)
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
		http.Error(w, "Segmentation fault", http.StatusInternalServerError)
	} else {
		http.Error(w, outputMessage, http.StatusInternalServerError)
	}

	return "", false
}

func compileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	semaphore <- struct{}{}        // acquire
	defer func() { <-semaphore }() // release

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	os.Chmod(tempDir, 0o777)
	codePath := filepath.Join(tempDir, "main.jule")
	inputCode, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	os.WriteFile(codePath, inputCode, 0o644)

	if outputMessage, ok := compileAndRunCode(w, tempDir); ok {
		fmt.Fprint(w, outputMessage)
	}
}

func formatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	inputCode, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// When an error occurs, julefmt writes to stderr but still returns 0 as an exit code.
	// Therefore you have to check directly stderr and stdout content.
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command("julefmt")
	cmd.Stdin = strings.NewReader(string(inputCode))
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	if err != nil {
		errorMessage := fmt.Sprintf("Exit error: %v\n", err)
		http.Error(w, errorMessage, http.StatusInternalServerError)
	} else if stderr.Len() != 0 {
		http.Error(w, stderr.String(), http.StatusInternalServerError)
	} else if stdout.Len() != 0 {
		fmt.Fprint(w, stdout.String())
	}
}

func transpileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tempDir)

	os.Chmod(tempDir, 0o777)
	codePath := filepath.Join(tempDir, "main.jule")
	inputCode, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	os.WriteFile(codePath, inputCode, 0o644)

	cmd := exec.Command("julec", "build", "--transpile", ".")
	cmd.Dir = tempDir
	if err = cmd.Run(); err != nil {
		http.Error(w, "error: could not transpile the code.", http.StatusInternalServerError)
		return
	}

	irPath := filepath.Join(tempDir, "dist", "ir.cpp")
	irCode, err := os.ReadFile(irPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, string(irCode))
}

func main() {
	port := flag.Int("port", 8080, "server port")
	flag.Parse()
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/playground/", http.StripPrefix("/playground/", fs))

	http.HandleFunc("/playground/compile", compileHandler)
	http.HandleFunc("/playground/format", formatHandler)
	http.HandleFunc("/playground/transpile", transpileHandler)
	addr := fmt.Sprintf(":%d", *port)
	fmt.Println("http://0.0.0.0" + addr + "/playground/")
	log.Fatal(http.ListenAndServe(addr, nil))
}
