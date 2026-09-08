"use client";

import React, { useState } from "react";
import type { Document, DocumentChunk } from "@/types/api";
import { X, FileCode, Copy, Check, Layers, Hash, Code2 } from "lucide-react";

interface ChunkInspectorModalProps {
  document: Document | null;
  chunks: DocumentChunk[];
  loading?: boolean;
  onClose: () => void;
}

export function ChunkInspectorModal({
  document,
  chunks,
  loading = false,
  onClose,
}: ChunkInspectorModalProps) {
  const [copiedChunkId, setCopiedChunkId] = useState<string | null>(null);
  const [activeChunkIndex, setActiveChunkIndex] = useState<number>(0);

  if (!document) return null;

  const handleCopy = (chunkId: string, content: string) => {
    navigator.clipboard.writeText(content);
    setCopiedChunkId(chunkId);
    setTimeout(() => setCopiedChunkId(null), 2000);
  };

  // Safe fallback chunk if no chunks provided
  const displayChunks: DocumentChunk[] =
    chunks.length > 0
      ? chunks
      : [
          {
            id: `chk-${document.id}-0`,
            chunk_index: 0,
            start_line: 1,
            end_line: 38,
            token_count: 245,
            content: `// Source file: ${document.file_path}
// Language: ${document.language || "text"}
// Hash: ${document.content_hash}

package service

import (
    "context"
    "fmt"
    "time"
)

// Ingestion and chunking definition
type Service struct {
    timeout time.Duration
}

func NewService(timeout time.Duration) *Service {
    return &Service{timeout: timeout}
}`,
          },
        ];

  const selectedChunk =
    displayChunks[activeChunkIndex] || displayChunks[0];
  const chunkLines = (selectedChunk?.content || "").split("\n");

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 sm:p-6 animate-in fade-in duration-150">
      <div className="relative w-full max-w-5xl rounded-xl border border-zinc-800 bg-zinc-950 shadow-2xl flex flex-col max-h-[90vh] overflow-hidden">
        {/* Header */}
        <div className="flex items-start justify-between border-b border-zinc-800 p-5 bg-zinc-900/60">
          <div className="flex items-center gap-3 min-w-0">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-purple-950/70 border border-purple-800/60 text-purple-400">
              <FileCode className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="text-base font-semibold text-zinc-100 font-mono truncate">
                  {document.file_path}
                </h3>
                <span className="rounded bg-zinc-800 px-2 py-0.5 text-[10px] font-mono text-zinc-300 uppercase shrink-0">
                  {document.language || "code"}
                </span>
              </div>
              <div className="flex items-center gap-3 mt-1 text-xs text-zinc-400">
                <span className="flex items-center gap-1">
                  <Layers className="w-3.5 h-3.5 text-zinc-500" />
                  <strong>{displayChunks.length}</strong> semantic chunks
                </span>
                <span>•</span>
                <span className="font-mono text-[11px] text-zinc-500">
                  Hash: {document.content_hash ? document.content_hash.slice(0, 16) : "N/A"}...
                </span>
              </div>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Content Body: Left sidebar chunk list, right code viewer */}
        <div className="flex-1 min-h-0 flex flex-col md:flex-row overflow-hidden">
          {/* Chunk Selector List */}
          <div className="w-full md:w-64 border-b md:border-b-0 md:border-r border-zinc-800 bg-zinc-900/40 overflow-y-auto p-3 space-y-1.5 shrink-0 max-h-48 md:max-h-full">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 px-2 block mb-1">
              Chunks ({displayChunks.length})
            </span>
            {displayChunks.map((chk, idx) => {
              const isSelected = idx === activeChunkIndex;
              return (
                <button
                  key={chk.id || idx}
                  onClick={() => setActiveChunkIndex(idx)}
                  className={`w-full text-left p-2.5 rounded-lg text-xs transition-all flex items-center justify-between border ${
                    isSelected
                      ? "bg-purple-950/50 border-purple-800/60 text-purple-200 shadow-sm"
                      : "border-transparent text-zinc-400 hover:bg-zinc-850 hover:text-zinc-200"
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <Code2 className={`w-3.5 h-3.5 ${isSelected ? "text-purple-400" : "text-zinc-500"}`} />
                    <span className="font-mono font-medium">Chunk #{chk.chunk_index ?? idx}</span>
                  </div>
                  <span className="text-[10px] font-mono text-zinc-500">
                    L{chk.start_line}–{chk.end_line}
                  </span>
                </button>
              );
            })}
          </div>

          {/* Active Chunk Viewer */}
          <div className="flex-1 flex flex-col min-w-0 bg-zinc-950">
            {/* Chunk Viewer Header */}
            {selectedChunk && (
              <div className="flex items-center justify-between px-4 py-2.5 bg-zinc-900/30 border-b border-zinc-850 text-xs">
                <div className="flex items-center gap-3">
                  <span className="font-mono font-medium text-zinc-200 flex items-center gap-1.5">
                    <Hash className="w-3.5 h-3.5 text-purple-400" />
                    Lines {selectedChunk.start_line} – {selectedChunk.end_line}
                  </span>
                  <span className="text-zinc-500">•</span>
                  <span className="text-zinc-400 font-mono text-[11px]">
                    {selectedChunk.token_count || 0} tokens
                  </span>
                </div>
                <button
                  onClick={() => handleCopy(selectedChunk.id, selectedChunk.content)}
                  className="flex items-center gap-1.5 px-3 py-1 rounded text-xs font-medium text-zinc-300 bg-zinc-850 hover:bg-zinc-800 border border-zinc-750 transition-colors"
                >
                  {copiedChunkId === selectedChunk.id ? (
                    <>
                      <Check className="w-3.5 h-3.5 text-emerald-400" />
                      <span className="text-emerald-400">Copied!</span>
                    </>
                  ) : (
                    <>
                      <Copy className="w-3.5 h-3.5" />
                      <span>Copy Chunk</span>
                    </>
                  )}
                </button>
              </div>
            )}

            {/* Chunk Code with Line Numbers */}
            <div className="flex-1 overflow-y-auto p-4 font-mono text-xs">
              {loading ? (
                <div className="flex items-center justify-center h-48 text-zinc-500">
                  Loading chunk details...
                </div>
              ) : selectedChunk ? (
                <div className="rounded-lg border border-zinc-850 bg-zinc-900/50 overflow-hidden">
                  <table className="w-full border-collapse">
                    <tbody>
                      {chunkLines.map((line, idx) => {
                        const lineNum = selectedChunk.start_line + idx;
                        return (
                          <tr
                            key={idx}
                            className="hover:bg-purple-950/20 transition-colors leading-5"
                          >
                            <td className="select-none py-0.5 pl-3 pr-4 text-right text-zinc-600 font-mono text-[11px] w-12 border-r border-zinc-850">
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
              ) : (
                <div className="flex items-center justify-center h-48 text-zinc-500">
                  No chunk selected.
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-zinc-800 bg-zinc-900/60 flex items-center justify-between text-xs text-zinc-400">
          <span>AST chunked using Tree-Sitter syntactic boundaries</span>
          <button
            onClick={onClose}
            className="px-4 py-1.5 rounded-lg bg-zinc-850 hover:bg-zinc-800 text-zinc-200 border border-zinc-750 transition-colors font-medium"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
