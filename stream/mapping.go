package stream

import (
	"github.com/tdewolff/parse/v2/css"
	"github.com/tdewolff/parse/v2/html"
	"github.com/tdewolff/parse/v2/js"
	"github.com/tdewolff/parse/v2/xml"
)

// mapToken converts a native lexer event into the format-independent Token.
// All byte slices are copied so the token stays valid after the driver swaps
// its internal buffer.
func mapToken(format Format, native uint32, data []byte, lex formatLexer) Token {
	t := Token{
		Format:     format,
		Native:     native,
		NativeName: nativeName(format, native),
		Data:       copyBytes(data),
		Text:       copyBytes(lex.text()),
		AttrKey:    copyBytes(lex.attrKey()),
		AttrVal:    copyBytes(lex.attrVal()),
	}
	switch format {
	case HTML:
		t.Kind = kindForHTML(native)
	case CSS:
		t.Kind = kindForCSS(native)
	case JS:
		t.Kind = kindForJS(native)
	case XML:
		t.Kind = kindForXML(native)
	}
	return t
}

// nativeName returns the native TokenType's String() representation.
func nativeName(format Format, native uint32) string {
	switch format {
	case HTML:
		return html.TokenType(native).String()
	case CSS:
		return css.TokenType(native).String()
	case JS:
		return js.TokenType(native).String()
	case XML:
		return xml.TokenType(native).String()
	}
	return ""
}

// kindForHTML maps an html.TokenType onto a unified Kind.
func kindForHTML(native uint32) Kind {
	switch html.TokenType(native) {
	case html.ErrorToken:
		return KindError
	case html.CommentToken:
		return KindComment
	case html.DoctypeToken:
		return KindDoctype
	case html.StartTagToken:
		return KindTagStart
	case html.StartTagCloseToken:
		return KindTagClose
	case html.StartTagVoidToken:
		return KindTagSelfClose
	case html.EndTagToken:
		return KindTagEnd
	case html.AttributeToken:
		return KindAttribute
	case html.TextToken:
		return KindText
	default: // SVG, Math, XML foreign content and template tokens
		return KindOther
	}
}

// kindForXML maps an xml.TokenType onto a unified Kind.
func kindForXML(native uint32) Kind {
	switch xml.TokenType(native) {
	case xml.ErrorToken:
		return KindError
	case xml.CommentToken:
		return KindComment
	case xml.DOCTYPEToken:
		return KindDoctype
	case xml.CDATAToken:
		return KindCDATA
	case xml.StartTagToken:
		return KindTagStart
	case xml.StartTagPIToken:
		return KindPI
	case xml.StartTagCloseToken:
		return KindTagClose
	case xml.StartTagCloseVoidToken, xml.StartTagClosePIToken:
		return KindTagSelfClose
	case xml.EndTagToken:
		return KindTagEnd
	case xml.AttributeToken:
		return KindAttribute
	case xml.TextToken:
		return KindText
	default:
		return KindOther
	}
}

// kindForCSS maps a css.TokenType onto a unified Kind.
func kindForCSS(native uint32) Kind {
	tt := css.TokenType(native)
	switch tt {
	case css.ErrorToken:
		return KindError
	case css.CommentToken:
		return KindComment
	case css.WhitespaceToken:
		return KindWhitespace
	case css.StringToken, css.BadStringToken:
		return KindString
	case css.NumberToken, css.PercentageToken, css.DimensionToken, css.UnicodeRangeToken:
		return KindNumber
	case css.IdentToken, css.FunctionToken, css.AtKeywordToken, css.HashToken,
		css.URLToken, css.BadURLToken, css.CustomPropertyNameToken, css.CustomPropertyValueToken:
		return KindIdentifier
	case css.DelimToken:
		return KindPunctuator
	default:
		return KindOther
	}
}

// kindForJS maps a js.TokenType onto a unified Kind. The native token type
// carries category bitmasks, so the native Is* predicates decide the kind.
func kindForJS(native uint32) Kind {
	tt := js.TokenType(native)
	switch tt {
	case js.ErrorToken:
		return KindError
	case js.WhitespaceToken:
		return KindWhitespace
	case js.LineTerminatorToken, js.CommentLineTerminatorToken:
		return KindLineTerminator
	case js.CommentToken:
		return KindComment
	case js.StringToken:
		return KindString
	case js.RegExpToken:
		return KindRegexp
	case js.TemplateToken, js.TemplateStartToken, js.TemplateMiddleToken, js.TemplateEndToken:
		return KindTemplate
	}
	switch {
	case js.IsNumeric(tt):
		return KindNumber
	case js.IsOperator(tt):
		return KindOperator
	case js.IsPunctuator(tt):
		return KindPunctuator
	case js.IsReservedWord(tt):
		return KindKeyword
	case js.IsIdentifierName(tt):
		return KindIdentifier
	}
	return KindOther
}
