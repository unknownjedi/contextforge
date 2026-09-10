package docparser

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxFileSize is the maximum allowed document file size (10MB).
const MaxFileSize int64 = 10 * 1024 * 1024

var (
	// ErrFileTooLarge is returned when file size exceeds the 10MB limit.
	ErrFileTooLarge = errors.New("file size exceeds maximum allowed limit of 10MB")
	// ErrEmptyFile is returned when file has 0 bytes.
	ErrEmptyFile = errors.New("file is empty")
	// ErrInvalidUTF8 is returned when text-based file contains invalid UTF-8 sequences.
	ErrInvalidUTF8 = errors.New("file contains invalid UTF-8 text")
	// ErrUnsupportedFormat is returned when the file extension is not supported.
	ErrUnsupportedFormat = errors.New("unsupported file format")
	// ErrInvalidPDF is returned when PDF file is corrupt or missing standard markers.
	ErrInvalidPDF = errors.New("invalid or corrupt PDF file")
	// ErrNoExtractableText is returned when a document contains no extractable text.
	ErrNoExtractableText = errors.New("no extractable text found in file")
)

// Parser defines the pure Go contract for parsing documents into clean text/markdown.
type Parser interface {
	Parse(ctx context.Context, filename string, r io.Reader) (string, error)
}

// DefaultParser implements Parser for md, txt, json, csv, and pdf files.
type DefaultParser struct {
	maxFileSize int64
}

// NewParser creates a new DefaultParser enforcing the standard 10MB limit.
func NewParser() Parser {
	return &DefaultParser{
		maxFileSize: MaxFileSize,
	}
}

// Parse is a convenience package-level function delegating to DefaultParser.
func Parse(ctx context.Context, filename string, r io.Reader) (string, error) {
	return NewParser().Parse(ctx, filename, r)
}

// Parse processes the reader according to the file extension, enforcing size and encoding constraints.
func (p *DefaultParser) Parse(ctx context.Context, filename string, r io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown", ".txt", ".text", ".json", ".csv", ".pdf":
		// Supported
	default:
		return "", fmt.Errorf("%w: %q (supported: .md, .txt, .json, .csv, .pdf)", ErrUnsupportedFormat, ext)
	}

	// 1. Enforce 10MB maximum file size limit using LimitReader
	limitReader := io.LimitReader(r, p.maxFileSize+1)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return "", fmt.Errorf("reading document stream: %w", err)
	}

	if int64(len(data)) > p.maxFileSize {
		return "", ErrFileTooLarge
	}

	if len(data) == 0 {
		return "", ErrEmptyFile
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	switch ext {
	case ".md", ".markdown", ".txt", ".text":
		if !utf8.Valid(data) {
			return "", ErrInvalidUTF8
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			return "", ErrNoExtractableText
		}
		return text, nil

	case ".json":
		if !utf8.Valid(data) {
			return "", ErrInvalidUTF8
		}
		return parseJSON(data)

	case ".csv":
		if !utf8.Valid(data) {
			return "", ErrInvalidUTF8
		}
		return parseCSV(data)

	case ".pdf":
		return parsePDF(ctx, data)

	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedFormat, ext)
	}
}

// parseJSON validates and pretty-prints JSON documents.
func parseJSON(data []byte) (string, error) {
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("invalid json format: %w", err)
	}

	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return "", fmt.Errorf("formatting json: %w", err)
	}

	text := strings.TrimSpace(string(pretty))
	if text == "" {
		return "", ErrNoExtractableText
	}
	return text, nil
}

// parseCSV converts CSV tabular data into a clean Markdown table.
func parseCSV(data []byte) (string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1 // Allow variable row lengths across records
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("invalid csv format: %w", err)
	}

	if len(records) == 0 {
		return "", ErrEmptyFile
	}

	// Calculate maximum columns across all rows
	maxCols := 0
	for _, row := range records {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	if maxCols == 0 {
		return "", ErrEmptyFile
	}

	var sb strings.Builder

	// Header row
	sb.WriteString("|")
	for c := 0; c < maxCols; c++ {
		val := ""
		if c < len(records[0]) {
			val = sanitizeMarkdownCell(records[0][c])
		}
		sb.WriteString(" " + val + " |")
	}
	sb.WriteString("\n|")

	// Separator row
	for c := 0; c < maxCols; c++ {
		sb.WriteString(" --- |")
	}
	sb.WriteString("\n")

	// Data rows
	for r := 1; r < len(records); r++ {
		sb.WriteString("|")
		for c := 0; c < maxCols; c++ {
			val := ""
			if c < len(records[r]) {
				val = sanitizeMarkdownCell(records[r][c])
			}
			sb.WriteString(" " + val + " |")
		}
		sb.WriteString("\n")
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "", ErrNoExtractableText
	}
	return result, nil
}

// sanitizeMarkdownCell escapes pipes and collapses internal newlines for markdown tables.
func sanitizeMarkdownCell(val string) string {
	val = strings.ReplaceAll(val, "|", `\|`)
	val = strings.ReplaceAll(val, "\r\n", " ")
	val = strings.ReplaceAll(val, "\n", " ")
	val = strings.ReplaceAll(val, "\r", " ")
	return strings.TrimSpace(val)
}

// parsePDF extracts text from PDF content streams using pure Go.
func parsePDF(ctx context.Context, data []byte) (string, error) {
	if !bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-")) {
		return "", ErrInvalidPDF
	}

	var extractedLines []string

	// Find and parse all stream objects
	streamMatches := findPDFStreams(data)
	for _, s := range streamMatches {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		streamBytes := s.data
		// Decompress if FlateDecode
		if s.isFlate {
			zr, err := zlib.NewReader(bytes.NewReader(streamBytes))
			if err == nil {
				decompressed, readErr := io.ReadAll(zr)
				_ = zr.Close()
				if readErr == nil {
					streamBytes = decompressed
				}
			}
		}

		lines := extractTextFromPDFContent(streamBytes)
		extractedLines = append(extractedLines, lines...)
	}

	// Fallback: if no text from streams, scan whole file for standard BT ... ET or (text) Tj tokens
	if len(extractedLines) == 0 {
		extractedLines = extractTextFromPDFContent(data)
	}

	// Sanitize, clean and join lines
	var cleaned []string
	for _, l := range extractedLines {
		t := cleanExtractedText(l)
		if t != "" {
			cleaned = append(cleaned, t)
		}
	}

	if len(cleaned) == 0 {
		return "", ErrNoExtractableText
	}

	return strings.Join(cleaned, "\n"), nil
}

type pdfStream struct {
	data    []byte
	isFlate bool
}

// findPDFStreams locates all stream ... endstream sections in a PDF document.
func findPDFStreams(data []byte) []pdfStream {
	var streams []pdfStream

	streamMarker := []byte("stream")
	endStreamMarker := []byte("endstream")

	offset := 0
	for offset < len(data) {
		idx := bytes.Index(data[offset:], streamMarker)
		if idx == -1 {
			break
		}
		streamStart := offset + idx

		// Check dictionary preceding stream
		dictStart := max(0, streamStart-1024)
		preceding := data[dictStart:streamStart]
		isFlate := bytes.Contains(preceding, []byte("/FlateDecode"))

		// Skip stream keyword and trailing newline (\r\n or \n)
		contentStart := streamStart + len(streamMarker)
		if contentStart < len(data) && data[contentStart] == '\r' {
			contentStart++
		}
		if contentStart < len(data) && data[contentStart] == '\n' {
			contentStart++
		}

		endIdx := bytes.Index(data[contentStart:], endStreamMarker)
		if endIdx == -1 {
			break
		}

		contentEnd := contentStart + endIdx
		// Trim optional trailing \r\n before endstream
		if contentEnd > contentStart && data[contentEnd-1] == '\n' {
			contentEnd--
		}
		if contentEnd > contentStart && data[contentEnd-1] == '\r' {
			contentEnd--
		}

		if contentEnd >= contentStart {
			streams = append(streams, pdfStream{
				data:    data[contentStart:contentEnd],
				isFlate: isFlate,
			})
		}

		offset = contentStart + endIdx + len(endStreamMarker)
	}

	return streams
}

// extractTextFromPDFContent parses PDF text drawing operators: Tj, TJ, ', " and line repositioning operators.
func extractTextFromPDFContent(content []byte) []string {
	var lines []string
	var currentLine strings.Builder

	i := 0
	n := len(content)

	for i < n {
		b := content[i]

		// Check for string literal (...)
		if b == '(' {
			str, nextIdx := parsePDFString(content, i)
			i = nextIdx

			// Skip spaces to check following operator
			op, opNext := readNextWord(content, i)
			switch op {
			case "Tj":
				currentLine.WriteString(str)
				i = opNext
			case "'":
				if currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
				currentLine.WriteString(str)
				i = opNext
			case "\"":
				if currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
				currentLine.WriteString(str)
				i = opNext
			default:
				// Append text if it looks like a text block
				if len(str) > 0 {
					currentLine.WriteString(str)
				}
			}
			continue
		}

		// Check for hex string literal <...>
		if b == '<' && i+1 < n && content[i+1] != '<' {
			hexStr, nextIdx := parsePDFHexString(content, i)
			i = nextIdx

			op, opNext := readNextWord(content, i)
			if op == "Tj" || op == "'" || op == "\"" {
				if op != "Tj" && currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
				currentLine.WriteString(hexStr)
				i = opNext
			} else if len(hexStr) > 0 {
				currentLine.WriteString(hexStr)
			}
			continue
		}

		// Check for TJ array [...] TJ
		if b == '[' {
			tjArray, nextIdx := parsePDFTJArray(content, i)
			i = nextIdx

			op, opNext := readNextWord(content, i)
			if op == "TJ" {
				currentLine.WriteString(tjArray)
				i = opNext
			} else if len(tjArray) > 0 {
				currentLine.WriteString(tjArray)
			}
			continue
		}

		// Check operators for newlines or block breaks
		if b == 'T' || b == 'E' {
			word, wordNext := readNextWord(content, i)
			if word == "ET" || word == "T*" || word == "Td" || word == "TD" {
				if currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
				i = wordNext
				continue
			}
		}

		i++
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// parsePDFString extracts and decodes a balanced, escaped PDF string: (content)
func parsePDFString(data []byte, start int) (string, int) {
	if start >= len(data) || data[start] != '(' {
		return "", start + 1
	}

	var sb strings.Builder
	depth := 0
	i := start

	for i < len(data) {
		ch := data[i]
		if ch == '\\' && i+1 < len(data) {
			i++
			next := data[i]
			switch next {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case '(':
				sb.WriteByte('(')
			case ')':
				sb.WriteByte(')')
			case '\\':
				sb.WriteByte('\\')
			default:
				// Handle octal escape \ooo (up to 3 octal digits)
				if next >= '0' && next <= '7' {
					octalVal := int(next - '0')
					count := 1
					for count < 3 && i+1 < len(data) && data[i+1] >= '0' && data[i+1] <= '7' {
						i++
						octalVal = octalVal*8 + int(data[i]-'0')
						count++
					}
					sb.WriteByte(byte(octalVal))
				} else {
					sb.WriteByte(next)
				}
			}
			i++
			continue
		}

		if ch == '(' {
			depth++
			if depth > 1 {
				sb.WriteByte(ch)
			}
		} else if ch == ')' {
			depth--
			if depth == 0 {
				i++
				break
			}
			sb.WriteByte(ch)
		} else {
			sb.WriteByte(ch)
		}
		i++
	}

	return sb.String(), i
}

// parsePDFHexString parses a hex string literal: <48656c6c6f>
func parsePDFHexString(data []byte, start int) (string, int) {
	if start >= len(data) || data[start] != '<' {
		return "", start + 1
	}

	end := bytes.IndexByte(data[start:], '>')
	if end == -1 {
		return "", len(data)
	}

	hexSlice := data[start+1 : start+end]
	var cleanedHex []byte
	for _, b := range hexSlice {
		if !unicode.IsSpace(rune(b)) {
			cleanedHex = append(cleanedHex, b)
		}
	}

	// If odd number of hex digits, append '0' per PDF specification
	if len(cleanedHex)%2 != 0 {
		cleanedHex = append(cleanedHex, '0')
	}

	decoded, err := hex.DecodeString(string(cleanedHex))
	if err != nil {
		return "", start + end + 1
	}

	return string(decoded), start + end + 1
}

var numberRegex = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// parsePDFTJArray extracts text from a TJ array: [(Text) -200 (More Text)]
func parsePDFTJArray(data []byte, start int) (string, int) {
	if start >= len(data) || data[start] != '[' {
		return "", start + 1
	}

	var sb strings.Builder
	i := start + 1
	n := len(data)

	for i < n {
		// Skip whitespace
		for i < n && unicode.IsSpace(rune(data[i])) {
			i++
		}
		if i >= n || data[i] == ']' {
			if i < n && data[i] == ']' {
				i++
			}
			break
		}

		if data[i] == '(' {
			str, next := parsePDFString(data, i)
			sb.WriteString(str)
			i = next
			continue
		}

		if data[i] == '<' && i+1 < n && data[i+1] != '<' {
			hexStr, next := parsePDFHexString(data, i)
			sb.WriteString(hexStr)
			i = next
			continue
		}

		// Check for numeric kerning offset (e.g. -250 indicates word spacing)
		numStart := i
		for i < n && (data[i] == '-' || data[i] == '+' || (data[i] >= '0' && data[i] <= '9') || data[i] == '.') {
			i++
		}

		if i > numStart {
			numStr := string(data[numStart:i])
			if val, err := strconv.ParseFloat(numStr, 64); err == nil {
				// Significant negative kerning indicates a space between words in PDF
				if val < -100 {
					sb.WriteByte(' ')
				}
			}
			continue
		}

		i++
	}

	return sb.String(), i
}

// readNextWord extracts the next non-whitespace word token.
func readNextWord(data []byte, start int) (word string, nextIdx int) {
	i := start
	n := len(data)

	// Skip spaces
	for i < n && unicode.IsSpace(rune(data[i])) {
		i++
	}

	wordStart := i
	for i < n && !unicode.IsSpace(rune(data[i])) && data[i] != '(' && data[i] != '[' && data[i] != '<' {
		i++
	}

	if i > wordStart {
		return string(data[wordStart:i]), i
	}
	return "", i
}

// cleanExtractedText normalizes UTF-8 strings and strips non-printable control characters except \n, \t.
func cleanExtractedText(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		if unicode.IsPrint(r) || r == '\n' || r == '\t' {
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}
