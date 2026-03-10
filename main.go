package main

import (
	"bytes"
	"context"
	"encoding/json"
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
const programName = "program"

var semaphore = make(chan struct{}, maxConcurrentCompilations)
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func executeIsolatedCommand(tempDir string, command string, commandArgs ...string) *exec.Cmd {
	allArgs := []string{
		"run",
		"--rm",
		"--network=none",
		"--memory=512m",
		"--cpus=1",
		"--pids-limit=50", // to avoid fork bombs
		"-u", "1000:1000", // to avoid root access
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--tmpfs=/tmp:size=128m",
		"-v", tempDir + ":/sandbox",
		"--workdir=/sandbox",
		"jule-clang",
		command,
	}
	allArgs = append(allArgs, commandArgs...)
	cmd := exec.Command("docker", allArgs...)
	cmd.Dir = tempDir
	return cmd
}

func getIrCode(tempDir string, data map[string]string) error {
	transpileCmd := executeIsolatedCommand(tempDir, "julec", "build", "--transpile", ".")
	start := time.Now()
	if output, err := transpileCmd.CombinedOutput(); err != nil {
		// Jule error messages use ANSI color codes, so they must be filtered out for
		// web display.
		outputMessage := ansi.ReplaceAllString(string(output), "")
		outputMessage = strings.TrimSpace(outputMessage)
		// For some reason in some error messages there is a null character
		// which is displayed as a square.
		outputMessage = strings.ReplaceAll(outputMessage, "\x00", "")
		return errors.New(outputMessage)
	}
	duration := time.Since(start)
	data["transpilationDuration"] = fmt.Sprintf("Transpilation took %dms", duration.Milliseconds())

	irPath := filepath.Join(tempDir, "dist", "ir.cpp")
	irCode, err := os.ReadFile(irPath)
	if err != nil {
		return err
	}

	data["irCode"] = string(irCode)
	return nil
}

func compileIrCode(tempDir string) error {
	irPath := filepath.Join("dist", "ir.cpp")
	compileCmd := executeIsolatedCommand(
		tempDir,
		"clang++",
		"-Wno-everything",
		"--std=c++20",
		"-fwrapv",
		"-ffloat-store",
		"-fno-fast-math",
		"-fexcess-precision=standard",
		"-fno-rounding-math",
		"-ffp-contract=fast",
		"-O0",
		"-fno-strict-aliasing",
		"-o",
		programName,
		irPath,
	)
	output, err := compileCmd.CombinedOutput()
	if err != nil {
		return errors.New(string(output))
	}
	return nil
}

func getCodeOutput(tempDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	containerName := filepath.Base(tempDir)
	runCmd := exec.CommandContext(
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
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--tmpfs=/tmp:size=128m",
		"-v", tempDir+":/sandbox",
		"--workdir=/sandbox",
		"jule-clang",
		"./program",
	)
	runCmd.Dir = tempDir
	output, err := runCmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		killCmd := exec.Command("docker", "kill", containerName)
		_ = killCmd.Run()
		return "", errors.New("execution timed out after 30s, check for infinite loops or blocking calls")
	}

	if err == nil {
		return string(output), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 139 {
		return "", errors.New("segmentation fault")
	}
	return "", errors.New(string(output))
}

func generateHttpError(w http.ResponseWriter, data map[string]string, errorMessage string) {
	w.WriteHeader(http.StatusBadRequest)
	data["errorMessage"] = errorMessage
	_ = json.NewEncoder(w).Encode(data)
}

func transpileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data := make(map[string]string)

	if r.Method != http.MethodPost {
		generateHttpError(w, data, "Method not allowed")
		return
	}

	semaphore <- struct{}{}        // acquire
	defer func() { <-semaphore }() // release

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		generateHttpError(w, data, err.Error())
		return
	}

	_ = os.Chmod(tempDir, 0o777)
	codePath := filepath.Join(tempDir, "main.jule")
	inputCode, err := io.ReadAll(r.Body)
	if err != nil {
		generateHttpError(w, data, err.Error())
		return
	}
	if err := os.WriteFile(codePath, inputCode, 0o644); err != nil {
		generateHttpError(w, data, "Could not write to "+codePath)
		return
	}

	if err := getIrCode(tempDir, data); err != nil {
		generateHttpError(w, data, err.Error())
		return
	}
	data["tempDir"] = tempDir
	_ = json.NewEncoder(w).Encode(data)
}

func compileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data := make(map[string]string)

	if r.Method != http.MethodPost {
		generateHttpError(w, data, "Method not allowed")
		return
	}

	tempDirBytes, err := io.ReadAll(r.Body)
	if err != nil {
		generateHttpError(w, data, err.Error())
		return
	}
	tempDir := string(tempDirBytes)

	start := time.Now()
	if err := compileIrCode(tempDir); err != nil {
		generateHttpError(w, data, err.Error())
		return
	}
	duration := time.Since(start)
	data["compilationDuration"] = fmt.Sprintf("Compilation took %.2fs", duration.Seconds())
	data["tempDir"] = tempDir
	_ = json.NewEncoder(w).Encode(data)
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data := make(map[string]string)

	if r.Method != http.MethodPost {
		generateHttpError(w, data, "Method not allowed")
		return
	}

	tempDirBytes, err := io.ReadAll(r.Body)
	if err != nil {
		generateHttpError(w, data, err.Error())
		return
	}
	tempDir := string(tempDirBytes)

	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	codeOutput, err := getCodeOutput(tempDir)
	if err != nil {
		generateHttpError(w, data, err.Error())
		return
	}
	data["codeOutput"] = codeOutput
	_ = json.NewEncoder(w).Encode(data)
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
		errorMessage := fmt.Sprintf("Exit error: %v", err)
		http.Error(w, errorMessage, http.StatusInternalServerError)
	} else if stderr.Len() != 0 {
		http.Error(w, stderr.String(), http.StatusInternalServerError)
	} else if stdout.Len() != 0 {
		_, _ = fmt.Fprint(w, stdout.String())
	}
}

func main() {
	port := flag.Int("port", 8080, "server port")
	flag.Parse()
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/playground/", http.StripPrefix("/playground/", fs))

	http.HandleFunc("/playground/transpile", transpileHandler)
	http.HandleFunc("/playground/compile", compileHandler)
	http.HandleFunc("/playground/run", runHandler)
	http.HandleFunc("/playground/format", formatHandler)

	addr := fmt.Sprintf(":%d", *port)
	fmt.Println("http://0.0.0.0" + addr + "/playground/")
	log.Fatal(http.ListenAndServe(addr, nil))
}
