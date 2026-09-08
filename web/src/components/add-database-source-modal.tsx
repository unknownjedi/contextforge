"use client";

import React, { useState } from "react";
import {
  X,
  Database,
  Loader2,
  CheckCircle2,
  AlertCircle,
  ShieldAlert,
  Server,
  Layers,
  ChevronRight,
  ChevronDown,
  Info,
} from "lucide-react";
import type {
  DatabaseType,
  CreateDatabaseSourceRequest,
  DatabaseSource,
  ConnectionTestResult,
  DatabaseMetadata,
} from "@/types/api";
import {
  testDatabaseConnection,
  createDatabaseSource,
  syncDatabaseSource,
} from "@/lib/api";

interface AddDatabaseSourceModalProps {
  projectId: string;
  isOpen: boolean;
  onClose: () => void;
  onSuccess: (source: DatabaseSource) => void;
}

const DB_ENGINES: { type: DatabaseType; label: string; placeholder: string }[] = [
  {
    type: "postgres",
    label: "PostgreSQL",
    placeholder: "postgresql://user:password@localhost:5432/app_db?sslmode=disable",
  },
  {
    type: "mysql",
    label: "MySQL",
    placeholder: "mysql://user:password@localhost:3306/app_db",
  },
  {
    type: "mariadb",
    label: "MariaDB",
    placeholder: "mariadb://user:password@localhost:3306/app_db",
  },
  {
    type: "sqlite",
    label: "SQLite",
    placeholder: "sqlite:///path/to/database.db",
  },
  {
    type: "sqlserver",
    label: "SQL Server (MSSQL)",
    placeholder: "sqlserver://sa:StrongPass123@localhost:1433?database=app_db",
  },
  {
    type: "cockroachdb",
    label: "CockroachDB",
    placeholder: "postgresql://root@localhost:26257/defaultdb?sslmode=disable",
  },
];

export function AddDatabaseSourceModal({
  projectId,
  isOpen,
  onClose,
  onSuccess,
}: AddDatabaseSourceModalProps) {
  const [dbType, setDbType] = useState<DatabaseType>("postgres");
  const [sourceName, setSourceName] = useState("");
  const [connectionUrl, setConnectionUrl] = useState("");
  const [ingestionMode, setIngestionMode] = useState<"schema" | "schema_and_data">("schema");

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<ConnectionTestResult | null>(null);

  const [metadataLoading, setMetadataLoading] = useState(false);
  const [metadata, setMetadata] = useState<DatabaseMetadata | null>(null);
  const [selectedTables, setSelectedTables] = useState<Record<string, boolean>>({});

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!isOpen) return null;

  const currentEngine = DB_ENGINES.find((e) => e.type === dbType) || DB_ENGINES[0];

  const handleEngineChange = (type: DatabaseType) => {
    setDbType(type);
    setTestResult(null);
    setMetadata(null);
    setSelectedTables({});
    if (!connectionUrl || DB_ENGINES.some((e) => e.placeholder === connectionUrl)) {
      setConnectionUrl("");
    }
  };

  const handleTestConnection = async () => {
    if (!connectionUrl.trim()) {
      setError("Please enter a database connection URL.");
      return;
    }

    setTesting(true);
    setError(null);
    setTestResult(null);

    try {
      const res = await testDatabaseConnection(projectId, {
        database_type: dbType,
        connection_url: connectionUrl.trim(),
      });
      setTestResult(res);

      if (res.success && !sourceName) {
        setSourceName(
          `${currentEngine.label} ${dbType === "sqlite" ? "Local" : "Knowledge"}`
        );
      }
    } catch (err: any) {
      setTestResult({
        success: false,
        database_type: dbType,
        error_message: err.message || "Failed to test database connection.",
      });
    } finally {
      setTesting(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!sourceName.trim()) {
      setError("Please provide a name for this database source.");
      return;
    }
    if (!connectionUrl.trim()) {
      setError("Please provide a connection URL.");
      return;
    }

    setSubmitting(true);
    setError(null);

    try {
      const tablesList = Object.keys(selectedTables).filter((t) => selectedTables[t]);

      const payload: CreateDatabaseSourceRequest = {
        name: sourceName.trim(),
        database_type: dbType,
        connection_url: connectionUrl.trim(),
        configuration: {
          mode: ingestionMode,
          tables: tablesList.length > 0 ? tablesList : undefined,
        },
      };

      const created = await createDatabaseSource(projectId, payload);
      // Trigger initial background sync
      try {
        await syncDatabaseSource(projectId, created.source_id);
      } catch (syncErr) {
        // Source is created; sync error will be visible on status card
      }

      onSuccess(created);
      onClose();
    } catch (err: any) {
      setError(err.message || "Failed to register database source.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="w-full max-w-2xl max-h-[90vh] flex flex-col rounded-xl border border-zinc-800 bg-zinc-950 shadow-2xl overflow-hidden">
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-zinc-800 p-5 bg-zinc-950/80 shrink-0">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-950/60 border border-indigo-800/50 text-indigo-400">
              <Database className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-zinc-100">
                Connect External Database
              </h2>
              <p className="text-xs text-zinc-400">
                Safely introspect and index schema and relational models for RAG context.
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

        {/* Scrollable Form Content */}
        <form onSubmit={handleSubmit} className="p-6 overflow-y-auto space-y-5 flex-1">
          {error && (
            <div className="rounded-lg bg-rose-950/50 border border-rose-800/60 p-3 text-xs text-rose-300 flex items-start gap-2">
              <AlertCircle className="w-4 h-4 shrink-0 mt-0.5 text-rose-400" />
              <span>{error}</span>
            </div>
          )}

          {/* Engine Selector */}
          <div>
            <label className="block text-xs font-semibold uppercase tracking-wider text-zinc-400 mb-2">
              1. Database Engine
            </label>
            <div className="grid grid-cols-3 gap-2.5">
              {DB_ENGINES.map((engine) => (
                <button
                  key={engine.type}
                  type="button"
                  onClick={() => handleEngineChange(engine.type)}
                  className={`flex items-center gap-2.5 p-2.5 rounded-lg border text-xs text-left transition-all ${
                    dbType === engine.type
                      ? "bg-indigo-950/60 border-indigo-600 text-indigo-200 shadow-sm"
                      : "bg-zinc-900/80 border-zinc-800 text-zinc-400 hover:border-zinc-700 hover:text-zinc-200"
                  }`}
                >
                  <Server className="w-3.5 h-3.5 shrink-0 text-indigo-400" />
                  <span className="font-medium truncate">{engine.label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Source Name */}
          <div>
            <label className="block text-xs font-medium text-zinc-300 mb-1.5">
              Source Name <span className="text-rose-400">*</span>
            </label>
            <input
              type="text"
              required
              placeholder="e.g. Production PostgreSQL or App Database"
              value={sourceName}
              onChange={(e) => setSourceName(e.target.value)}
              className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
          </div>

          {/* Connection URL Input & Test Button */}
          <div>
            <div className="flex justify-between items-center mb-1.5">
              <label className="block text-xs font-medium text-zinc-300">
                Connection URL <span className="text-rose-400">*</span>
              </label>
              <span className="text-[11px] text-zinc-500">
                Encrypted at rest with AES-256-GCM
              </span>
            </div>
            <div className="flex gap-2">
              <input
                type="password"
                required
                placeholder={currentEngine.placeholder}
                value={connectionUrl}
                onChange={(e) => {
                  setConnectionUrl(e.target.value);
                  setTestResult(null);
                }}
                className="flex-1 rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 font-mono"
              />
              <button
                type="button"
                onClick={handleTestConnection}
                disabled={testing || !connectionUrl.trim()}
                className="flex items-center gap-1.5 px-3.5 py-2 text-xs font-medium rounded-lg border border-zinc-700 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 transition-colors disabled:opacity-50 shrink-0"
              >
                {testing ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin text-indigo-400" />
                ) : (
                  <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                )}
                Test Connection
              </button>
            </div>

            {/* Test Connection Feedback */}
            {testResult && (
              <div
                className={`mt-2.5 rounded-lg p-2.5 text-xs flex items-start gap-2 animate-in fade-in ${
                  testResult.success
                    ? "bg-emerald-950/50 border border-emerald-800/60 text-emerald-300"
                    : "bg-rose-950/50 border border-rose-800/60 text-rose-300"
                }`}
              >
                {testResult.success ? (
                  <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
                ) : (
                  <AlertCircle className="w-4 h-4 shrink-0 text-rose-400 mt-0.5" />
                )}
                <div className="flex-1">
                  <div className="font-medium">
                    {testResult.success ? "Connection Successful" : "Connection Failed"}
                  </div>
                  <div className="text-[11px] opacity-80 mt-0.5">
                    {testResult.success
                      ? `Connected to ${testResult.database_type} ${
                          testResult.database_version ? `(${testResult.database_version.split("\n")[0]})` : ""
                        } - Latency: ${testResult.latency_ms}ms`
                      : testResult.error_message || "Unable to reach database host."}
                  </div>
                </div>
              </div>
            )}
          </div>

          {/* Ingestion Mode */}
          <div>
            <label className="block text-xs font-semibold uppercase tracking-wider text-zinc-400 mb-2">
              2. Ingestion Mode
            </label>
            <div className="grid grid-cols-2 gap-3">
              <button
                type="button"
                onClick={() => setIngestionMode("schema")}
                className={`p-3 rounded-lg border text-left transition-all ${
                  ingestionMode === "schema"
                    ? "bg-indigo-950/60 border-indigo-600 text-indigo-200"
                    : "bg-zinc-900/70 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                }`}
              >
                <div className="font-semibold text-xs text-zinc-100 flex items-center gap-1.5">
                  <Layers className="w-3.5 h-3.5 text-indigo-400" />
                  Schema Only (Recommended)
                </div>
                <p className="text-[11px] text-zinc-400 mt-1">
                  Indexes DDL, column types, primary/foreign keys, indexes, and table comments. Zero row data ingested.
                </p>
              </button>

              <button
                type="button"
                onClick={() => setIngestionMode("schema_and_data")}
                className={`p-3 rounded-lg border text-left transition-all ${
                  ingestionMode === "schema_and_data"
                    ? "bg-amber-950/60 border-amber-600 text-amber-200"
                    : "bg-zinc-900/70 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                }`}
              >
                <div className="font-semibold text-xs text-amber-200 flex items-center gap-1.5">
                  <Database className="w-3.5 h-3.5 text-amber-400" />
                  Schema & Data Records
                </div>
                <p className="text-[11px] text-zinc-400 mt-1">
                  Indexes schema and sample row batches. Sensitive columns (passwords, tokens, SSN) are auto-excluded.
                </p>
              </button>
            </div>

            {/* Row-Level Ingestion Security Warning */}
            {ingestionMode === "schema_and_data" && (
              <div className="mt-3 rounded-lg bg-amber-950/40 border border-amber-800/60 p-3 text-xs text-amber-300 flex items-start gap-2.5 animate-in fade-in">
                <ShieldAlert className="w-4 h-4 shrink-0 text-amber-400 mt-0.5" />
                <div className="space-y-1">
                  <div className="font-semibold">Security & Privacy Warning</div>
                  <p className="text-[11px] text-amber-300/90 leading-relaxed">
                    Database records may contain personal or confidential information.
                    Only enable row ingestion for non-sensitive public reference tables.
                    Credentials and tokens will be automatically excluded, but review table contents carefully before indexing.
                  </p>
                </div>
              </div>
            )}
          </div>

          {/* Security & Read-Only Notice */}
          <div className="rounded-lg border border-zinc-800/80 bg-zinc-900/40 p-3 flex items-start gap-2.5 text-xs text-zinc-400">
            <Info className="w-4 h-4 shrink-0 text-blue-400 mt-0.5" />
            <div className="text-[11px] leading-relaxed">
              <span className="font-medium text-zinc-300">Read-Only Safety Guarantee:</span>{" "}
              ContextForge connectors execute strictly read-only queries with bounded concurrency.
              For optimal security, use a dedicated database user granted only{" "}
              <code className="text-zinc-300 bg-zinc-800 px-1 py-0.5 rounded">SELECT</code> permissions.
            </div>
          </div>
        </form>

        {/* Modal Actions */}
        <div className="flex justify-end gap-3 p-4 border-t border-zinc-800 bg-zinc-950/90 shrink-0">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-900 rounded-lg border border-zinc-800 transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={submitting || !connectionUrl.trim() || !sourceName.trim()}
            className="flex items-center gap-2 px-4 py-2 text-xs font-medium text-white bg-indigo-600 hover:bg-indigo-500 rounded-lg transition-colors disabled:opacity-50 shadow-sm shadow-indigo-500/20"
          >
            {submitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            Connect & Ingest
          </button>
        </div>
      </div>
    </div>
  );
}
