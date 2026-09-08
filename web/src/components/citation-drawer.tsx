"use client";

import React, { useState, useEffect } from "react";
import type { Citation } from "@/types/api";
import { X, FileCode, Copy, Check, ExternalLink, Percent, Hash } from "lucide-react";

interface CitationDrawerProps {
  citation: Citation | null;
  onClose: () => void;
  onOpenDocument?: (filePath: string) => void;
}

export function CitationDrawer({
  citation,
  onClose,
  onOpenDocument,
}: CitationDrawerProps) {
  const [copiedSnippet, setCopiedSnippet] = useState(false);
  const [copiedPath, setCopiedPath] = useState(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  if (!citation) return null;

  const handleCopySnippet = () => {
    navigator.clipboard.writeText(citation.snippet);
    setCopiedSnippet(true);
    setTimeout(() => setCopiedSnippet(false), 2000);
  };

  const handleCopyPath = () => {
    navigator.clipboard.writeText(citation.file_path);
    setCopiedPath(true);
    setTimeout(() => setCopiedPath(false), 2000);
  };

  const lines = (citation.snippet || "").split("\n");
  const similarityPct = Math.round(citation.similarity * 100);

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="absolute inset-0" onClick={onClose} />

      <div className="fixed inset-y-0 right-0 max-w-full flex pl-10">
        <div className="w-screen max-w-2xl bg-zinc-950 border-l border-zinc-800 shadow-2xl flex flex-col z-10 animate-in slide-in-from-right duration-200">
          {/* Drawer Header */}
          <div className="p-5 border-b border-zinc-800 bg-zinc-900/60">
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-center gap-3 min-w-0">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-blue-950/80 border border-blue-800/60 text-blue-400">
                  <FileCode className="h-5 w-5" />
                </div>
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <h2 className="text-sm font-semibold text-zinc-100 font-mono truncate">
                      {citation.file_path}
                    </h2>
                    <button
                      onClick={handleCopyPath}
                      className="text-zinc-500 hover:text-zinc-300 transition-colors p-1"
                      title="Copy file path"
                    >
                      {copiedPath ? (
                        <Check className="w-3.5 h-3.5 text-emerald-400" />
                      ) : (
                        <Copy className="w-3.5 h-3.5" />
                      )}
                    </button>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 mt-1.5 text-xs text-zinc-400">
                    <span className="inline-flex items-center gap-1 rounded bg-zinc-800/80 px-2 py-0.5 text-zinc-300 font-mono text-[11px]">
                      <Hash className="w-3 h-3 text-zinc-500" />
                      Lines {citation.start_line} – {citation.end_line}
                    </span>
                    <span className="inline-flex items-center gap-1 rounded bg-emerald-950/80 text-emerald-400 border border-emerald-800/50 px-2 py-0.5 font-mono text-[11px]">
                      <Percent className="w-3 h-3" />
                      {similarityPct}% Match
                    </span>
                  </div>
                </div>
              </div>

              <button
                onClick={onClose}
                className="rounded-lg p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100 transition-colors"
                title="Close drawer (Esc)"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
          </div>

          {/* Action Toolbar */}
          <div className="flex items-center justify-between px-5 py-2.5 bg-zinc-900/30 border-b border-zinc-850 text-xs">
            <span className="text-zinc-400 text-[11px]">
              Indexed semantic chunk ({lines.length} lines)
            </span>
            <div className="flex items-center gap-2">
              {onOpenDocument && (
                <button
                  onClick={() => onOpenDocument(citation.file_path)}
                  className="flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium text-zinc-300 hover:text-white bg-zinc-850 hover:bg-zinc-800 border border-zinc-750 transition-colors"
                >
                  <ExternalLink className="w-3.5 h-3.5" />
                  View in Documents
                </button>
              )}
              <button
                onClick={handleCopySnippet}
                className="flex items-center gap-1.5 px-3 py-1 rounded text-xs font-medium text-zinc-200 bg-blue-600/20 hover:bg-blue-600/30 border border-blue-500/40 hover:border-blue-500/60 transition-colors"
              >
                {copiedSnippet ? (
                  <>
                    <Check className="w-3.5 h-3.5 text-emerald-400" />
                    <span className="text-emerald-300">Copied!</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5 text-blue-400" />
                    <span>Copy Snippet</span>
                  </>
                )}
              </button>
            </div>
          </div>

          {/* Code Body */}
          <div className="flex-1 overflow-y-auto p-4 bg-zinc-950 font-mono text-xs">
            <div className="rounded-lg border border-zinc-850 bg-zinc-900/50 overflow-hidden shadow-inner">
              <table className="w-full border-collapse">
                <tbody>
                  {lines.map((line, idx) => {
                    const lineNum = citation.start_line + idx;
                    return (
                      <tr
                        key={idx}
                        className="hover:bg-blue-950/20 transition-colors leading-5 group"
                      >
                        <td className="select-none py-0.5 pl-3 pr-4 text-right text-zinc-600 font-mono text-[11px] w-12 border-r border-zinc-850 group-hover:text-zinc-400">
                          {lineNum}
                        </td>
                        <td className="py-0.5 pl-4 pr-3 whitespace-pre text-zinc-200 font-mono text-xs">
                          {line || " "}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* Drawer Footer */}
          <div className="p-4 border-t border-zinc-800 bg-zinc-900/60 flex items-center justify-between text-xs text-zinc-400">
            <span>
              Cosine similarity:{" "}
              <strong className="text-zinc-200 font-mono">
                {citation.similarity.toFixed(4)}
              </strong>
            </span>
            <button
              onClick={onClose}
              className="px-4 py-1.5 rounded-lg bg-zinc-850 hover:bg-zinc-800 text-zinc-200 border border-zinc-750 transition-colors text-xs font-medium"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
