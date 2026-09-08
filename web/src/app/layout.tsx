import type { Metadata } from "next";
import { Sidebar } from "@/components/sidebar";
import "./globals.css";

export const metadata: Metadata = {
  title: "ContextForge | High-Performance RAG Context Engine",
  description:
    "Enterprise-grade, high-performance RAG and context engine designed specifically for AI coding agents and developers.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className="bg-zinc-950 text-zinc-100 min-h-screen antialiased flex flex-col md:flex-row">
        <Sidebar />
        <div className="flex-1 md:pl-64 flex flex-col min-w-0">
          <main className="flex-1 min-w-0 p-4 sm:p-6 lg:p-8">
            {children}
          </main>
        </div>
      </body>
    </html>
  );
}
