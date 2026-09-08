"use client";

import React, { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  FolderGit2,
  BookOpen,
  Key,
  ShieldCheck,
  Terminal,
  Menu,
  X,
  ExternalLink,
} from "lucide-react";
import { cn } from "@/lib/utils";

export function Sidebar() {
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [showPatModal, setShowPatModal] = useState(false);
  const [patInput, setPatInput] = useState("");
  const [patSaved, setPatSaved] = useState(false);

  const navigation = [
    {
      name: "Dashboard",
      href: "/",
      icon: LayoutDashboard,
      current: pathname === "/",
    },
    {
      name: "Projects",
      href: "/projects",
      icon: FolderGit2,
      current: pathname.startsWith("/projects"),
    },
  ];

  const handleSavePat = (e: React.FormEvent) => {
    e.preventDefault();
    if (patInput.trim()) {
      localStorage.setItem("cf_pat", patInput.trim());
      setPatSaved(true);
      setTimeout(() => {
        setPatSaved(false);
        setShowPatModal(false);
      }, 1000);
    }
  };

  const navContent = (
    <div className="flex h-full flex-col justify-between p-4">
      <div className="space-y-6">
        {/* Brand */}
        <div className="flex items-center justify-between px-2">
          <Link href="/" className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-blue-600 shadow-md shadow-blue-500/20 text-white font-bold">
              <Terminal className="h-5 w-5" />
            </div>
            <div>
              <div className="flex items-center gap-1.5">
                <span className="font-semibold text-sm tracking-tight text-white">
                  ContextForge
                </span>
                <span className="rounded bg-blue-950/80 px-1.5 py-0.2 text-[10px] font-semibold text-blue-400 border border-blue-800/40">
                  v1.0
                </span>
              </div>
              <p className="text-[11px] text-zinc-400">Context & RAG Engine</p>
            </div>
          </Link>
          <button
            onClick={() => setMobileOpen(false)}
            className="md:hidden text-zinc-400 hover:text-zinc-200"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Navigation */}
        <nav className="space-y-1">
          <p className="px-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-500 mb-2">
            Navigation
          </p>
          {navigation.map((item) => {
            const Icon = item.icon;
            return (
              <Link
                key={item.name}
                href={item.href}
                onClick={() => setMobileOpen(false)}
                className={cn(
                  "flex items-center gap-3 px-3 py-2 text-sm font-medium rounded-lg transition-colors",
                  item.current
                    ? "bg-blue-600/15 text-blue-400 border border-blue-500/30"
                    : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200"
                )}
              >
                <Icon className={cn("h-4 w-4", item.current ? "text-blue-400" : "text-zinc-400")} />
                {item.name}
              </Link>
            );
          })}
        </nav>

        {/* Resources */}
        <div className="space-y-1">
          <p className="px-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-500 mb-2">
            Resources
          </p>
          <a
            href="https://github.com"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-between px-3 py-2 text-sm font-medium text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200 rounded-lg transition-colors"
          >
            <div className="flex items-center gap-3">
              <BookOpen className="h-4 w-4 text-zinc-400" />
              <span>Docs & API</span>
            </div>
            <ExternalLink className="h-3.5 w-3.5 text-zinc-500" />
          </a>
        </div>
      </div>

      {/* Footer / System Status */}
      <div className="space-y-3 pt-4 border-t border-zinc-800/80">
        <button
          onClick={() => setShowPatModal(true)}
          className="flex w-full items-center justify-between px-3 py-2 text-xs font-medium text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200 rounded-lg border border-zinc-850 transition-colors"
        >
          <div className="flex items-center gap-2">
            <Key className="h-3.5 w-3.5 text-amber-400" />
            <span>GitHub PAT</span>
          </div>
          <span className="text-[10px] text-zinc-500">Config</span>
        </button>

        <div className="flex items-center justify-between px-2 py-1 text-xs text-zinc-400">
          <div className="flex items-center gap-2">
            <span className="flex h-2 w-2 rounded-full bg-emerald-500 ring-2 ring-emerald-500/20" />
            <span className="text-[11px] text-zinc-400">Engine Online</span>
          </div>
          <ShieldCheck className="h-3.5 w-3.5 text-zinc-500" />
        </div>
      </div>

      {/* PAT Modal */}
      {showPatModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-in fade-in">
          <div className="w-full max-w-sm rounded-xl border border-zinc-800 bg-zinc-950 p-5 shadow-2xl">
            <div className="flex items-center justify-between pb-3 mb-3 border-b border-zinc-800">
              <h3 className="text-sm font-semibold text-zinc-200 flex items-center gap-2">
                <Key className="h-4 w-4 text-amber-400" />
                GitHub Personal Access Token
              </h3>
              <button
                onClick={() => setShowPatModal(false)}
                className="text-zinc-400 hover:text-zinc-200"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <p className="text-xs text-zinc-400 mb-3">
              Configure a GitHub PAT with repository read access for local development ingestion.
            </p>
            <form onSubmit={handleSavePat} className="space-y-3">
              <input
                type="password"
                placeholder="ghp_..."
                value={patInput}
                onChange={(e) => setPatInput(e.target.value)}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-200 focus:outline-none focus:border-blue-500"
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setShowPatModal(false)}
                  className="px-3 py-1.5 text-xs text-zinc-400 hover:bg-zinc-900 rounded-lg"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-500 rounded-lg"
                >
                  {patSaved ? "Saved!" : "Save Token"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );

  return (
    <>
      {/* Mobile hamburger header */}
      <div className="md:hidden flex items-center justify-between p-3 border-b border-zinc-800 bg-zinc-950">
        <Link href="/" className="flex items-center gap-2">
          <div className="flex h-7 w-7 items-center justify-center rounded bg-blue-600 text-white font-bold">
            <Terminal className="h-4 w-4" />
          </div>
          <span className="font-semibold text-sm text-white">ContextForge</span>
        </Link>
        <button
          onClick={() => setMobileOpen(true)}
          className="p-1.5 text-zinc-400 hover:text-zinc-200"
        >
          <Menu className="h-5 w-5" />
        </button>
      </div>

      {/* Desktop sidebar */}
      <aside className="hidden md:flex md:w-64 md:flex-col fixed inset-y-0 left-0 z-30 bg-zinc-950 border-r border-zinc-850">
        {navContent}
      </aside>

      {/* Mobile drawer */}
      {mobileOpen && (
        <div className="fixed inset-0 z-50 md:hidden flex">
          <div
            className="fixed inset-0 bg-black/80 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
          />
          <div className="relative w-72 bg-zinc-950 h-full border-r border-zinc-800 shadow-xl z-10">
            {navContent}
          </div>
        </div>
      )}
    </>
  );
}
