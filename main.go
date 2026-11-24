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

const (
	programName = "program"
	
	transpileSlots = 3
	cpuSlots = 2

	acquireTimeout = 10 * time.Second
	transpileTimeout = 20 * time.Second
	compileTimeout = 20 * time.Second
	runTimeout = 10 * time.Second
	formatTimeout = 10 * time.Second

	maxBodySize = 1 << 20 // 1 MiB
)

var (
	transpileSemaphore = make(chan struct{}, transpileSlots)
	cpuSemaphore = make(chan struct{}, cpuSlots)
	ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

func acquire(ctx context.Context, semaphore chan struct{}) error {
	ctx, cancel := context.WithTimeout(ctx, acquireTimeout)
	defer cancel()
	select {
	case semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release(semaphore chan struct{}) {
	<-semaphore
}

func dockerCmd(ctx context.Context, name string, tempDir string, useStdin bool, command string, args ...string) *exec.Cmd {
	allArgs := []string{
		"run",
		"--rm",
		"--name", name,
		"--network=none",
		"--memory=512m",
		"--cpus=1",
		"--pids-limit=50", // to avoid fork bombs
		"-u", "1000:1000", // to avoid root access
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--tmpfs=/tmp:size=128m",
	}
	if tempDir != "" {
		allArgs = append(allArgs, "-v", tempDir+":/sandbox")
	}
	if useStdin {
		allArgs = append(allArgs, "-i")
	}
	allArgs = append(allArgs, "--workdir=/sandbox", "jule-clang", command)
	allArgs = append(allArgs, args...)
	cmd := exec.CommandContext(ctx, "docker", allArgs...)
	cmd.Dir = tempDir
	return cmd
}

func runDocker(timeout time.Duration, tempDir string, stdin io.Reader, command string, args ...string) (out []byte, timedOut bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	name := "jule-" + NewToken()
	cmd := dockerCmd(ctx, name, tempDir, stdin != nil, command, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}

	out, err = cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		_ = exec.Command("docker", "kill", name).Run()
		return out, true, err
	}
	return out, false, err
}

func cleanOutput(output []byte) string {
	// Jule error messages use ANSI color codes, so they must be filtered out
	// for web display.
	s := ansi.ReplaceAllString(string(output), "")
	// For some reason, in some error messages there is a null character which
	// is displayed as a square.
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

func getIrCode(tempDir string, data map[string]string, buildArguments []string) error {
	args := []string{"build", "--transpile", "."}
	args = append(args, buildArguments...)

	start := time.Now()
	output, timedOut, err := runDocker(transpileTimeout, tempDir, nil, "julec", args...)
	if timedOut {
		return errors.New("transpilation timed out")
	}

	if err != nil {
		return errors.New(cleanOutput(output))
	}
	
	data["transpilationDuration"] = fmt.Sprintf("Transpilation took %d ms", time.Since(start).Milliseconds())

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
	output, timedOut, err := runDocker(
		compileTimeout, tempDir, nil,
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
		"-o", programName,
		irPath,
	)

	if timedOut {
		return errors.New("compilation timed out")
	}
	
	if err != nil {
		return errors.New(string(output))
	}
	
	return nil
}

func getCodeOutput(tempDir string) (string, error) {
	output, timedOut, err := runDocker(runTimeout, tempDir, nil, "./" + programName)

	if timedOut {
		return "", fmt.Errorf("execution timed out after %s, check for infinite loops or blocking calls", runTimeout)
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

func fail(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"errorMessage": message,
	})
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
}

func transpileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	body, err := readBody(w, r)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	info := make(map[string]string)
	if err := json.Unmarshal(body, &info); err != nil {
		fail(w, http.StatusBadRequest, "Could not parse body parameters")
		return
	}

	inputCode, ok := info["inputCode"]
	if !ok {
		fail(w, http.StatusBadRequest, "Could not find the required body parameter 'inputCode'")
		return
	}

	// Using an invalid key on info returns an empty string
	buildArguments := strings.Fields(info["buildArguments"])

	if err := acquire(r.Context(), transpileSemaphore); err != nil {
		fail(w, http.StatusServiceUnavailable, "Server busy, try again later")
		return
	}
	defer release(transpileSemaphore)

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	success := false
	defer func(){
		if !success {
			_ = os.RemoveAll(tempDir)
		}
	}()

	_ = os.Chmod(tempDir, 0o777)
	codePath := filepath.Join(tempDir, "main.jule")
	if err := os.WriteFile(codePath, []byte(inputCode), 0o644); err != nil {
		fail(w, http.StatusInternalServerError, "Could not write source file")
		return
	}

	data := make(map[string]string)
	if err := getIrCode(tempDir, data, buildArguments); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	success = true
	// We return a token instead of the temporary directory path
	data["tempDir"] = AddSession(tempDir)
	_ = json.NewEncoder(w).Encode(data)
}

func compileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	body, err := readBody(w, r)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	token := strings.TrimSpace(string(body))
	tempDir, ok := GetSession(token)
	if !ok {
		fail(w, http.StatusNotFound, "Unknown or expired session")
		return
	}

	if err := acquire(r.Context(), cpuSemaphore); err != nil {
		fail(w, http.StatusServiceUnavailable, "Server busy, try again later")
		return
	}
	defer release(cpuSemaphore)

	start := time.Now()
	if err := compileIrCode(tempDir); err != nil {
		if dir, ok := TakeSession(token); ok {
			_ = os.RemoveAll(dir)
		}
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	data := map[string]string{
		"compilationDuration": fmt.Sprintf("Compilation took %.2f s", time.Since(start).Seconds()),
		"tempDir": token,
	}
	_ = json.NewEncoder(w).Encode(data)
}

func runHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	body, err := readBody(w, r)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}

	token := strings.TrimSpace(string(body))
	tempDir, ok := TakeSession(token)
	if !ok {
		fail(w, http.StatusNotFound, "Unknown or expired session")
		return
	}

	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if err := acquire(r.Context(), cpuSemaphore); err != nil {
		fail(w, http.StatusServiceUnavailable, "Server busy, try again later")
		return
	}
	defer release(cpuSemaphore)

	codeOutput, err := getCodeOutput(tempDir)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"codeOutput": codeOutput,
	})
}

func formatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	inputCode, err := readBody(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := acquire(r.Context(), transpileSemaphore); err != nil {
		http.Error(w, "Server busy, try again later", http.StatusServiceUnavailable)
		return
	}
	defer release(transpileSemaphore)

	tempDir, err := os.MkdirTemp("", "jule-playground-")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	_ = os.Chmod(tempDir, 0o777)


	ctx, cancel := context.WithTimeout(context.Background(), formatTimeout)
	defer cancel()

	// When an error occurs, julefmt writes to stderr but still returns 0 as an exit code.
	// Therefore you have to check directly stderr and stdout content.
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	name := "jule-" + NewToken()

	cmd := dockerCmd(ctx, name, tempDir, true, "julefmt")
	cmd.Stdin = bytes.NewReader(inputCode)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		_ = exec.Command("docker", "kill", name).Run()
		http.Error(w, "Formatting timed out", http.StatusInternalServerError)
	case err != nil:
		http.Error(w, fmt.Sprintf("Exit error: %v", err), http.StatusInternalServerError)
	case stderr.Len() != 0:
		http.Error(w, stderr.String(), http.StatusInternalServerError)
	case stdout.Len() != 0:
		_, _ = fmt.Fprint(w, stdout.String())
	}
}

func main() {
	port := flag.Int("port", 8080, "server port")
	flag.Parse()

	go CleanupLoop()
	
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/playground/", http.StripPrefix("/playground/", fs))

	http.HandleFunc("/playground/transpile", transpileHandler)
	http.HandleFunc("/playground/compile", compileHandler)
	http.HandleFunc("/playground/run", runHandler)
	http.HandleFunc("/playground/format", formatHandler)

	address := fmt.Sprintf(":%d", *port)
	fmt.Println("http://0.0.0.0" + address + "/playground/")
	log.Fatal(http.ListenAndServe(address, nil))
}
