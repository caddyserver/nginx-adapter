// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nginxconf

import (
	"bufio"
	"bytes"
	"io"
	"unicode"
)

func tokenize(body []byte) []token {
	lex := newLexer("nginx.conf", bytes.NewReader(body))

	var tokens []token
	for lex.nextToken() {
		tokens = append(tokens, lex.token)
	}

	// TODO: Debug mode
	// for _, tkn := range tokens {
	// 	log.Printf("Line % 3d: %s\n", tkn.line, tkn.text)
	// }

	return tokens
}

func newLexer(filename string, input io.Reader) *nginxLexer {
	return &nginxLexer{
		reader: bufio.NewReader(input),
		file:   filename,
		line:   1,
	}
}

// this nginx lexer is freakishly similar to Caddyfile
// lexer (in fact, I copied the Caddyfile lexer and only
// had to change a few lines)

type nginxLexer struct {
	reader *bufio.Reader
	token  token
	file   string
	line   int
}

// needs escaping: <space> " ' { } ; $ \
// https://serverfault.com/questions/793550/when-do-you-have-to-use-quotes-in-the-configuration/793553

func (l *nginxLexer) nextToken() bool {
	var val []rune
	var comment, escaped bool
	var quoted rune

	makeToken := func() bool {
		l.token.text = string(val)
		return true
	}

	for {
		ch, _, err := l.reader.ReadRune()
		if err != nil {
			if len(val) > 0 {
				return makeToken()
			}
			if err == io.EOF {
				return false
			}
			panic(err)
		}

		if quoted > 0 {
			if !escaped {
				if ch == '\\' {
					escaped = true
					continue
				} else if ch == quoted {
					return makeToken()
				}
			}
			if ch == '\n' {
				l.line++
			}
			if escaped {
				// only escape quotes
				if ch != quoted {
					val = append(val, '\\')
				}
			}
			val = append(val, ch)
			escaped = false
			continue
		}

		if unicode.IsSpace(ch) {
			if ch == '\r' {
				continue
			}
			if ch == '\n' {
				l.line++
				comment = false
			}
			if len(val) > 0 {
				return makeToken()
			}
			continue
		}

		if ch == ';' {
			if len(val) == 0 {
				val = append(val, ch)
				return makeToken()
			}
			l.reader.UnreadRune()
			return makeToken()
		}

		if ch == '#' {
			comment = true
		}

		if comment {
			continue
		}

		if len(val) == 0 {
			l.token = token{file: l.file, line: l.line}
			if ch == '"' || ch == '\'' {
				quoted = ch
				continue
			}
		}

		val = append(val, ch)
	}
}

type token struct {
	file string
	line int
	text string
}
