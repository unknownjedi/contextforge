"use client";

import React, { useState, useRef, useEffect, useCallback } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  Send,
  StopCircle,
  FileCode,
  ArrowLeft,
  SlidersHorizontal,
  Bot,
  User,
  Sparkles,
  ExternalLink,
  ChevronDown,
  ChevronUp,
  AlertCircle,
  Cpu,
  Trash2,
  Plus,
  MessageSquare,
  PanelLeftClose,
  PanelLeftOpen,
  Loader2,
} from "lucide-react";
import type { Citation, ChatCompletionRequest, Conversation, ChatMessageRecord } from "@/types/api";
import {
  streamChat,
  listConversations,
  getConversation,
  createConversation,
  deleteConversation,
} from "@/lib/api";
import { CitationDrawer } from "@/components/citation-drawer";
import { MarkdownRenderer } from "@/components/markdown-renderer";

interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  citations?: Citation[];
  durationMs?: number;
  tokensUsed?: number;
  isStreaming?: boolean;
  provider?: string;
  model?: string;
  error?: string;
}

interface ProviderOption {
  id: string;
  name: string;
  model: string;
  displayName: string;
  description: string;
  badge: string;
  badgeColor: string;
}

const PROVIDER_OPTIONS: ProviderOption[] = [
  {
    id: "ollama",
    name: "Ollama (Local)",
    model: "qwen3.5:latest",
    displayName: "Ollama (qwen3.5:latest)",
    description: "Local private LLM running on your machine via Ollama (zero API key needed)",
    badge: "Local Ollama",
    badgeColor: "bg-emerald-950/80 text-emerald-400 border-emerald-800/60",
  },
  {
    id: "openai",
    name: "OpenAI",
    model: "gpt-4o",
    displayName: "OpenAI (gpt-4o)",
    description: "Multimodal frontier model with advanced code reasoning (requires OPENAI_API_KEY)",
    badge: "GPT-4o",
    badgeColor: "bg-blue-950/80 text-blue-400 border-blue-800/60",
  },
  {
    id: "anthropic",
    name: "Anthropic",
    model: "claude-3-5-sonnet",
    displayName: "Anthropic (claude-3-5-sonnet)",
    description: "Industry-leading code generation and refactoring speed (requires ANTHROPIC_API_KEY)",
    badge: "Claude 3.5",
    badgeColor: "bg-amber-950/80 text-amber-400 border-amber-800/60",
  },
  {
    id: "gemini",
    name: "Gemini",
    model: "gemini-1.5-pro",
    displayName: "Gemini (gemini-1.5-pro)",
    description: "2M token context window with cross-file AST analysis (requires GEMINI_API_KEY)",
    badge: "Gemini 1.5",
    badgeColor: "bg-indigo-950/80 text-indigo-400 border-indigo-800/60",
  },
  {
    id: "opencode",
    name: "OpenCode CLI",
    model: "opencode-go/deepseek-v4-flash",
    displayName: "OpenCode CLI (DeepSeek V4)",
    description: "Local OpenCode CLI with DeepSeek / GLM reasoning (zero cloud API key needed)",
    badge: "OpenCode",
    badgeColor: "bg-purple-950/80 text-purple-400 border-purple-800/60",
  },
  {
    id: "mock",
    name: "Development AI (Mock)",
    model: "mock",
    displayName: "Development AI (Mock)",
    description: "Offline simulated response agent for testing RAG context & citations without external API keys",
    badge: "Dev / Mock",
    badgeColor: "bg-zinc-800 text-zinc-300 border-zinc-700",
  },
];

function formatRelativeTime(dateString?: string): string {
  if (!dateString) return "";
  try {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);
    const diffDay = Math.floor(diffHour / 24);

    if (diffSec < 60) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHour < 24) return `${diffHour}h ago`;
    if (diffDay < 7) return `${diffDay}d ago`;
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  } catch {
    return "";
  }
}

export default function ProjectChatPage() {
  const params = useParams();
  const projectId = params?.id as string;

  // Conversations Thread State
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [loadingConversations, setLoadingConversations] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);

  // Selected Provider & Model state
  const [selectedProvider, setSelectedProvider] = useState<ProviderOption>(PROVIDER_OPTIONS[0]);
  const [showProviderDropdown, setShowProviderDropdown] = useState(false);

  // Chat messages
  const [messages, setMessages] = useState<ChatMessage[]>([
    {
      id: "welcome",
      role: "assistant",
      content:
        "### Welcome to ContextForge AI Context Assistant\n\nI have indexed the repositories in this project into high-dimensional vector space. Ask me questions about codebase architecture, specific functions, data flows, or dependency interactions.\n\nEvery response includes **structured code citations** with precise line numbers and similarity confidence scores.",
      provider: "system",
      model: "contextforge-v1",
    },
  ]);

  const [input, setInput] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [selectedCitation, setSelectedCitation] = useState<Citation | null>(null);
  const [errorBanner, setErrorBanner] = useState<string | null>(null);

  // Retrieval Settings
  const [showSettings, setShowSettings] = useState(false);
  const [topK, setTopK] = useState(5);
  const [similarityThreshold, setSimilarityThreshold] = useState(0.3);
  const [fileFilter, setFileFilter] = useState("");
  const [temperature, setTemperature] = useState(0.2);

  const abortControllerRef = useRef<AbortController | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  // Close dropdown on outside click
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowProviderDropdown(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Fetch conversations list for this project
  const loadConversations = useCallback(async () => {
    if (!projectId) return [];
    setLoadingConversations(true);
    try {
      const list = await listConversations(projectId);
      setConversations(list || []);
      return list || [];
    } catch (err: any) {
      console.error("Failed to load conversations:", err);
      return [];
    } finally {
      setLoadingConversations(false);
    }
  }, [projectId]);

  // Load conversation thread on selection
  const handleSelectConversation = useCallback(
    async (convId: string) => {
      if (activeConversationId === convId && messages.length > 0) return;

      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
        setIsStreaming(false);
      }

      setActiveConversationId(convId);
      setLoadingMessages(true);
      setErrorBanner(null);

      if (typeof window !== "undefined") {
        const url = new URL(window.location.href);
        url.searchParams.set("conv", convId);
        window.history.replaceState({}, "", `${url.pathname}?${url.searchParams.toString()}`);
      }

      try {
        const res = await getConversation(projectId, convId);
        const msgs: ChatMessageRecord[] =
          res.messages || (res as any).conversation?.messages || [];

        if (msgs.length === 0) {
          setMessages([
            {
              id: `empty-${convId}`,
              role: "assistant",
              content:
                "This conversation thread is empty. Ask a technical question below to start indexing context.",
              provider: "system",
            },
          ]);
        } else {
          const mapped: ChatMessage[] = msgs.map((m) => ({
            id: m.id,
            role: m.role === "user" ? "user" : "assistant",
            content: m.content,
            citations: m.citations,
            durationMs: m.duration_ms,
            tokensUsed: m.tokens_used,
            provider: m.role === "assistant" ? selectedProvider.name : undefined,
            model: m.role === "assistant" ? selectedProvider.model : undefined,
          }));
          setMessages(mapped);
        }
      } catch (err: any) {
        setErrorBanner(`Failed to load thread: ${err.message || err}`);
      } finally {
        setLoadingMessages(false);
      }
    },
    [activeConversationId, messages.length, projectId, selectedProvider.name, selectedProvider.model]
  );

  // Initial load on mount
  useEffect(() => {
    loadConversations().then((list) => {
      if (typeof window !== "undefined") {
        const searchParams = new URLSearchParams(window.location.search);
        const convParam = searchParams.get("conv") || searchParams.get("conversation_id");
        if (convParam && list.some((c) => c.id === convParam)) {
          handleSelectConversation(convParam);
        }
      }
    });
  }, [loadConversations, handleSelectConversation]);

  // New Chat Handler
  const handleNewChat = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsStreaming(false);
    setActiveConversationId(null);
    setMessages([
      {
        id: `welcome-${Date.now()}`,
        role: "assistant",
        content:
          "### New Conversation Thread\n\nI am ready to assist with your codebase. Ask about functions, architecture, data flows, or dependency interactions.\n\nEvery response includes **structured code citations** with precise line numbers and similarity confidence.",
        provider: "system",
        model: "contextforge-v1",
      },
    ]);
    setErrorBanner(null);

    if (typeof window !== "undefined") {
      const url = new URL(window.location.href);
      url.searchParams.delete("conv");
      url.searchParams.delete("conversation_id");
      window.history.replaceState({}, "", url.pathname);
    }
  };

  // Delete Conversation Handler
  const handleDeleteConversation = async (e: React.MouseEvent, convId: string) => {
    e.stopPropagation();
    try {
      await deleteConversation(projectId, convId);
      setConversations((prev) => prev.filter((c) => c.id !== convId));
      if (activeConversationId === convId) {
        handleNewChat();
      }
    } catch (err: any) {
      setErrorBanner(`Failed to delete conversation: ${err.message || err}`);
    }
  };

  // Stop Generation Handler
  const handleStopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsStreaming(false);

    setMessages((prev) =>
      prev.map((msg) =>
        msg.isStreaming
          ? {
              ...msg,
              isStreaming: false,
              content: msg.content
                ? `${msg.content}\n\n*(Generation stopped by user)*`
                : "*(Generation stopped by user)*",
            }
          : msg
      )
    );
  };

  // Send Message Handler
  const handleSendMessage = async (promptText?: string) => {
    const messageContent = promptText || input.trim();
    if (!messageContent || isStreaming) return;

    setErrorBanner(null);

    let currentConvId = activeConversationId;
    if (!currentConvId) {
      try {
        const newConv = await createConversation(projectId);
        currentConvId = newConv.id;
        setActiveConversationId(newConv.id);
        setConversations((prev) => [newConv, ...prev]);

        if (typeof window !== "undefined") {
          const url = new URL(window.location.href);
          url.searchParams.set("conv", newConv.id);
          window.history.replaceState({}, "", `${url.pathname}?${url.searchParams.toString()}`);
        }
      } catch (err: any) {
        console.warn("Failed to create conversation record, proceeding without persistence:", err);
      }
    }

    const userMessageId = `user-${Date.now()}`;
    const assistantMessageId = `asst-${Date.now()}`;

    const userMsg: ChatMessage = {
      id: userMessageId,
      role: "user",
      content: messageContent,
    };

    const initialAssistantMsg: ChatMessage = {
      id: assistantMessageId,
      role: "assistant",
      content: "",
      citations: [],
      isStreaming: true,
      provider: selectedProvider.name,
      model: selectedProvider.model,
    };

    // Remove welcome message placeholder if sending the first prompt
    setMessages((prev) => {
      const isOnlyWelcome =
        prev.length === 1 &&
        (prev[0].id === "welcome" ||
          prev[0].id.startsWith("welcome-") ||
          prev[0].id.startsWith("empty-"));
      return isOnlyWelcome ? [userMsg, initialAssistantMsg] : [...prev, userMsg, initialAssistantMsg];
    });

    setInput("");
    setIsStreaming(true);

    const citationsCollected: Citation[] = [];
    let fullResponse = "";
    const startTime = Date.now();

    const requestPayload: ChatCompletionRequest = {
      message: messageContent,
      provider: selectedProvider.id,
      model: selectedProvider.model,
      top_k: Number(topK),
      similarity_threshold: Number(similarityThreshold),
      temperature: Number(temperature),
      file_filters: fileFilter
        ? fileFilter.split(",").map((s) => s.trim()).filter(Boolean)
        : undefined,
      conversation_id: currentConvId || undefined,
    };

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    try {
      await streamChat(
        projectId,
        requestPayload,
        {
          onCitation: (citation) => {
            citationsCollected.push(citation);
            setMessages((prev) =>
              prev.map((msg) =>
                msg.id === assistantMessageId
                  ? { ...msg, citations: [...citationsCollected] }
                  : msg
              )
            );
          },
          onToken: (token) => {
            fullResponse += token;
            setMessages((prev) =>
              prev.map((msg) =>
                msg.id === assistantMessageId
                  ? { ...msg, content: fullResponse }
                  : msg
              )
            );
          },
          onDone: async (doneData) => {
            const duration = doneData.duration_ms || Date.now() - startTime;
            setMessages((prev) =>
              prev.map((msg) =>
                msg.id === assistantMessageId
                  ? {
                      ...msg,
                      isStreaming: false,
                      durationMs: duration,
                      tokensUsed: doneData.total_tokens || Math.round(fullResponse.length / 4),
                    }
                  : msg
              )
            );
            setIsStreaming(false);

            // Refresh conversations list to pick up backend auto-generated title
            try {
              const updated = await listConversations(projectId);
              setConversations(updated);
            } catch {
              // ignore list reload errors
            }
          },
          onError: (err) => {
            const message =
              typeof err === "string"
                ? err
                : err?.message || "Backend returned an error.";

            setMessages((prev) =>
              prev.map((msg) =>
                msg.id === assistantMessageId
                  ? {
                      ...msg,
                      isStreaming: false,
                      error: message,
                      content: "",
                    }
                  : msg
              )
            );
            setErrorBanner(`Chat error: ${message}`);
            setIsStreaming(false);
          },
        },
        abortController.signal
      );
    } catch (err: any) {
      if (err.name === "AbortError" || abortController.signal.aborted) {
        return;
      }
      setErrorBanner(`Query failed: ${err.message}`);
      setMessages((prev) =>
        prev.map((msg) =>
          msg.id === assistantMessageId
            ? {
                ...msg,
                isStreaming: false,
                content: `An error occurred while communicating with the ContextForge RAG server: ${err.message}`,
              }
            : msg
        )
      );
      setIsStreaming(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  const suggestedQueries = [
    "How does the pgvector similarity search isolate project embeddings?",
    "Explain the AST chunking strategy and Tree-Sitter parser implementation.",
    "Show where the JWT session authentication and GitHub PAT validation occur.",
    "What is the asynchronous queue architecture for repository sync jobs?",
  ];

  return (
    <div className="flex h-[calc(100vh-4.5rem)] max-w-7xl mx-auto gap-3">
      {/* ==================================================================== */}
      {/* Conversation Thread Sidebar / Drawer                                 */}
      {/* ==================================================================== */}
      <div
        className={`shrink-0 flex flex-col rounded-xl border border-zinc-800 bg-zinc-950/80 backdrop-blur-sm transition-all duration-200 overflow-hidden ${
          isSidebarOpen ? "w-72" : "w-0 border-transparent p-0 hidden md:hidden"
        }`}
      >
        {/* Sidebar Header */}
        <div className="p-3 border-b border-zinc-850 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <MessageSquare className="w-4 h-4 text-blue-400" />
            <span className="text-xs font-semibold text-zinc-200">Threads</span>
            <span className="text-[10px] font-mono text-zinc-400 bg-zinc-900 px-1.5 py-0.2 rounded border border-zinc-800">
              {conversations.length}
            </span>
          </div>
          <button
            type="button"
            onClick={handleNewChat}
            className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors shadow-sm shadow-blue-500/20"
            title="Start a new chat conversation"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>New Chat</span>
          </button>
        </div>

        {/* Conversation List */}
        <div className="flex-1 overflow-y-auto p-2 space-y-1">
          {loadingConversations ? (
            <div className="flex items-center justify-center py-8 text-zinc-500 text-xs gap-2">
              <Loader2 className="w-3.5 h-3.5 animate-spin text-blue-400" />
              <span>Loading threads...</span>
            </div>
          ) : conversations.length === 0 ? (
            <div className="p-4 text-center text-xs text-zinc-500">
              <MessageSquare className="w-6 h-6 text-zinc-700 mx-auto mb-2 opacity-50" />
              <p className="text-zinc-300 font-medium">No saved threads</p>
              <p className="text-[11px] text-zinc-500 mt-1">
                Your conversations will automatically appear here once you send a message.
              </p>
            </div>
          ) : (
            conversations.map((conv) => {
              const isActive = activeConversationId === conv.id;
              return (
                <div
                  key={conv.id}
                  onClick={() => handleSelectConversation(conv.id)}
                  className={`group flex items-center justify-between gap-2 px-3 py-2 rounded-lg text-xs cursor-pointer transition-all ${
                    isActive
                      ? "bg-blue-950/60 border border-blue-800/60 text-blue-200 shadow-sm"
                      : "hover:bg-zinc-900/80 text-zinc-300 border border-transparent"
                  }`}
                >
                  <div className="min-w-0 flex-1">
                    <p className="font-medium truncate leading-tight">
                      {conv.title || "New Chat"}
                    </p>
                    <span className="text-[10px] text-zinc-500 font-mono">
                      {formatRelativeTime(conv.updated_at || conv.created_at)}
                    </span>
                  </div>

                  <button
                    type="button"
                    onClick={(e) => handleDeleteConversation(e, conv.id)}
                    className="opacity-0 group-hover:opacity-100 p-1 text-zinc-500 hover:text-rose-400 hover:bg-zinc-800/80 rounded transition-all shrink-0"
                    title="Delete thread"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* ==================================================================== */}
      {/* Main Chat Area                                                       */}
      {/* ==================================================================== */}
      <div className="flex-1 flex flex-col min-w-0 h-full">
        {/* Top Header Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-zinc-850 pb-3 mb-3 shrink-0">
          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={() => setIsSidebarOpen(!isSidebarOpen)}
              className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-100 hover:bg-zinc-900 transition-colors"
              title={isSidebarOpen ? "Collapse sidebar" : "Show sidebar"}
            >
              {isSidebarOpen ? (
                <PanelLeftClose className="h-4 w-4" />
              ) : (
                <PanelLeftOpen className="h-4 w-4" />
              )}
            </button>

            <Link
              href={`/projects/${projectId}`}
              className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-100 hover:bg-zinc-900 transition-colors"
              title="Back to project overview"
            >
              <ArrowLeft className="h-4 w-4" />
            </Link>

            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-sm font-semibold text-white flex items-center gap-1.5">
                  <Sparkles className="h-4 w-4 text-blue-400" />
                  AI Context Chat
                </h1>
                <span className="flex items-center gap-1 text-[11px] text-emerald-400 font-mono bg-emerald-950/60 border border-emerald-800/40 px-2 py-0.5 rounded-full">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  RAG Active
                </span>
              </div>
              <p className="text-[11px] text-zinc-400">
                Project: <code className="text-blue-400 font-mono">{projectId.slice(0, 8)}</code>
                {activeConversationId && (
                  <>
                    {" "}• Thread:{" "}
                    <code className="text-zinc-400 font-mono">
                      {activeConversationId.slice(0, 8)}
                    </code>
                  </>
                )}
              </p>
            </div>
          </div>

          {/* Center/Right: Provider & Model Selector + Settings */}
          <div className="flex items-center gap-2">
            {!isSidebarOpen && (
              <button
                type="button"
                onClick={handleNewChat}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg transition-colors shadow-sm shadow-blue-500/20"
                title="Start a new chat conversation"
              >
                <Plus className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">New Chat</span>
              </button>
            )}

            {/* Provider Selector Dropdown */}
            <div className="relative" ref={dropdownRef}>
              <button
                type="button"
                onClick={() => setShowProviderDropdown(!showProviderDropdown)}
                disabled={isStreaming}
                className="flex items-center gap-2 px-3 py-1.5 rounded-lg border border-zinc-750 bg-zinc-900 hover:bg-zinc-850 text-xs text-zinc-200 transition-colors shadow-sm disabled:opacity-60"
              >
                <Cpu className="w-3.5 h-3.5 text-blue-400" />
                <div className="flex items-center gap-1.5">
                  <span className="font-semibold text-zinc-100">
                    {selectedProvider.name}
                  </span>
                  <span
                    className={`text-[10px] font-mono px-1.5 py-0.2 rounded border ${selectedProvider.badgeColor}`}
                  >
                    {selectedProvider.model}
                  </span>
                </div>
                <ChevronDown className="w-3.5 h-3.5 text-zinc-400 ml-1" />
              </button>

              {showProviderDropdown && (
                <div className="absolute right-0 mt-2 w-72 rounded-xl border border-zinc-800 bg-zinc-950 p-2 shadow-2xl z-40 animate-in fade-in zoom-in-95 duration-100">
                  <span className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-zinc-500 block">
                    Select Provider & Model
                  </span>
                  <div className="space-y-1 mt-1">
                    {PROVIDER_OPTIONS.map((opt) => {
                      const isSelected = selectedProvider.id === opt.id;
                      return (
                        <button
                          key={opt.id}
                          onClick={() => {
                            setSelectedProvider(opt);
                            setShowProviderDropdown(false);
                          }}
                          className={`w-full text-left p-2.5 rounded-lg text-xs transition-colors flex flex-col gap-1 ${
                            isSelected
                              ? "bg-blue-950/60 border border-blue-800/60 text-blue-200"
                              : "hover:bg-zinc-900 text-zinc-300 border border-transparent"
                          }`}
                        >
                          <div className="flex items-center justify-between">
                            <span className="font-semibold text-zinc-100">
                              {opt.name}
                            </span>
                            <span
                              className={`text-[10px] font-mono px-1.5 py-0.2 rounded border ${opt.badgeColor}`}
                            >
                              {opt.model}
                            </span>
                          </div>
                          <p className="text-[11px] text-zinc-400 leading-tight">
                            {opt.description}
                          </p>
                        </button>
                      );
                    })}
                  </div>
                </div>
              )}
            </div>

            {/* Retrieval Settings Toggle */}
            <button
              onClick={() => setShowSettings(!showSettings)}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border transition-colors ${
                showSettings
                  ? "bg-blue-600/20 text-blue-400 border-blue-500/50"
                  : "bg-zinc-900 text-zinc-300 border-zinc-800 hover:bg-zinc-850"
              }`}
              title="Configure RAG Retrieval Parameters"
            >
              <SlidersHorizontal className="h-3.5 w-3.5" />
              <span className="hidden sm:inline">Parameters</span>
              {showSettings ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
            </button>
          </div>
        </div>

        {/* Error Banner */}
        {errorBanner && (
          <div className="mb-3 rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300 flex items-center justify-between animate-in fade-in shrink-0">
            <div className="flex items-center gap-2">
              <AlertCircle className="w-4 h-4 text-rose-400 shrink-0" />
              <span>{errorBanner}</span>
            </div>
            <button
              onClick={() => setErrorBanner(null)}
              className="text-rose-400 hover:text-rose-200 text-xs"
            >
              Dismiss
            </button>
          </div>
        )}

        {/* ==================================================================== */}
        {/* Retrieval Settings Drawer                                            */}
        {/* ==================================================================== */}
        {showSettings && (
          <div className="mb-3 p-4 rounded-xl border border-zinc-800 bg-zinc-900/80 backdrop-blur-md grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 text-xs animate-in fade-in shrink-0">
            <div>
              <div className="flex justify-between text-zinc-300 mb-1">
                <span>Top K Chunks</span>
                <span className="font-mono text-blue-400 font-semibold">{topK}</span>
              </div>
              <input
                type="range"
                min="1"
                max="20"
                value={topK}
                onChange={(e) => setTopK(Number(e.target.value))}
                className="w-full accent-blue-500"
              />
              <span className="text-[10px] text-zinc-500">Max chunks retrieved into prompt</span>
            </div>

            <div>
              <div className="flex justify-between text-zinc-300 mb-1">
                <span>Similarity Threshold</span>
                <span className="font-mono text-blue-400 font-semibold">
                  {similarityThreshold}
                </span>
              </div>
              <input
                type="range"
                min="0"
                max="1"
                step="0.05"
                value={similarityThreshold}
                onChange={(e) => setSimilarityThreshold(Number(e.target.value))}
                className="w-full accent-blue-500"
              />
              <span className="text-[10px] text-zinc-500">Minimum cosine similarity score</span>
            </div>

            <div>
              <div className="flex justify-between text-zinc-300 mb-1">
                <span>LLM Temperature</span>
                <span className="font-mono text-blue-400 font-semibold">
                  {temperature}
                </span>
              </div>
              <input
                type="range"
                min="0"
                max="1"
                step="0.05"
                value={temperature}
                onChange={(e) => setTemperature(Number(e.target.value))}
                className="w-full accent-blue-500"
              />
              <span className="text-[10px] text-zinc-500">Determinism of generated response</span>
            </div>

            <div>
              <span className="block text-zinc-300 mb-1">File Filter Glob</span>
              <input
                type="text"
                placeholder="e.g. internal/**/*.go"
                value={fileFilter}
                onChange={(e) => setFileFilter(e.target.value)}
                className="w-full px-2.5 py-1 text-xs rounded border border-zinc-700 bg-zinc-800 text-zinc-100 placeholder-zinc-500 focus:outline-none focus:border-blue-500 font-mono"
              />
              <span className="text-[10px] text-zinc-500">Restrict context to matching paths</span>
            </div>
          </div>
        )}

        {/* ==================================================================== */}
        {/* Streaming Message Feed                                               */}
        {/* ==================================================================== */}
        {loadingMessages ? (
          <div className="flex-1 flex flex-col items-center justify-center gap-3 text-zinc-400 text-xs">
            <Loader2 className="w-6 h-6 animate-spin text-blue-400" />
            <span>Loading thread messages and citations...</span>
          </div>
        ) : (
          <div className="flex-1 overflow-y-auto space-y-4 pr-1 mb-3">
            {messages.map((message) => {
              const isUser = message.role === "user";
              return (
                <div
                  key={message.id}
                  className={`flex gap-3.5 p-4 rounded-xl border transition-all ${
                    isUser
                      ? "bg-zinc-900/60 border-zinc-800/80 ml-6 sm:ml-12"
                      : "bg-zinc-950 border-zinc-850 mr-6 sm:mr-12 shadow-sm"
                  }`}
                >
                  {/* Avatar */}
                  <div
                    className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${
                      isUser
                        ? "bg-blue-600 text-white"
                        : "bg-purple-950/80 border border-purple-800/60 text-purple-400"
                    }`}
                  >
                    {isUser ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
                  </div>

                  {/* Message Body */}
                  <div className="flex-1 min-w-0 space-y-2.5">
                    <div className="flex items-center justify-between text-xs">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-zinc-200">
                          {isUser ? "You" : "ContextForge Engine"}
                        </span>
                        {!isUser && message.provider && (
                          <span className="text-[10px] font-mono text-zinc-500 bg-zinc-900 px-1.5 py-0.2 rounded border border-zinc-800">
                            {message.provider} / {message.model}
                          </span>
                        )}
                      </div>
                      {message.durationMs !== undefined && (
                        <span className="text-[11px] text-zinc-500 font-mono">
                          {message.tokensUsed} tokens • {message.durationMs}ms
                        </span>
                      )}
                    </div>

                    {/* Error state */}
                    {message.error ? (
                      <div className="flex items-start gap-2 rounded-lg bg-rose-950/40 border border-rose-800/60 px-3 py-2.5 text-xs text-rose-300">
                        <AlertCircle className="w-3.5 h-3.5 text-rose-400 shrink-0 mt-0.5" />
                        <div>
                          <span className="font-semibold block text-rose-200">Backend error</span>
                          <span className="text-rose-400">{message.error}</span>
                        </div>
                      </div>
                    ) : (
                      /* Markdown Rendered Content */
                      <div className="text-sm text-zinc-200 leading-relaxed font-sans">
                        <MarkdownRenderer
                          content={message.content}
                          isStreaming={message.isStreaming}
                        />
                      </div>
                    )}

                    {/* Structured Citation Anchors / Pills */}
                    {message.citations && message.citations.length > 0 && (
                      <div className="mt-3 pt-3 border-t border-zinc-850 space-y-2">
                        <span className="text-[10px] font-semibold text-zinc-400 uppercase tracking-wider block">
                          Retrieved Code Citations ({message.citations.length})
                        </span>
                        <div className="flex flex-wrap gap-2">
                          {message.citations.map((citation, cIdx) => {
                            const simPct = Math.round(citation.similarity * 100);
                            return (
                              <button
                                key={cIdx}
                                onClick={() => setSelectedCitation(citation)}
                                className="group flex items-center gap-2 px-2.5 py-1.5 rounded-lg bg-zinc-900/90 border border-zinc-800 hover:border-blue-500/60 hover:bg-zinc-850 text-xs text-zinc-300 transition-all text-left shadow-sm"
                                title={`Inspect snippet from ${citation.file_path}`}
                              >
                                <FileCode className="w-3.5 h-3.5 text-blue-400 group-hover:text-blue-300 shrink-0" />
                                <span className="font-mono text-[11px] text-zinc-200 truncate max-w-[220px]">
                                  {citation.file_path}
                                </span>
                                <span className="text-[10px] font-mono text-zinc-400 bg-zinc-800/80 px-1.5 py-0.2 rounded">
                                  L{citation.start_line}–{citation.end_line}
                                </span>
                                <span className="text-[10px] text-emerald-400 font-mono font-semibold">
                                  {simPct}%
                                </span>
                                <ExternalLink className="w-3 h-3 text-zinc-500 group-hover:text-zinc-300 ml-0.5" />
                              </button>
                            );
                          })}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
            <div ref={messagesEndRef} />
          </div>
        )}

        {/* Suggested Prompts (when no query asked yet in this thread) */}
        {messages.length <= 1 && (
          <div className="mb-3 shrink-0">
            <p className="text-xs text-zinc-500 mb-1.5">Suggested queries for this codebase:</p>
            <div className="flex flex-wrap gap-2">
              {suggestedQueries.map((query, i) => (
                <button
                  key={i}
                  onClick={() => handleSendMessage(query)}
                  className="text-xs px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-900/60 hover:bg-zinc-850 hover:border-zinc-700 text-zinc-300 transition-colors text-left"
                >
                  {query}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* ==================================================================== */}
        {/* Chat Input Bar with Stop Generation and Send Controls               */}
        {/* ==================================================================== */}
        <div className="shrink-0 relative rounded-xl border border-zinc-800 bg-zinc-900/90 p-2.5 shadow-lg focus-within:border-blue-500/80 transition-colors">
          <textarea
            rows={2}
            placeholder="Ask a technical query about project code, architecture, or functions (Enter to send, Shift+Enter for newline)..."
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={isStreaming}
            className="w-full resize-none bg-transparent px-3 py-1.5 text-sm text-zinc-100 placeholder-zinc-500 focus:outline-none"
          />

          <div className="flex items-center justify-between border-t border-zinc-800/80 px-3 pt-2">
            <div className="flex items-center gap-2 text-xs text-zinc-400">
              <span className="font-medium text-zinc-300">{selectedProvider.name}</span>
              <span>•</span>
              <span className="font-mono text-zinc-400">Top {topK} chunks</span>
              <span>•</span>
              <span className="font-mono text-zinc-400">≥ {similarityThreshold} sim</span>
            </div>

            <div className="flex items-center gap-2">
              {isStreaming ? (
                <button
                  onClick={handleStopGeneration}
                  className="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-medium text-rose-300 bg-rose-950/70 border border-rose-800/70 hover:bg-rose-900/70 transition-colors shadow-sm"
                >
                  <StopCircle className="w-3.5 h-3.5 text-rose-400" />
                  Stop Generation
                </button>
              ) : (
                <button
                  onClick={() => handleSendMessage()}
                  disabled={!input.trim()}
                  className="flex items-center gap-1.5 px-4 py-1.5 rounded-lg text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 transition-colors disabled:opacity-50 shadow-sm shadow-blue-500/20"
                >
                  <Send className="w-3.5 h-3.5" />
                  Send Query
                </button>
              )}
            </div>
          </div>
        </div>

        {/* Code Snippet Drawer for Citation Anchors */}
        <CitationDrawer
          citation={selectedCitation}
          onClose={() => setSelectedCitation(null)}
        />
      </div>
    </div>
  );
}
