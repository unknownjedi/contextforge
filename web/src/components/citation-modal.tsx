import React, { useState } from "react";
import type { Citation } from "@/types/api";
import { X, FileCode, Copy, Check } from "lucide-react";

interface CitationModalProps {
  citation: Citation | null;
  onClose: () => void;
}

export function CitationModal({ citation, onClose }: CitationModalProps) {
  const [copied, setCopied] = useState(false);

  if (!citation) return null;

  const handleCopy = () => {
    navigator.clipboard.writeText(citation.snippet);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const lines = citation.snippet.split("\n");

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="relative w-full max-w-3xl rounded-xl border border-zinc-800 bg-zinc-950 p-6 shadow-2xl flex flex-col max-h-[85vh]">
        {/* Header */}
        <div className="flex items-start justify-between border-b border-zinc-800 pb-4 mb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-950/60 border border-blue-800/50 text-blue-400">
              <FileCode className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-zinc-100 font-mono">
                {citation.file_path}
              </h3>
              <div className="flex items-center gap-3 mt-1 text-xs text-zinc-400">
                <span>
                  Lines: <strong className="text-zinc-300">{citation.start_line} – {citation.end_line}</strong>
                </span>
                <span>•</span>
                <span>
                  Relevance:{" "}
                  <strong className="text-emerald-400">
                    {(citation.similarity * 100).toFixed(1)}%
                  </strong>
                </span>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={handleCopy}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium text-zinc-300 bg-zinc-900 border border-zinc-700 hover:bg-zinc-800 transition-colors"
              title="Copy snippet"
            >
              {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              {copied ? "Copied" : "Copy"}
            </button>
            <button
              onClick={onClose}
              className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100 transition-colors"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* Code Snippet */}
        <div className="relative flex-1 overflow-auto rounded-lg border border-zinc-800/80 bg-zinc-900/60 font-mono text-xs">
          <div className="overflow-x-auto p-4">
            <table className="w-full border-collapse">
              <tbody>
                {lines.map((line, idx) => {
                  const lineNum = citation.start_line + idx;
                  return (
                    <tr key={idx} className="hover:bg-zinc-800/30">
                      <td className="select-none pr-4 text-right text-zinc-600 font-mono w-10">
                        {lineNum}
                      </td>
                      <td className="whitespace-pre text-zinc-200">
                        {line || " "}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>

        {/* Footer */}
        <div className="mt-4 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-zinc-300 bg-zinc-900 hover:bg-zinc-800 rounded-lg border border-zinc-800 transition-colors"
          >
            Close Preview
          </button>
        </div>
      </div>
    </div>
  );
}
