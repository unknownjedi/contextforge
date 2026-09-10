package docparser_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/docparser"
)

func createUncompressedPDF(streamContent string) []byte {
	return []byte(fmt.Sprintf("%%PDF-1.4\n1 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%%%EOF", len(streamContent), streamContent))
}

func createFlatePDF(streamContent string) []byte {
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write([]byte(streamContent))
	_ = zw.Close()

	compBytes := compressed.Bytes()
	header := fmt.Sprintf("%%PDF-1.4\n1 0 obj\n<< /Filter /FlateDecode /Length %d >>\nstream\n", len(compBytes))
	footer := "\nendstream\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF"

	var full bytes.Buffer
	full.WriteString(header)
	full.Write(compBytes)
	full.WriteString(footer)
	return full.Bytes()
}

func TestParser_Markdown(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	content := "# Project Overview\n\nThis is a **markdown** document for ContextForge.\n\n- Item 1\n- Item 2"
	parsed, err := parser.Parse(ctx, "README.md", strings.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, content, parsed)

	// Also support .markdown extension
	parsedExt, err := parser.Parse(ctx, "notes.markdown", strings.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, content, parsedExt)
}

func TestParser_PlainText(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	content := "Line 1: Plain text file.\nLine 2: ContextForge documentation."
	parsed, err := parser.Parse(ctx, "document.txt", strings.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, content, parsed)

	// Also support .text extension
	parsedText, err := parser.Parse(ctx, "log.text", strings.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, content, parsedText)
}

func TestParser_JSON(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	// Minified JSON
	rawJSON := `{"name":"ContextForge","version":1,"active":true,"tags":["rag","vector"]}`
	parsed, err := parser.Parse(ctx, "config.json", strings.NewReader(rawJSON))
	require.NoError(t, err)

	expected := `{
  "active": true,
  "name": "ContextForge",
  "tags": [
    "rag",
    "vector"
  ],
  "version": 1
}`
	assert.Equal(t, expected, parsed)

	// Invalid JSON
	_, err = parser.Parse(ctx, "bad.json", strings.NewReader(`{invalid json}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid json")
}

func TestParser_CSV(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	csvContent := "ID,Name,Role\n1,Alice,Engineer\n2,Bob|Lead,Designer\n3,Charlie,\"Product\nManager\""
	parsed, err := parser.Parse(ctx, "team.csv", strings.NewReader(csvContent))
	require.NoError(t, err)

	// Check Markdown table headers and data
	assert.Contains(t, parsed, "| ID | Name | Role |")
	assert.Contains(t, parsed, "| --- | --- | --- |")
	assert.Contains(t, parsed, "| 1 | Alice | Engineer |")
	assert.Contains(t, parsed, `| 2 | Bob\|Lead | Designer |`)
	assert.Contains(t, parsed, "| 3 | Charlie | Product Manager |")
}

func TestParser_PDF_Uncompressed(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	stream := "BT /F1 12 Tf 72 712 Td (Hello ContextForge PDF Text!) Tj ET"
	pdfData := createUncompressedPDF(stream)

	parsed, err := parser.Parse(ctx, "sample.pdf", bytes.NewReader(pdfData))
	require.NoError(t, err)
	assert.Contains(t, parsed, "Hello ContextForge PDF Text!")
}

func TestParser_PDF_FlateCompressed(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	stream := "BT /F1 12 Tf [(ContextForge) -250 (Document) -250 (Ingestion)] TJ ET"
	pdfData := createFlatePDF(stream)

	parsed, err := parser.Parse(ctx, "compressed.pdf", bytes.NewReader(pdfData))
	require.NoError(t, err)
	assert.Contains(t, parsed, "ContextForge Document Ingestion")
}

func TestParser_PDF_EscapedAndHexStrings(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	// Escaped parens, octal space (\040), and hex string <476f> ("Go")
	stream := "BT /F1 12 Tf (Hello\\(ContextForge\\)\\040World) Tj T* <476f204c616e67> Tj ET"
	pdfData := createUncompressedPDF(stream)

	parsed, err := parser.Parse(ctx, "escaped.pdf", bytes.NewReader(pdfData))
	require.NoError(t, err)
	assert.Contains(t, parsed, "Hello(ContextForge) World")
	assert.Contains(t, parsed, "Go Lang")
}

func TestParser_PDF_Invalid(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	// Not a PDF
	_, err := parser.Parse(ctx, "corrupt.pdf", strings.NewReader("Not a PDF file at all"))
	require.Error(t, err)
	assert.ErrorIs(t, err, docparser.ErrInvalidPDF)

	// PDF with no text
	emptyPDF := []byte("%PDF-1.4\n1 0 obj\n<< /Length 0 >>\nstream\n\nendstream\nendobj\n%%EOF")
	_, err = parser.Parse(ctx, "empty.pdf", bytes.NewReader(emptyPDF))
	require.Error(t, err)
	assert.ErrorIs(t, err, docparser.ErrNoExtractableText)
}

func TestParser_FileTooLarge(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	// 10MB + 1 byte
	oversized := bytes.Repeat([]byte("A"), int(docparser.MaxFileSize)+10)
	_, err := parser.Parse(ctx, "huge.txt", bytes.NewReader(oversized))
	require.Error(t, err)
	assert.ErrorIs(t, err, docparser.ErrFileTooLarge)
}

func TestParser_EmptyFile(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	_, err := parser.Parse(ctx, "empty.txt", bytes.NewReader([]byte{}))
	require.Error(t, err)
	assert.ErrorIs(t, err, docparser.ErrEmptyFile)
}

func TestParser_InvalidUTF8(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	// Invalid UTF-8 byte sequence
	invalidUTF8 := []byte{0xff, 0xfe, 0xfd}
	_, err := parser.Parse(ctx, "invalid.txt", bytes.NewReader(invalidUTF8))
	require.Error(t, err)
	assert.ErrorIs(t, err, docparser.ErrInvalidUTF8)
}

func TestParser_UnsupportedFormat(t *testing.T) {
	ctx := context.Background()
	parser := docparser.NewParser()

	_, err := parser.Parse(ctx, "script.py", strings.NewReader("print('hello')"))
	require.Error(t, err)
	assert.ErrorIs(t, err, docparser.ErrUnsupportedFormat)
}

func TestParser_PackageLevelParse(t *testing.T) {
	ctx := context.Background()

	parsed, err := docparser.Parse(ctx, "doc.txt", strings.NewReader("Package level convenience test"))
	require.NoError(t, err)
	assert.Equal(t, "Package level convenience test", parsed)
}
