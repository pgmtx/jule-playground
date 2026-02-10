import { EditorView, basicSetup } from "https://esm.sh/codemirror@6.0.1";
import { javascript } from "https://esm.sh/@codemirror/lang-javascript@6.2.1";

function main() {
  const editor = new EditorView({
    doc: `function hello() {
  console.log("Hello World!");
  return true;
}`,
    extensions: [basicSetup, javascript()],
    parent: document.getElementById("editor"),
  });

  const runButton = document.getElementById("run-button");
  runButton.onclick = () => {
    console.log("Run button clicked!");
    console.log("Code:\n", editor.state.doc.toString());
  };

  document.addEventListener("keydown", (e) => {
    if (e.ctrlKey && e.key === "Enter") {
      runButton.click();
    }
  });
}

main();
