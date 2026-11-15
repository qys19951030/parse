package stream

// mapToken converts a native lexer event (token type plus the lexer itself
// for Text/AttrKey/AttrVal) into the format-independent Token, leaving the
// position fields for the driver to fill.
func mapToken(format Format, native uint32, data []byte, lex formatLexer) Token {
	return Token{
		Format:     format,
		Kind:       KindOther,
		Native:     native,
		NativeName: "",
		Data:       copyBytes(data),
	}
}

// nativeName returns the native TokenType's String() representation for the
// given format.
func nativeName(format Format, native uint32) string {
	return ""
}

// kindForHTML maps an html.TokenType onto a unified Kind.
func kindForHTML(native uint32) Kind {
	return KindOther
}

// kindForCSS maps a css.TokenType onto a unified Kind.
func kindForCSS(native uint32) Kind {
	return KindOther
}

// kindForJS maps a js.TokenType onto a unified Kind.
func kindForJS(native uint32) Kind {
	return KindOther
}

// kindForXML maps an xml.TokenType onto a unified Kind.
func kindForXML(native uint32) Kind {
	return KindOther
}
