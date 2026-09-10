"use client";

import React, { useState, useEffect, useRef } from "react";
import type { CreateSourceRequest, AuthMethod } from "@/types/api";
import { createSource, uploadDocument } from "@/lib/api";
import {
  X,
  GitBranch,
  Loader2,
  Key,
  ShieldCheck,
  Github,
  Globe,
  UploadCloud,
  FileText,
  AlertCircle,
} from "lucide-react";

interface AddSourceModalProps {
  projectId: string;
  isOpen: boolean;
  onClose: () => void;
  onSuccess: (result?: any) => void;
}

const ALLOWED_EXTENSIONS = [".md", ".markdown", ".txt", ".text", ".json", ".csv", ".pdf"];
const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

export function AddSourceModal({
  projectId,
  isOpen,
  onClose,
  onSuccess,
}: AddSourceModalProps) {
  const [sourceType, setSourceType] = useState<"github" | "url" | "document">("github");

  // GitHub tab state
  const [repoUrl, setRepoUrl] = useState("");
  const [repoOwner, setRepoOwner] = useState("");
  const [repoName, setRepoName] = useState("");
  const [branch, setBranch] = useState("main");
  const [authMethod, setAuthMethod] = useState<AuthMethod>("public");
  const [patToken, setPatToken] = useState("");
  const [githubLoading, setGithubLoading] = useState(false);
  const [githubError, setGithubError] = useState<string | null>(null);

  // URL tab state
  const [webUrl, setWebUrl] = useState("");
  const [webName, setWebName] = useState("");
  const [urlLoading, setUrlLoading] = useState(false);
  const [urlError, setUrlError] = useState<string | null>(null);

  // Document tab state
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [docLoading, setDocLoading] = useState(false);
  const [docProgress, setDocProgress] = useState(0);
  const [docError, setDocError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Auto-fill PAT from localStorage if available
  useEffect(() => {
    if (typeof window !== "undefined") {
      const savedPat = localStorage.getItem("cf_pat");
      if (savedPat) {
        setPatToken(savedPat);
      }
    }
  }, [isOpen]);

  // Reset errors and progress when modal opens/closes
  useEffect(() => {
    if (!isOpen) {
      setGithubError(null);
      setUrlError(null);
      setDocError(null);
      setSelectedFile(null);
      setDocProgress(0);
    }
  }, [isOpen]);

  // Parse repo owner and repo name from GitHub URL or shorthand
  const handleUrlChange = (value: string) => {
    setRepoUrl(value);
    const cleaned = value.trim().replace(/\/+$/, "");

    const ghMatch = cleaned.match(/(?:github\.com\/|git@github\.com:)?([^/\s]+)\/([^/\s#]+)/);
    if (ghMatch) {
      setRepoOwner(ghMatch[1]);
      setRepoName(ghMatch[2].replace(/\.git$/, ""));
    }
  };

  const handleGithubSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    let owner = repoOwner.trim();
    let name = repoName.trim();

    if (!owner || !name) {
      if (repoUrl) {
        const ghMatch = repoUrl.trim().match(/(?:github\.com\/|git@github\.com:)?([^/\s]+)\/([^/\s#]+)/);
        if (ghMatch) {
          owner = ghMatch[1];
          name = ghMatch[2].replace(/\.git$/, "");
        }
      }
    }

    if (!owner || !name) {
      setGithubError("Please provide a valid GitHub repository URL or specify owner and name.");
      return;
    }

    if (authMethod === "pat" && !patToken.trim()) {
      setGithubError("Please enter a GitHub Personal Access Token (PAT).");
      return;
    }

    setGithubLoading(true);
    setGithubError(null);

    try {
      const payload: CreateSourceRequest = {
        name: `${owner}/${name}`,
        type: "github",
        repo_url: repoUrl.trim() || `https://github.com/${owner}/${name}`,
        repo_owner: owner,
        repo_name: name,
        branch: branch.trim() || "main",
        auth_method: authMethod,
        pat_token: authMethod === "pat" ? patToken.trim() : undefined,
      };

      const result = await createSource(projectId, payload);
      onSuccess(result);
      onClose();
    } catch (err: any) {
      setGithubError(err.message || "Failed to connect repository.");
    } finally {
      setGithubLoading(false);
    }
  };

  const handleUrlSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = webUrl.trim();

    if (!trimmed.startsWith("http://") && !trimmed.startsWith("https://")) {
      setUrlError("URL must begin with http:// or https://");
      return;
    }

    setUrlLoading(true);
    setUrlError(null);

    try {
      const name = webName.trim() || new URL(trimmed).hostname;
      const result = await createSource(projectId, {
        name,
        type: "url",
        url: trimmed,
      });
      onSuccess(result);
      onClose();
    } catch (err: any) {
      setUrlError(err.message || "Failed to index URL.");
    } finally {
      setUrlLoading(false);
    }
  };

  const validateAndSetFile = (file: File) => {
    setDocError(null);
    const ext = "." + file.name.split(".").pop()?.toLowerCase();
    if (!ALLOWED_EXTENSIONS.includes(ext)) {
      setDocError(
        `Unsupported format "${ext}". Supported formats: Markdown (.md), Text (.txt), JSON (.json), CSV (.csv), PDF (.pdf).`
      );
      return;
    }
    if (file.size > MAX_FILE_SIZE) {
      setDocError(
        `File size (${formatFileSize(file.size)}) exceeds maximum allowed size of 10MB.`
      );
      return;
    }
    setSelectedFile(file);
  };

  const handleDocumentUpload = async () => {
    if (!selectedFile) return;

    setDocLoading(true);
    setDocError(null);
    setDocProgress(15);

    const progressInterval = setInterval(() => {
      setDocProgress((prev) => (prev < 85 ? prev + 15 : prev));
    }, 250);

    try {
      const result = await uploadDocument(projectId, selectedFile);
      clearInterval(progressInterval);
      setDocProgress(100);
      onSuccess(result);
      onClose();
    } catch (err: any) {
      clearInterval(progressInterval);
      setDocError(err.message || "Failed to upload and index document.");
    } finally {
      setDocLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="w-full max-w-lg rounded-xl border border-zinc-800 bg-zinc-950 p-6 shadow-2xl">
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-zinc-800 pb-4 mb-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-950/60 border border-blue-800/50 text-blue-400">
              {sourceType === "github" && <Github className="h-5 w-5" />}
              {sourceType === "url" && <Globe className="h-5 w-5" />}
              {sourceType === "document" && <UploadCloud className="h-5 w-5" />}
            </div>
            <div>
              <h2 className="text-lg font-semibold text-zinc-100">
                {sourceType === "github" && "Connect Repository"}
                {sourceType === "url" && "Index Web URL"}
                {sourceType === "document" && "Upload Document"}
              </h2>
              <p className="text-xs text-zinc-400">
                {sourceType === "github" && "Attach a code repository to this isolated context container."}
                {sourceType === "url" && "Crawl, chunk, and index external technical documentation or web articles."}
                {sourceType === "document" && "Upload architecture specifications, PDFs, or Markdown files for semantic search."}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Source Type Selector */}
        <div className="flex rounded-lg bg-zinc-900/90 p-1 border border-zinc-800 mb-4">
          <button
            type="button"
            onClick={() => {
              setSourceType("github");
              setGithubError(null);
            }}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-md transition-colors ${
              sourceType === "github"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <Github className="w-3.5 h-3.5" />
            <span>GitHub</span>
          </button>
          <button
            type="button"
            onClick={() => {
              setSourceType("url");
              setUrlError(null);
            }}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-md transition-colors ${
              sourceType === "url"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <Globe className="w-3.5 h-3.5" />
            <span>URL / Web</span>
          </button>
          <button
            type="button"
            onClick={() => {
              setSourceType("document");
              setDocError(null);
            }}
            className={`flex-1 flex items-center justify-center gap-1.5 py-1.5 text-xs font-medium rounded-md transition-colors ${
              sourceType === "document"
                ? "bg-blue-600 text-white shadow-sm"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <UploadCloud className="w-3.5 h-3.5" />
            <span>Document</span>
          </button>
        </div>

        {/* ================================================================= */}
        {/* URL / Web Source Form                                             */}
        {/* ================================================================= */}
        {sourceType === "url" && (
          <form onSubmit={handleUrlSubmit} className="space-y-4">
            {urlError && (
              <div className="rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
                <span>{urlError}</span>
              </div>
            )}

            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                Documentation or Web URL <span className="text-rose-400">*</span>
              </label>
              <input
                type="url"
                required
                placeholder="https://docs.python.org/3/"
                value={webUrl}
                onChange={(e) => {
                  setWebUrl(e.target.value);
                  if (!webName && e.target.value) {
                    try {
                      const parsed = new URL(e.target.value);
                      setWebName(parsed.hostname.replace(/^www\./, ""));
                    } catch {
                      // ignore parse errors while typing
                    }
                  }
                }}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                Source Name
              </label>
              <input
                type="text"
                placeholder="Python 3 Documentation"
                value={webName}
                onChange={(e) => setWebName(e.target.value)}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            </div>

            <div className="rounded-lg bg-blue-950/30 border border-blue-800/30 p-3 text-[11px] text-zinc-400 space-y-1">
              <div className="font-semibold text-blue-300 flex items-center gap-1.5">
                <Globe className="w-3.5 h-3.5 text-blue-400" />
                Crawl & Indexing Details
              </div>
              <p>
                The crawler fetches HTML content, extracts semantic body text and code snippets, chunks the text, and computes high-dimensional embeddings for project RAG.
              </p>
            </div>

            <div className="flex justify-end gap-3 pt-4 border-t border-zinc-800">
              <button
                type="button"
                onClick={onClose}
                disabled={urlLoading}
                className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={urlLoading || !webUrl.trim()}
                className="flex items-center gap-2 px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors disabled:opacity-50 shadow-sm shadow-blue-500/20"
              >
                {urlLoading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                Index URL
              </button>
            </div>
          </form>
        )}

        {/* ================================================================= */}
        {/* Document Upload Form                                              */}
        {/* ================================================================= */}
        {sourceType === "document" && (
          <div className="space-y-4">
            {docError && (
              <div className="rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
                <span>{docError}</span>
              </div>
            )}

            <input
              type="file"
              ref={fileInputRef}
              onChange={(e) => {
                if (e.target.files?.[0]) {
                  validateAndSetFile(e.target.files[0]);
                }
              }}
              accept=".md,.markdown,.txt,.text,.json,.csv,.pdf"
              className="hidden"
            />

            {!selectedFile ? (
              <div
                onDragOver={(e) => {
                  e.preventDefault();
                  setIsDragging(true);
                }}
                onDragLeave={(e) => {
                  e.preventDefault();
                  setIsDragging(false);
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  setIsDragging(false);
                  if (e.dataTransfer.files?.[0]) {
                    validateAndSetFile(e.dataTransfer.files[0]);
                  }
                }}
                onClick={() => fileInputRef.current?.click()}
                className={`border-2 border-dashed rounded-xl p-8 text-center cursor-pointer transition-all ${
                  isDragging
                    ? "border-blue-500 bg-blue-950/30"
                    : "border-zinc-800 hover:border-zinc-700 bg-zinc-900/40 hover:bg-zinc-900/70"
                }`}
              >
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-blue-950/60 border border-blue-800/50 text-blue-400 mb-3">
                  <UploadCloud className="h-6 w-6" />
                </div>
                <p className="text-sm font-semibold text-zinc-200 mb-1">
                  Drag & drop your document here, or{" "}
                  <span className="text-blue-400 underline underline-offset-2">Browse File</span>
                </p>
                <p className="text-xs text-zinc-400 mb-3">
                  Markdown (.md), Plain Text (.txt), JSON (.json), CSV (.csv), PDF (.pdf)
                </p>
                <span className="inline-block text-[11px] font-mono text-zinc-400 px-2.5 py-0.5 rounded bg-zinc-800/80 border border-zinc-700">
                  Maximum file size: 10MB
                </span>
              </div>
            ) : (
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/80 p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-blue-950/70 border border-blue-800/60 text-blue-400">
                      <FileText className="h-5 w-5" />
                    </div>
                    <div className="min-w-0">
                      <p className="text-sm font-semibold text-zinc-200 truncate">
                        {selectedFile.name}
                      </p>
                      <p className="text-xs text-zinc-400 font-mono">
                        {formatFileSize(selectedFile.size)}
                      </p>
                    </div>
                  </div>
                  {!docLoading && (
                    <button
                      type="button"
                      onClick={() => {
                        setSelectedFile(null);
                        if (fileInputRef.current) fileInputRef.current.value = "";
                      }}
                      className="p-1.5 text-zinc-400 hover:text-rose-400 hover:bg-zinc-800 rounded-lg transition-colors"
                      title="Remove file"
                    >
                      <X className="w-4 h-4" />
                    </button>
                  )}
                </div>

                {docLoading && (
                  <div className="space-y-1.5 pt-1">
                    <div className="flex justify-between text-xs text-zinc-400">
                      <span className="flex items-center gap-1.5 text-blue-400">
                        <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        Uploading, chunking & computing embeddings...
                      </span>
                      <span className="font-mono">{docProgress}%</span>
                    </div>
                    <div className="h-2 w-full bg-zinc-850 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-blue-600 rounded-full transition-all duration-300"
                        style={{ width: `${docProgress}%` }}
                      />
                    </div>
                  </div>
                )}
              </div>
            )}

            <div className="flex justify-end gap-3 pt-4 border-t border-zinc-800">
              <button
                type="button"
                onClick={onClose}
                disabled={docLoading}
                className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleDocumentUpload}
                disabled={docLoading || !selectedFile}
                className="flex items-center gap-2 px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors disabled:opacity-50 shadow-sm shadow-blue-500/20"
              >
                {docLoading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                Upload Document
              </button>
            </div>
          </div>
        )}

        {/* ================================================================= */}
        {/* GitHub Repository Form                                            */}
        {/* ================================================================= */}
        {sourceType === "github" && (
          <form onSubmit={handleGithubSubmit} className="space-y-4">
            {githubError && (
              <div className="rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
                <span>{githubError}</span>
              </div>
            )}

            {/* Repo URL Input */}
            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                Repository URL or Path <span className="text-rose-400">*</span>
              </label>
              <input
                type="text"
                required
                placeholder="https://github.com/facebook/react or owner/repo"
                value={repoUrl}
                onChange={(e) => handleUrlChange(e.target.value)}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
              />
            </div>

            {/* Owner & Repo Name (auto-parsed) */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                  Owner / Organization
                </label>
                <input
                  type="text"
                  placeholder="e.g. facebook"
                  value={repoOwner}
                  onChange={(e) => setRepoOwner(e.target.value)}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                  Repository Name
                </label>
                <input
                  type="text"
                  placeholder="e.g. react"
                  value={repoName}
                  onChange={(e) => setRepoName(e.target.value)}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
            </div>

            {/* Branch Input */}
            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5 flex items-center gap-1.5">
                <GitBranch className="w-3.5 h-3.5 text-zinc-400" />
                Default Branch
              </label>
              <input
                type="text"
                placeholder="main"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
              />
            </div>

            {/* Authentication Method */}
            <div>
              <label className="block text-xs font-medium text-zinc-300 mb-1.5">
                Access Type
              </label>
              <div className="grid grid-cols-3 gap-2">
                <button
                  type="button"
                  onClick={() => setAuthMethod("public")}
                  className={`flex flex-col items-center gap-1.5 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                    authMethod === "public"
                      ? "border-emerald-500 bg-emerald-600/15 text-emerald-300"
                      : "border-zinc-800 bg-zinc-900/60 text-zinc-400 hover:border-zinc-700"
                  }`}
                >
                  <Globe className="h-4 w-4" />
                  <span>Public</span>
                  <span className="text-[9px] font-normal opacity-70 leading-tight text-center">
                    No auth needed
                  </span>
                </button>

                <button
                  type="button"
                  onClick={() => setAuthMethod("oauth")}
                  className={`flex flex-col items-center gap-1.5 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                    authMethod === "oauth"
                      ? "border-blue-500 bg-blue-600/15 text-blue-300"
                      : "border-zinc-800 bg-zinc-900/60 text-zinc-400 hover:border-zinc-700"
                  }`}
                >
                  <ShieldCheck className="h-4 w-4" />
                  <span>GitHub App</span>
                  <span className="text-[9px] font-normal opacity-70 leading-tight text-center">
                    OAuth / App install
                  </span>
                </button>

                <button
                  type="button"
                  onClick={() => setAuthMethod("pat")}
                  className={`flex flex-col items-center gap-1.5 p-2.5 rounded-lg border text-xs font-medium transition-all ${
                    authMethod === "pat"
                      ? "border-amber-500 bg-amber-600/15 text-amber-300"
                      : "border-zinc-800 bg-zinc-900/60 text-zinc-400 hover:border-zinc-700"
                  }`}
                >
                  <Key className="h-4 w-4 text-amber-400" />
                  <span>PAT</span>
                  <span className="text-[9px] font-normal opacity-70 leading-tight text-center">
                    Personal token
                  </span>
                </button>
              </div>

              {authMethod === "public" && (
                <p className="mt-2 text-[10px] text-emerald-400/80 flex items-center gap-1">
                  <Globe className="w-3 h-3 shrink-0" />
                  Public repositories are cloned without credentials. Use PAT or GitHub App for private repos.
                </p>
              )}
              {authMethod === "oauth" && (
                <p className="mt-2 text-[10px] text-blue-400/80 flex items-center gap-1">
                  <ShieldCheck className="w-3 h-3 shrink-0" />
                  Requires GitHub App / OAuth installation configured server-side.
                </p>
              )}
            </div>

            {/* PAT Input Field */}
            {authMethod === "pat" && (
              <div className="space-y-1.5 animate-in fade-in slide-in-from-top-1 duration-150">
                <div className="flex items-center justify-between">
                  <label className="block text-xs font-medium text-zinc-300">
                    GitHub PAT Token <span className="text-rose-400">*</span>
                  </label>
                  {patToken && (
                    <button
                      type="button"
                      onClick={() => setPatToken("")}
                      className="text-[10px] text-zinc-500 hover:text-zinc-400"
                    >
                      Clear
                    </button>
                  )}
                </div>
                <input
                  type="password"
                  placeholder="ghp_..."
                  value={patToken}
                  onChange={(e) => setPatToken(e.target.value)}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
                />
                <p className="text-[10px] text-zinc-500">
                  Requires <code className="text-zinc-400">repo:read</code> scope for private or public repositories.
                </p>
              </div>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-3 pt-4 border-t border-zinc-800">
              <button
                type="button"
                onClick={onClose}
                disabled={githubLoading}
                className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={githubLoading}
                className="flex items-center gap-2 px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors disabled:opacity-50 shadow-sm shadow-blue-500/20"
              >
                {githubLoading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                Connect & Ingest
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
