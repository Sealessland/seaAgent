import { Fragment } from "preact";
import { useEffect, useMemo, useState } from "preact/hooks";
import type { ComponentChildren } from "preact";
import { loadUIConfig, streamAgentChat } from "./api";
import type { AgentChatResponse, ChatMessage, ConversationSummary, UIConfig } from "./types";

type AsyncState<T> = {
  loading: boolean;
  data: T | null;
  error: string | null;
};

export function App() {
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string>("");
  const [ui, setUI] = useState<AsyncState<UIConfig>>({ loading: true, data: null, error: null });
  const [chat, setChat] = useState<AsyncState<AgentChatResponse>>({ loading: false, data: null, error: null });
  const [composer, setComposer] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);

  useEffect(() => {
    void (async () => {
      try {
        const result = await loadUIConfig();
        setUI({ loading: false, data: result.data, error: null });
        setComposer(result.data.default_prompt);
        document.title = result.data.title;
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setUI({ loading: false, data: null, error: message });
      }
    })();
  }, []);

  useEffect(() => {
    if (messages.length === 0 || !activeConversationId) {
      return;
    }
    setConversations((current) => upsertConversation(current, activeConversationId, messages));
  }, [messages, activeConversationId]);

  useEffect(() => {
    if (activeConversationId) {
      return;
    }
    const id = `conv-${Date.now()}`;
    setActiveConversationId(id);
    setConversations([{ id, title: "New chat", updatedAt: new Date().toISOString(), messages: [] }]);
  }, [activeConversationId]);

  const history = useMemo(
    () => messages.map((message) => ({ role: message.role, content: message.content })),
    [messages]
  );
  const isBusy = chat.loading;
  const canSend = !isBusy && composer.trim().length > 0;
  const title = ui.data?.title ?? "Jetson Agent";
  const subtitle = ui.data?.description ?? "Vision and dialogue";
  const activeConversation = conversations.find((item) => item.id === activeConversationId);

  function startNewConversation() {
    if (isBusy) {
      return;
    }
    const id = `conv-${Date.now()}`;
    setActiveConversationId(id);
    setMessages([]);
    setConversations((current) => [
      { id, title: "New chat", updatedAt: new Date().toISOString(), messages: [] },
      ...current
    ]);
  }

  function openConversation(conversation: ConversationSummary) {
    if (isBusy) {
      return;
    }
    setActiveConversationId(conversation.id);
    setMessages(conversation.messages);
  }

  function applyStarter(text: string) {
    if (isBusy) {
      return;
    }
    setComposer(text);
  }

  async function sendChat() {
    if (isBusy) {
      return;
    }
    const content = composer.trim();
    if (!content) {
      return;
    }

    const requestHistory = [...history];
    const userID = `${Date.now()}-user`;
    const assistantID = `${Date.now()}-assistant`;

    setMessages((current) => [
      ...current,
      { id: userID, role: "user", content },
      { id: assistantID, role: "assistant", content: "" }
    ]);
    setChat((current) => ({ ...current, loading: true, error: null }));

    try {
      let sessionID = activeConversationId || undefined;
      let reply = "";
      let finalResponse: AgentChatResponse | null = null;

      await streamAgentChat(
        {
          session_id: sessionID,
          message: content,
          history: requestHistory,
          include_snapshot: true
        },
        ({ event, data }) => {
          if (event === "meta" && data.session_id) {
            sessionID = data.session_id;
            setActiveConversationId(data.session_id);
          }
          if (event === "delta") {
            reply += data.content || "";
            setMessages((current) =>
              current.map((message) =>
                message.id === assistantID ? { ...message, content: reply } : message
              )
            );
          }
          if (event === "done") {
            finalResponse = data as AgentChatResponse;
          }
          if (event === "error") {
            throw new Error(data.error || "stream failed");
          }
        }
      );

      setChat({ loading: false, data: finalResponse, error: null });
      setComposer("");
      if (sessionID) {
        setActiveConversationId(sessionID);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setChat({ loading: false, data: null, error: message });
      setMessages((current) =>
        current.map((item) =>
          item.id === assistantID ? { ...item, content: `Agent error: ${message}` } : item
        )
      );
    }
  }

  return (
    <main class="app-shell">
      <aside class="sidebar">
        <div class="sidebar-brand">
          <div class="brand-mark">J</div>
          <div class="brand-copy">
            <p class="eyebrow">Agent</p>
            <h1>{title}</h1>
          </div>
        </div>
        <p class="sidebar-copy">{subtitle}</p>
        <button class="accent sidebar-new" onClick={startNewConversation} disabled={isBusy}>New chat</button>
        <div class="index-caption">
          <span>Recent</span>
          <strong>{formatCount(conversations.length, "thread")}</strong>
        </div>
        <div class="conversation-list">
          {conversations.map((conversation, index) => (
            <button
              key={conversation.id}
              class={`conversation-item ${conversation.id === activeConversationId ? "conversation-item-active" : ""}`}
              onClick={() => openConversation(conversation)}
              disabled={isBusy}
            >
              <span class="conversation-index">{String(index + 1).padStart(2, "0")}</span>
              <div class="conversation-copy">
                <strong>{conversation.title}</strong>
                <span>{formatTime(conversation.updatedAt)}</span>
              </div>
            </button>
          ))}
        </div>
      </aside>

      <section class="main-pane">
        <header class="topbar">
          <div>
            <p class="eyebrow">Conversation</p>
            <h2>{activeConversation?.title ?? "New chat"}</h2>
          </div>
          <StatusPill tone={isBusy ? "busy" : chat.error ? "error" : "idle"}>
            {isBusy ? "Thinking" : chat.error ? "Unavailable" : "Available"}
          </StatusPill>
        </header>

        <section class="chat-surface">
          <div class="chat-scroll">
            <div class="message-list">
              {messages.length === 0 ? (
                <EmptyState onSelect={applyStarter} />
              ) : (
                messages.map((message) => <ChatBubble key={message.id} message={message} />)
              )}
              {chat.loading ? (
                <div class="message message-assistant message-pending">
                  <strong>Agent</strong>
                  <p>Thinking...</p>
                </div>
              ) : null}
            </div>
          </div>

          <div class="composer composer-dock">
            <div class="composer-head">
              <div>
                <strong>Message</strong>
                <p>Ask about the scene, device state, or continue the thread.</p>
              </div>
              <StatusPill tone={canSend ? "ready" : "idle"}>{canSend ? "Ready" : "Idle"}</StatusPill>
            </div>
            <textarea
              value={composer}
              onInput={(event) => setComposer((event.target as HTMLTextAreaElement).value)}
              placeholder={isBusy ? "Agent is responding" : "Message the agent"}
              disabled={isBusy}
            />
            <div class="composer-controls">
              <button class="ghost" onClick={() => setComposer(ui.data?.default_prompt ?? "")} disabled={isBusy}>Reset</button>
              <button class="accent" onClick={sendChat} disabled={!canSend}>Send</button>
            </div>
          </div>
        </section>
      </section>
    </main>
  );
}

function ChatBubble(props: { message: ChatMessage }) {
  return (
    <article class={`message message-${props.message.role}`}>
      <strong>{props.message.role === "user" ? "You" : "Agent"}</strong>
      <div class="message-body">{renderFormattedMessage(props.message.content)}</div>
    </article>
  );
}

function EmptyState(props: { onSelect: (text: string) => void }) {
  return (
    <section class="empty-state">
      <p class="eyebrow">Start Here</p>
      <h3>Ask naturally. Keep the interface out of the way.</h3>
      <p>The app is structured like a modern agent product: a lightweight thread rail, a clean conversation surface, and readable answers.</p>
      <div class="starter-list">
        <button type="button" class="starter-chip" onClick={() => props.onSelect("Describe the current scene.")}>Describe the current scene</button>
        <button type="button" class="starter-chip" onClick={() => props.onSelect("Check whether there is any obstacle ahead.")}>Check obstacles ahead</button>
        <button type="button" class="starter-chip" onClick={() => props.onSelect("Compare the image with the current device status.")}>Compare with device status</button>
      </div>
    </section>
  );
}

function StatusPill(props: { tone: "idle" | "busy" | "error" | "ready"; children: ComponentChildren }) {
  return <span class={`status-pill status-pill-${props.tone}`}>{props.children}</span>;
}

function upsertConversation(current: ConversationSummary[], activeId: string, messages: ChatMessage[]) {
  if (!activeId) {
    return current;
  }
  const summary: ConversationSummary = {
    id: activeId,
    title: deriveConversationTitle(messages),
    updatedAt: new Date().toISOString(),
    messages
  };
  const rest = current.filter((item) => item.id !== activeId);
  return [summary, ...rest];
}

function deriveConversationTitle(messages: ChatMessage[]) {
  const firstUser = messages.find((message) => message.role === "user");
  if (!firstUser) {
    return "New chat";
  }
  return firstUser.content.slice(0, 28) || "New chat";
}

function formatTime(iso: string) {
  return new Date(iso).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
}

function formatCount(count: number, label: string) {
  return `${count} ${label}${count === 1 ? "" : "s"}`;
}

function renderFormattedMessage(content: string): ComponentChildren {
  const cleaned = sanitizeMessage(content);
  if (!cleaned) {
    return <p class="message-paragraph" />;
  }

  const lines = cleaned.split("\n");
  const blocks: ComponentChildren[] = [];
  let paragraph: string[] = [];
  let listItems: string[] = [];
  let codeFence: string[] = [];
  let inCodeFence = false;

  const flushParagraph = () => {
    if (paragraph.length === 0) {
      return;
    }
    blocks.push(<p class="message-paragraph">{renderInline(paragraph.join(" "))}</p>);
    paragraph = [];
  };

  const flushList = () => {
    if (listItems.length === 0) {
      return;
    }
    blocks.push(
      <ul class="message-list-block">
        {listItems.map((item) => <li>{renderInline(item)}</li>)}
      </ul>
    );
    listItems = [];
  };

  const flushCodeFence = () => {
    if (codeFence.length === 0) {
      blocks.push(<pre class="message-code"><code /></pre>);
      return;
    }
    blocks.push(
      <pre class="message-code">
        <code>{codeFence.join("\n")}</code>
      </pre>
    );
    codeFence = [];
  };

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    const trimmed = line.trim();

    if (trimmed.startsWith("```")) {
      flushParagraph();
      flushList();
      if (inCodeFence) {
        flushCodeFence();
      }
      inCodeFence = !inCodeFence;
      continue;
    }

    if (inCodeFence) {
      codeFence.push(rawLine);
      continue;
    }

    if (!trimmed) {
      flushParagraph();
      flushList();
      continue;
    }

    const headingMatch = trimmed.match(/^(#{1,3})\s+(.+)$/);
    if (headingMatch) {
      flushParagraph();
      flushList();
      blocks.push(
        <p class={`message-heading message-heading-${headingMatch[1].length}`}>
          {renderInline(headingMatch[2])}
        </p>
      );
      continue;
    }

    const listMatch = trimmed.match(/^[-*]\s+(.+)$/);
    if (listMatch) {
      flushParagraph();
      listItems.push(listMatch[1]);
      continue;
    }

    paragraph.push(trimmed);
  }

  flushParagraph();
  flushList();
  if (inCodeFence || codeFence.length > 0) {
    flushCodeFence();
  }

  return blocks.map((block, index) => <Fragment key={index}>{block}</Fragment>);
}

function sanitizeMessage(content: string) {
  return content
    .replace(/\uFFFD+/g, " ")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function renderInline(text: string): ComponentChildren[] {
  const parts: ComponentChildren[] = [];
  const pattern = /(`[^`]+`|\*\*[^*]+\*\*)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }

    const token = match[0];
    if (token.startsWith("**") && token.endsWith("**")) {
      parts.push(<strong>{token.slice(2, -2)}</strong>);
    } else if (token.startsWith("`") && token.endsWith("`")) {
      parts.push(<code>{token.slice(1, -1)}</code>);
    } else {
      parts.push(token);
    }
    lastIndex = match.index + token.length;
  }

  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts;
}
