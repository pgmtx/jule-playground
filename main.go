package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
)

func postHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	codeInput, _ := io.ReadAll(r.Body)
	os.WriteFile("main.jule", codeInput, 0644)

	cmd := exec.Command(
		"docker", "run", "--rm",
		"-v", ".:/work",
		"-w", "/work",
		"jule-clang",
		"sh", "-c", "/jule/bin/julec build . && ./main",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error: %v\n%s", err, output), 500)
		return
	}
	w.Write(output)
}

func main() {
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	http.HandleFunc("/send", postHandler)
	fmt.Println("http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
