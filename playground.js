function main() {
    const codearea = document.getElementById("input-code");
    codearea.value = 'fn main() {\n    println("Hello world!")\n}\n';

    const runButton = document.getElementById("run-button");
    runButton.onclick = () => {
        alert("you clicked run button");
    };

    document.addEventListener("keydown", (e) => {
      if (e.ctrlKey && e.key === "Enter") {
        runButton.click();
      }
    })
}

main();
