import { useRef, useState } from "react";
import { motion } from "framer-motion";
import { apiFetch } from "../apiFetch";

interface ChatMessage {
  role: "user" | "assistant" | "error";
  content: string;
}

const SUGGESTIONS = [
  "What should I fix first, and why?",
  "Why did my bill spike?",
  "Which team is spending the most?",
  "Have any of my applied fixes actually worked?",
];

// Real grounded chat, not a scripted demo: every answer comes from the
// model calling real tenant-scoped tools against /api/v1/chat's backend
// (see controlplane/chat_tools.go) -- if there's no real data to answer
// a question, the assistant says so instead of inventing a number.
export function AssistantPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);

  async function send(text: string) {
    if (!text.trim() || sending) return;
    const history = messages
      .filter((m) => m.role !== "error")
      .map((m) => ({ role: m.role, content: m.content }));

    setMessages((prev) => [...prev, { role: "user", content: text }]);
    setInput("");
    setSending(true);

    try {
      const res = await apiFetch("/api/v1/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: text, history }),
      });

      if (res.status === 503) {
        setUnavailable(true);
        setMessages((prev) => [
          ...prev,
          { role: "error", content: "The chat assistant isn't configured on this control plane (no GROQ_API_KEY set)." },
        ]);
        return;
      }
      if (!res.ok) {
        setMessages((prev) => [...prev, { role: "error", content: `Couldn't get a response (HTTP ${res.status}).` }]);
        return;
      }

      const data: { reply: string } = await res.json();
      setMessages((prev) => [...prev, { role: "assistant", content: data.reply }]);
    } catch {
      setMessages((prev) => [...prev, { role: "error", content: "Network error reaching the assistant." }]);
    } finally {
      setSending(false);
      requestAnimationFrame(() => listRef.current?.scrollTo({ top: listRef.current.scrollHeight, behavior: "smooth" }));
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex h-[calc(100vh-8rem)] flex-col gap-4"
    >
      <div>
        <h2 className="text-lg font-semibold">Assistant</h2>
        <p className="text-sm text-[var(--ink-muted)]">
          Answers using your real ingested data — findings, anomalies, workload trends, team spend, and fix outcomes.
          It never invents a number it can't look up.
        </p>
      </div>

      <div
        ref={listRef}
        className="flex-1 overflow-y-auto rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
      >
        {messages.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
            <p className="text-sm text-[var(--ink-muted)]">Ask about your real cost data.</p>
            <div className="flex flex-wrap justify-center gap-2">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => send(s)}
                  className="rounded-full border border-[var(--border)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] hover:border-[var(--accent)] hover:text-[var(--accent)]"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {messages.map((m, i) => (
              <div key={i} className={`flex ${m.role === "user" ? "justify-end" : "justify-start"}`}>
                <div
                  className={`max-w-[80%] rounded-2xl px-3.5 py-2 text-sm ${
                    m.role === "user"
                      ? "bg-[var(--accent)] text-white"
                      : m.role === "error"
                        ? "border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 text-[var(--status-critical)]"
                        : "bg-[var(--surface-sunken)] text-[var(--ink)]"
                  }`}
                >
                  {m.content}
                </div>
              </div>
            ))}
            {sending && (
              <div className="flex justify-start">
                <div className="rounded-2xl bg-[var(--surface-sunken)] px-3.5 py-2 text-sm text-[var(--ink-muted)]">Thinking…</div>
              </div>
            )}
          </div>
        )}
      </div>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          send(input);
        }}
        className="flex gap-2"
      >
        <input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          disabled={unavailable}
          placeholder={unavailable ? "Assistant not configured" : "Ask about your real cost data…"}
          className="flex-1 rounded-xl border border-[var(--border)] bg-[var(--page)] px-3.5 py-2.5 text-sm outline-none focus:border-[var(--accent)] disabled:opacity-50"
        />
        <button
          type="submit"
          disabled={sending || unavailable || !input.trim()}
          className="rounded-xl bg-[var(--accent)] px-4 py-2.5 text-sm font-medium text-white disabled:opacity-50"
        >
          Send
        </button>
      </form>
    </motion.div>
  );
}
