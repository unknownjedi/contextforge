"use client";

import React, { useState } from "react";
import { Copy, Check, Terminal } from "lucide-react";

interface MarkdownRendererProps {
  content: string;
  isStreaming?: boolean;
}

interface CodeBlockProps {
  language: string;
  code: string;
}

function CodeBlock({ language, code }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const lines = code.split("\n");

  return (
    <div className="my-3 rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden text-xs font-mono shadow-sm">
      <div className="flex items-center justify-between px-3 py-1.5 bg-zinc-900/80 border-b border-zinc-850 text-zinc-400 select-none">
        <span className="flex items-center gap-1.5 font-sans font-semibold uppercase tracking-wider text-[10px] text-zinc-300">
          <Terminal className="w-3 h-3 text-blue-400" />
          {language || "code"}
        </span>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1 px-2 py-0.5 rounded hover:bg-zinc-800 text-zinc-400 hover:text-zinc-200 transition-colors"
          title="Copy code"
        >
          {copied ? (
            <>
              <Check className="w-3 h-3 text-emerald-400" />
              <span className="text-[10px] text-emerald-400">Copied!</span>
            </>
          ) : (
            <>
              <Copy className="w-3 h-3" />
              <span className="text-[10px]">Copy</span>
            </>
          )}
        </button>
      </div>
      <div className="overflow-x-auto p-3">
        <table className="w-full border-collapse">
          <tbody>
            {lines.map((line, idx) => (
              <tr key={idx} className="hover:bg-zinc-900/50 leading-5">
                <td className="select-none pr-3 text-right text-zinc-600 font-mono text-[11px] w-8">
                  {idx + 1}
                </td>
                <td className="whitespace-pre text-zinc-200 font-mono text-xs">
                  {line || " "}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/**
 * Parses inline formatting: **bold**, *italic*, `code`, and [link](url)
 */
function renderFormattedText(text: string): React.ReactNode[] {
  const parts: React.ReactNode[] = [];
  // Regex to match `code`, **bold**, *italic*, or [link](url)
  const tokenRegex = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*|\[[^\]]+\]\([^)]+\))/g;

  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = tokenRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.substring(lastIndex, match.index));
    }

    const token = match[0];
    if (token.startsWith("`") && token.endsWith("`")) {
      parts.push(
        <code
          key={`code-${match.index}`}
          className="rounded bg-zinc-800/80 px-1.5 py-0.5 font-mono text-[11px] text-blue-300 border border-zinc-700/60"
        >
          {token.slice(1, -1)}
        </code>
      );
    } else if (token.startsWith("**") && token.endsWith("**")) {
      parts.push(
        <strong key={`bold-${match.index}`} className="font-semibold text-zinc-100">
          {token.slice(2, -2)}
        </strong>
      );
    } else if (token.startsWith("*") && token.endsWith("*")) {
      parts.push(
        <em key={`italic-${match.index}`} className="italic text-zinc-300">
          {token.slice(1, -1)}
        </em>
      );
    } else if (token.startsWith("[") && token.includes("](")) {
      const linkMatch = token.match(/\[([^\]]+)\]\(([^)]+)\)/);
      if (linkMatch) {
        parts.push(
          <a
            key={`link-${match.index}`}
            href={linkMatch[2]}
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-400 hover:text-blue-300 underline underline-offset-2 transition-colors"
          >
            {linkMatch[1]}
          </a>
        );
      } else {
        parts.push(token);
      }
    } else {
      parts.push(token);
    }

    lastIndex = match.index + token.length;
  }

  if (lastIndex < text.length) {
    parts.push(text.substring(lastIndex));
  }

  return parts;
}

export function MarkdownRenderer({ content, isStreaming }: MarkdownRendererProps) {
  if (!content && !isStreaming) return null;

  // Split into blocks: code blocks and standard text
  const blocks: React.ReactNode[] = [];
  const lines = (content || "").split("\n");

  let inCodeBlock = false;
  let codeLang = "";
  let codeLines: string[] = [];
  let paragraphLines: string[] = [];

  const flushParagraph = (keyPrefix: string) => {
    if (paragraphLines.length === 0) return;

    // Process line by line for headers, lists, blockquotes
    const elements: React.ReactNode[] = [];
    let listItems: React.ReactNode[] = [];
    let isOrderedList = false;

    const flushList = () => {
      if (listItems.length === 0) return;
      if (isOrderedList) {
        elements.push(
          <ol
            key={`ol-${elements.length}`}
            className="my-2 list-decimal list-inside space-y-1 text-zinc-300"
          >
            {listItems}
          </ol>
        );
      } else {
        elements.push(
          <ul
            key={`ul-${elements.length}`}
            className="my-2 list-disc list-inside space-y-1 text-zinc-300"
          >
            {listItems}
          </ul>
        );
      }
      listItems = [];
    };

    for (let i = 0; i < paragraphLines.length; i++) {
      const line = paragraphLines[i];
      const trimmed = line.trim();

      if (!trimmed) {
        flushList();
        continue;
      }

      // Headers
      if (trimmed.startsWith("### ")) {
        flushList();
        elements.push(
          <h4
            key={`h4-${i}`}
            className="text-sm font-bold text-zinc-100 mt-3 mb-1"
          >
            {renderFormattedText(trimmed.slice(4))}
          </h4>
        );
      } else if (trimmed.startsWith("## ")) {
        flushList();
        elements.push(
          <h3
            key={`h3-${i}`}
            className="text-base font-bold text-zinc-100 mt-3.5 mb-1.5"
          >
            {renderFormattedText(trimmed.slice(3))}
          </h3>
        );
      } else if (trimmed.startsWith("# ")) {
        flushList();
        elements.push(
          <h2
            key={`h2-${i}`}
            className="text-lg font-bold text-zinc-100 mt-4 mb-2"
          >
            {renderFormattedText(trimmed.slice(2))}
          </h2>
        );
      } else if (trimmed.startsWith("> ")) {
        // Blockquote
        flushList();
        elements.push(
          <blockquote
            key={`quote-${i}`}
            className="border-l-2 border-blue-500/70 bg-blue-950/20 px-3 py-1.5 my-2 text-zinc-300 text-xs italic rounded-r"
          >
            {renderFormattedText(trimmed.slice(2))}
          </blockquote>
        );
      } else if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
        // Unordered list item
        if (isOrderedList && listItems.length > 0) flushList();
        isOrderedList = false;
        listItems.push(
          <li key={`li-${i}`} className="text-xs text-zinc-200">
            {renderFormattedText(trimmed.slice(2))}
          </li>
        );
      } else if (/^\d+\.\s/.test(trimmed)) {
        // Ordered list item
        const match = trimmed.match(/^\d+\.\s(.*)/);
        if (!isOrderedList && listItems.length > 0) flushList();
        isOrderedList = true;
        listItems.push(
          <li key={`li-num-${i}`} className="text-xs text-zinc-200">
            {renderFormattedText(match ? match[1] : trimmed)}
          </li>
        );
      } else {
        flushList();
        elements.push(
          <p key={`p-${i}`} className="my-1.5 leading-relaxed text-zinc-200 text-xs sm:text-sm">
            {renderFormattedText(line)}
          </p>
        );
      }
    }

    flushList();

    blocks.push(
      <div key={`${keyPrefix}-para-${blocks.length}`} className="space-y-1">
        {elements}
      </div>
    );
    paragraphLines = [];
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith("```")) {
      if (!inCodeBlock) {
        // Start code block
        flushParagraph(`block-${i}`);
        inCodeBlock = true;
        codeLang = line.slice(3).trim();
        codeLines = [];
      } else {
        // End code block
        inCodeBlock = false;
        blocks.push(
          <CodeBlock
            key={`code-block-${i}`}
            language={codeLang}
            code={codeLines.join("\n")}
          />
        );
        codeLines = [];
        codeLang = "";
      }
    } else if (inCodeBlock) {
      codeLines.push(line);
    } else {
      paragraphLines.push(line);
    }
  }

  // If streaming and code block is still open, render what we have
  if (inCodeBlock && codeLines.length > 0) {
    blocks.push(
      <CodeBlock
        key="open-code-block"
        language={codeLang}
        code={codeLines.join("\n")}
      />
    );
  } else {
    flushParagraph("final");
  }

  return (
    <div className="markdown-content text-zinc-200 leading-relaxed font-sans">
      {blocks}
      {isStreaming && (
        <span className="inline-block w-2 h-4 ml-1 bg-blue-500 animate-pulse align-middle" />
      )}
    </div>
  );
}
