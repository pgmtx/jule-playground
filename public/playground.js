import { indentWithTab } from "https://esm.sh/@codemirror/commands";
import {
	HighlightStyle,
	StreamLanguage,
	syntaxHighlighting,
} from "https://esm.sh/@codemirror/language";
import { keymap } from "https://esm.sh/@codemirror/view";
import { tags } from "https://esm.sh/@lezer/highlight";
import { basicSetup, EditorView } from "https://esm.sh/codemirror";

const isMobile = /Mobi|Android|iPhone|iPad|iPod/i.test(navigator.userAgent);
if (isMobile) {
	document.getElementById("run-button").innerText = "Run";
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

const editor = new EditorView({
	doc: `fn main() {
  println("Hello World!")
}`,
	extensions: [
		basicSetup,
		jule,
		syntaxHighlighting(style),
		keymap.of([indentWithTab]),
	],
	parent: document.getElementById("editor"),
});

const runButton = document.getElementById("run-button");
runButton.onclick = () => {
	const outputElement = document.getElementById("output");
	outputElement.textContent = "Compiling...";

	const inputCode = editor.state.doc.toString();

	const start = performance.now();
	fetch("/playground/compile", {
		method: "POST",
		body: inputCode,
		headers: { "Content-Type": "text/plain" },
	})
		.then((res) => res.text())
		.then((output) => {
			const end = performance.now();
			const duration = (end - start) / 1000;
			console.log("Compilation took", duration, "s");
			outputElement.textContent = output;
		})
		.catch((err) => {
			outputElement.textContent = `Error: ${err}`;
		});
};

document.addEventListener(
	"keydown",
	(e) => {
		if (e.ctrlKey && e.key === "Enter") {
			// This and the capture parameter below prevents from the newline addition
			// inside the code editor.
			e.preventDefault();
			runButton.click();
		}
	},
	{ capture: true },
);
