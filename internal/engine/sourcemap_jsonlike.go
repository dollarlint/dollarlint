package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type jsonLikeSourceScanner struct {
	raw        []byte
	lineIndex  []int
	sourceMap  SourceMap
	allowJSON5 bool
	offset     int
}

func buildJSONLikeSourceMap(raw []byte, allowJSON5 bool) (SourceMap, error) {
	scanner := &jsonLikeSourceScanner{
		raw:        raw,
		lineIndex:  newLineIndex(raw),
		sourceMap:  SourceMap{},
		allowJSON5: allowJSON5,
	}
	if err := scanner.parseValue("/"); err != nil {
		return nil, err
	}
	return scanner.sourceMap, nil
}

func (s *jsonLikeSourceScanner) parseValue(pointer string) error {
	s.skipSpaceAndComments()
	if s.offset >= len(s.raw) {
		return ioErrUnexpectedEOF()
	}
	setSourcePosition(s.sourceMap, pointer, positionAtOffset(s.lineIndex, s.offset))
	switch s.raw[s.offset] {
	case '{':
		return s.parseObject(pointer)
	case '[':
		return s.parseArray(pointer)
	case '"':
		s.parseQuotedString('"')
	case '\'':
		if !s.allowJSON5 {
			return fmt.Errorf("unexpected single-quoted string")
		}
		s.parseQuotedString('\'')
	default:
		s.parsePrimitive()
	}
	return nil
}

func (s *jsonLikeSourceScanner) parseObject(pointer string) error {
	s.offset++
	for {
		s.skipSpaceAndComments()
		if s.offset >= len(s.raw) {
			return ioErrUnexpectedEOF()
		}
		if s.raw[s.offset] == '}' {
			s.offset++
			return nil
		}
		key, err := s.parseObjectKey()
		if err != nil {
			return err
		}
		s.skipSpaceAndComments()
		if s.offset >= len(s.raw) || s.raw[s.offset] != ':' {
			return fmt.Errorf("expected object key separator")
		}
		s.offset++
		if err := s.parseValue(joinPointer(pointer, key)); err != nil {
			return err
		}
		s.skipSpaceAndComments()
		if s.offset < len(s.raw) && s.raw[s.offset] == ',' {
			s.offset++
			continue
		}
	}
}

func (s *jsonLikeSourceScanner) parseArray(pointer string) error {
	s.offset++
	index := 0
	for {
		s.skipSpaceAndComments()
		if s.offset >= len(s.raw) {
			return ioErrUnexpectedEOF()
		}
		if s.raw[s.offset] == ']' {
			s.offset++
			return nil
		}
		if err := s.parseValue(joinPointer(pointer, strconv.Itoa(index))); err != nil {
			return err
		}
		index++
		s.skipSpaceAndComments()
		if s.offset < len(s.raw) && s.raw[s.offset] == ',' {
			s.offset++
			continue
		}
	}
}

func (s *jsonLikeSourceScanner) parseObjectKey() (string, error) {
	switch s.raw[s.offset] {
	case '"':
		return s.parseQuotedString('"'), nil
	case '\'':
		if !s.allowJSON5 {
			return "", fmt.Errorf("unexpected single-quoted object key")
		}
		return s.parseQuotedString('\''), nil
	default:
		if !s.allowJSON5 {
			return "", fmt.Errorf("expected quoted object key")
		}
		return s.parseIdentifier(), nil
	}
}

func (s *jsonLikeSourceScanner) parseQuotedString(quote byte) string {
	start := s.offset
	s.offset++
	escaped := false
	for s.offset < len(s.raw) {
		ch := s.raw[s.offset]
		s.offset++
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == quote {
			var out string
			if err := json.Unmarshal(s.raw[start:s.offset], &out); err == nil {
				return out
			}
			return strings.Trim(string(s.raw[start+1:s.offset-1]), " \t\r\n")
		}
	}
	return strings.Trim(string(s.raw[start+1:]), " \t\r\n")
}

func (s *jsonLikeSourceScanner) parseIdentifier() string {
	start := s.offset
	for s.offset < len(s.raw) {
		ch := s.raw[s.offset]
		if ch == ':' || unicode.IsSpace(rune(ch)) {
			break
		}
		s.offset++
	}
	return strings.TrimSpace(string(s.raw[start:s.offset]))
}

func (s *jsonLikeSourceScanner) parsePrimitive() {
	for s.offset < len(s.raw) {
		ch := s.raw[s.offset]
		if ch == ',' || ch == ']' || ch == '}' || unicode.IsSpace(rune(ch)) {
			return
		}
		if ch == '/' && s.offset+1 < len(s.raw) && (s.raw[s.offset+1] == '/' || s.raw[s.offset+1] == '*') {
			return
		}
		s.offset++
	}
}

func (s *jsonLikeSourceScanner) skipSpaceAndComments() {
	for s.offset < len(s.raw) {
		if unicode.IsSpace(rune(s.raw[s.offset])) {
			s.offset++
			continue
		}
		if s.offset+1 >= len(s.raw) || s.raw[s.offset] != '/' {
			return
		}
		switch s.raw[s.offset+1] {
		case '/':
			s.offset += 2
			for s.offset < len(s.raw) && s.raw[s.offset] != '\n' {
				s.offset++
			}
		case '*':
			s.offset += 2
			for s.offset+1 < len(s.raw) && !(s.raw[s.offset] == '*' && s.raw[s.offset+1] == '/') {
				s.offset++
			}
			if s.offset+1 < len(s.raw) {
				s.offset += 2
			}
		default:
			return
		}
	}
}

func ioErrUnexpectedEOF() error {
	return fmt.Errorf("unexpected EOF")
}
