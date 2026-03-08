import { indentWithTab } from "@codemirror/commands";
import { cpp } from "@codemirror/lang-cpp";
import {
	foldService,
	HighlightStyle,
	indentUnit,
	StreamLanguage,
	syntaxHighlighting,
} from "@codemirror/language";
import { EditorState } from "@codemirror/state";
import { keymap } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { basicSetup, EditorView } from "codemirror";

const isMobile = /Mobi|Android|iPhone|iPad|iPod/i.test(navigator.userAgent);
const runButton = document.getElementById("run-button");
const formatButton = document.getElementById("format-button");
if (isMobile) {
	runButton.innerText = "Run";
	formatButton.innerText = "Format";
}

const types = [
	"bool",
	"int",
	"uint",
	"uintptr",
	"i8",
	"i16",
	"i32",
	"i64",
	"u8",
	"u16",
	"u32",
	"u64",
	"f32",
	"f64",
	"str",
	"any",
	"rune",
	"byte",
	"cmplx64",
	"cmplx128",
	"map",
];

const typeRegex = new RegExp(`\\b(${types.join("|")})\\b`);

const keywords = [
	"type",
	"impl",
	"self",
	"trait",
	"struct",
	"enum",
	"fn",
	"const",
	"let",
	"static",
	"mut",
	"for",
	"in",
	"break",
	"continue",
	"goto",
	"match",
	"fall",
	"if",
	"else",
	"ret",
	"error",
	"use",
	"co",
	"extern",
	"async",
	"await",
	"unsafe",
	"defer",
	"chan",
	"select",
];

const keywordRegex = new RegExp(`\\b(${keywords.join("|")})\\b`);

const style = HighlightStyle.define([
	{ tag: tags.keyword, color: "#9900cc" },
	{ tag: tags.typeName, color: "#cc978e" },
	{ tag: tags.literal, color: "#d7af70" },
	{ tag: tags.string, color: "#cc0000" },
	{ tag: tags.number, color: "#009900" },
	{ tag: tags.comment, color: "#999999", fontStyle: "italic" },
	{ tag: tags.operator, color: "#666666" },
	{ tag: tags.variableName, color: "#000000" },
	{ tag: tags.invalid, color: "#f00", textDecoration: "underline" },

	// For some reason the tag "function" is not accepted, so I had to use another
	// one like `name`.
	{ tag: tags.name, color: "#0066cc" },
]);

const jule = StreamLanguage.define({
	startState() {
		return { inRawStringLiteral: false };
	},
	token(stream, state) {
		if (state.inRawStringLiteral) {
			while (!stream.eol()) {
				if (stream.next() === "`") {
					state.inRawStringLiteral = false;
					break;
				}
			}
			return "string";
		}

		if (stream.eatSpace()) return null;
		if (stream.match(/\/\/.*/)) return "lineComment";
		if (
			stream.match(/`[^`]*`/) ||
			stream.match(/"([^"\\]|\\.)*"/) ||
			stream.match(/'([^']|\\.)'/)
		) {
			return "string";
		}
		if (stream.match(/'[^']*'/)) return "invalid";
		if (stream.match(/\b\d+(\.\d+)?i?\b/)) return "number";
		if (stream.match(typeRegex)) return "typeName";
		if (stream.match(keywordRegex)) return "keyword";
		if (stream.match(/(true|false|nil|iota)/)) return "literal";
		if (stream.match(/[a-zA-Z_]\w*(?=\()/)) return "name";
		if (stream.match(/[+\-*/%:=<>!&|]+/)) return "operator";
		if (stream.match(/[a-zA-Z_]\w*/)) return "variableName";

		if (stream.peek() === "`") {
			stream.next();
			state.inRawStringLiteral = true;
			return "string";
		}

		stream.next();
		return null;
	},
});

const helloWorldCode = `fn main() {
	println("Hello World!")
}`;

const braceFoldService = foldService.of((state, lineStart) => {
	const line = state.doc.lineAt(lineStart);
	if (!line.text.trimEnd().endsWith("{")) {
		return null;
	}

	let depth = 0;
	for (let i = lineStart; i < state.doc.length; ++i) {
		const c = state.sliceDoc(i, i + 1);
		if (c === "{") {
			++depth;
		} else if (c === "}") {
			--depth;
			if (depth === 0) {
				return { from: line.to, to: i };
			}
		}
	}
	return null;
});

const editor = new EditorView({
	doc: helloWorldCode,
	extensions: [
		basicSetup,
		jule,
		syntaxHighlighting(style),
		keymap.of([indentWithTab]), // Handles the tab key
		indentUnit.of("\t"), // To add because by default indentations add spaces
		braceFoldService,
	],
	parent: document.getElementById("editor"),
});

const irEditor = new EditorView({
	extensions: [basicSetup, EditorState.readOnly.of(true), cpp()],
	parent: document.getElementById("ir-editor"),
});

let isCompiling = false;
let isFormatting = false;
let isTranspiling = false;

runButton.onclick = () => {
	if (isCompiling || isFormatting || isTranspiling) {
		return;
	}

	isCompiling = true;

	const outputElement = document.getElementById("output");
	outputElement.textContent = "Compiling...";

	const inputCode = editor.state.doc.toString();

	const start = performance.now();
	fetch("/playground/compile", {
		method: "POST",
		body: inputCode,
		headers: { "Content-Type": "application/json" },
	})
		.then((res) => res.json())
		.then((json) => {
			const end = performance.now();
			const duration = (end - start) / 1000;
			console.log("Compilation took", duration, "s");
			outputElement.textContent = json.codeOutput;

			irEditor.dispatch({
				changes: {
					from: 0,
					to: irEditor.state.doc.length,
					insert: json.irCode,
				},
			});
			isCompiling = false;
		})
		.catch((err) => {
			outputElement.textContent = err;
			isCompiling = false;
		});
};

function assignShortcutToButton(buttonId, shortcutEvent) {
	const button = document.getElementById(buttonId);
	document.addEventListener(
		"keydown",
		(e) => {
			if (shortcutEvent(e)) {
				// This and the capture parameter below prevents from the newline addition
				// inside the code editor.
				e.preventDefault();
				button.click();
			}
		},
		{ capture: true },
	);
}

assignShortcutToButton("run-button", (e) => e.ctrlKey && e.key === "Enter");

formatButton.onclick = () => {
	if (isFormatting || isCompiling || isTranspiling) {
		return;
	}
	isFormatting = true;
	const outputElement = document.getElementById("output");
	const inputCode = editor.state.doc.toString();
	outputElement.textContent = "Formatting...";

	fetch("/playground/format", {
		method: "POST",
		body: inputCode,
		headers: { "Content-Type": "text/plain" },
	})
		.then(async (res) => {
			if (res.status >= 500) {
				const message = await res.text();
				throw message;
			}
			return res.text();
		})
		.then((formattedCode) => {
			editor.dispatch({
				changes: {
					from: 0,
					to: editor.state.doc.length,
					insert: formattedCode,
				},
			});
			outputElement.textContent = "Code formatted successfully.";
			isFormatting = false;
		})
		.catch((err) => {
			outputElement.textContent = err;
			isFormatting = false;
		});
};

assignShortcutToButton("format-button", (e) => e.shiftKey && e.key === "Enter");

// document.getElementById("transpile-button").onclick = () => {
// 	if (isTranspiling || isCompiling || isFormatting) {
// 		return;
// 	}

// 	isTranspiling = true;
// 	const outputElement = document.getElementById("output");
// 	const inputCode = editor.state.doc.toString();
// 	outputElement.textContent = "Transpiling...";

// 	fetch("/playground/transpile", {
// 		method: "POST",
// 		body: inputCode,
// 		headers: { "Content-Type": "text/plain" },
// 	})
// 		.then(async (res) => {
// 			if (res.status >= 500) {
// 				const message = await res.text();
// 				throw message;
// 			}
// 			return res.text();
// 		})
// 		.then((formattedCode) => {
// 			irEditor.dispatch({
// 				changes: {
// 					from: 0,
// 					to: irEditor.state.doc.length,
// 					insert: formattedCode,
// 				},
// 			});
// 			outputElement.textContent = "Code transpiled successfully.";
// 			isTranspiling = false;
// 		})
// 		.catch((err) => {
// 			outputElement.textContent = err;
// 			isTranspiling = false;
// 		});
// };

const examples = document.getElementById("examples");
examples.onchange = (e) => {
	const value = e.target.value;

	let newCode = helloWorldCode;
	switch (value) {
		case "fizzbuzz":
			newCode = `fn main() {
	mut i := 1
	for i <= 16; i++ {
		if i%15 == 0 {
			println("FizzBuzz")
		} else if i%3 == 0 {
			println("Fizz")
		} else if i%5 == 0 {
			println("Buzz")
		}
	}
}`;
			break;
		case "randomness":
			newCode = `use "std/fmt"
use "std/math/rand"

fn main() {
	// Constants are compile-time known values
	const min = 1
	const max = 10

	random_number := rand::IntN(max-min) + min

	// print[ln] doesn't accept multiple arguments, so you have to use fmt::Print
	fmt::Print("Here is a number between ", min, " and ", max, ": ")
	println(random_number)
}`;
			break;
		case "comptime-matching":
			newCode = `fn printKind[T](value: T) {
	const match type T {
	| *int:
		println("int pointer")
	| &int:
		println("int reference")
	| u32:
		println("u32")
	| i32:
		println("i32")
	| u8:
		println("u8")
	| cmplx128:
		println("cmplx128")
	| cmplx64:
		println("cmplx64")
	| []int:
		println("slice of ints")
	| [5]int:
		println("array of 5 ints")
	|:
		panic("unexpected type")
	}
}

fn main() {
	let x: [5]int = [1, 2, 3, 4, 5]
	printKind(x)
	printKind(3 + 4i)
	slice := [2, 3, 4]
	printKind(slice)
}`;
			break;
	}
	editor.dispatch({
		changes: {
			from: 0,
			to: editor.state.doc.length,
			insert: newCode,
		},
	});
};
