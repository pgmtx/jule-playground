import {
	HighlightStyle,
	StreamLanguage,
	syntaxHighlighting,
} from "https://esm.sh/@codemirror/language@6.0.0";
import { tags } from "https://esm.sh/@lezer/highlight@1.0.0";
import { basicSetup, EditorView } from "https://esm.sh/codemirror@6.0.1";

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
	token(stream, _state) {
		if (stream.eatSpace()) return null;
		if (stream.match(/\/\/.*/)) return "lineComment";
		if (stream.match(/"([^"\\]|\\.)*"/)) return "string";
		if (stream.match(/'(\\.|[^'])'/)) return "string";
		if (stream.match(/'[^']*'/)) return "invalid";
		if (stream.match(/\b\d+(\.\d+)?i?\b/)) return "number";
		if (stream.match(typeRegex)) return "typeName";
		if (stream.match(keywordRegex)) return "keyword";
		if (stream.match(/(true|false|nil|iota)/)) return "literal";
		if (stream.match(/[a-zA-Z_]\w*(?=\()/)) return "name";
		if (stream.match(/[+\-*/%:=<>!&|]+/)) return "operator";
		if (stream.match(/[a-zA-Z_]\w*/)) return "variableName";

		stream.next();
		return null;
	},
});

const editor = new EditorView({
	doc: `fn main() {
  println("Hello World!")
}`,
	extensions: [basicSetup, jule, syntaxHighlighting(style)],
	parent: document.getElementById("editor"),
});

const runButton = document.getElementById("run-button");
runButton.onclick = () => {
	const inputCode = editor.state.doc.toString();
	fetch("/send", {
		method: "POST",
		body: inputCode,
		headers: { "Content-Type": "text/plain" },
	})
	.then(res => res.text())
	.then(output => console.log(output))
	.catch(err => console.log(err));
};

document.addEventListener("keydown", (e) => {
	if (e.ctrlKey && e.key === "Enter") {
		runButton.click();
	}
});
