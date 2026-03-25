import { Fragment } from "preact";
import { useEffect, useMemo, useState } from "preact/hooks";
import type { ComponentChildren } from "preact";
import { loadUIConfig, streamAgentChat } from "./api";
import type {
  AgentAction,
  AgentChatResponse,
  ChatMessage,
  ConversationSummary,
  DeviceSnapshot,
  FleetSnapshot,
  UIConfig
} from "./types";

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
    () =>
      messages.map((message) => ({ role: message.role, content: message.content })),
    [messages]
  );
  const isBusy = chat.loading;
  const canSend = !isBusy && composer.trim().length > 0;
  const lastResponse = chat.data;
  const latestCapture = lastResponse?.capture;
  const latestFleet = lastResponse?.peripherals;
  const latestSources = lastResponse?.sources ?? [];
  const latestTrace = lastResponse?.trace;
  const messageCount = messages.filter((message) => message.role !== "system").length;
  const assistantReplies = messages.filter((message) => message.role === "assistant" && message.content.trim()).length;
  const activeActions = latestTrace?.actions.filter((item) => item.enabled) ?? [];
  const primaryDevice = latestFleet?.primary_capture_device || latestCapture?.camera_sn || "--";

  function startNewConversation() {
    if (isBusy) {
      return;
    }
    const id = `conv-${Date.now()}`;
    setActiveConversationId(id);
    setMessages([]);
    setConversations((current) => [{ id, title: "New chat", updatedAt: new Date().toISOString(), messages: [] }, ...current]);
  }

  function openConversation(conversation: ConversationSummary) {
    if (isBusy) {
      return;
    }
    setActiveConversationId(conversation.id);
    setMessages(conversation.messages);
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

  const title = ui.data?.title ?? "Jetson Agent Console";
  const activeConversation = conversations.find((item) => item.id === activeConversationId);

  return (
    <main class="console-shell">
      <aside class="sidebar">
        <div class="sidebar-brand">
          <p class="eyebrow">Agent</p>
          <h1>{title}</h1>
          <p>{ui.data?.description ?? "用于视觉问答、外设状态理解和实时观察的工作台。"}</p>
        </div>
        <section class="sidebar-metrics">
          <Metric label="Active Thread" value={activeConversation ? formatCount(messageCount, "turn") : "--"} />
          <Metric label="Agent Replies" value={String(assistantReplies)} />
          <Metric label="Primary Input" value={truncate(primaryDevice, 18)} />
        </section>
        <button class="accent sidebar-new" onClick={startNewConversation} disabled={isBusy}>新建对话</button>
        <div class="conversation-list">
          {conversations.map((conversation) => (
            <button
              key={conversation.id}
              class={`conversation-item ${conversation.id === activeConversationId ? "conversation-item-active" : ""}`}
              onClick={() => openConversation(conversation)}
              disabled={isBusy}
            >
              <strong>{conversation.title}</strong>
              <span>{formatTime(conversation.updatedAt)}</span>
            </button>
          ))}
        </div>
      </aside>

      <section class="main-pane main-pane-full">
        <header class="chat-header workspace-hero">
          <div class="workspace-copy">
            <p class="eyebrow">Workspace</p>
            <h2>{activeConversation?.title ?? "New chat"}</h2>
            <p>{isBusy ? "模型正在生成回答，同时会保留会话状态和上下文。" : "从左侧切换会话，在中间完成提问，在右侧查看证据、动作和外设上下文。"}</p>
          </div>
          <div class="workspace-pulse">
            <StatusPill tone={isBusy ? "busy" : chat.error ? "error" : "idle"}>
              {isBusy ? "Streaming" : chat.error ? "Error" : "Ready"}
            </StatusPill>
            <div class="workspace-kpis">
              <Metric label="Sources" value={String(latestSources.length)} />
              <Metric label="Tool Calls" value={String(latestTrace?.tool_calls?.length ?? 0)} />
            </div>
          </div>
        </header>

        <section class="chat-overview">
          <div class="overview-block">
            <span>System Prompt</span>
            <strong>{truncate(ui.data?.default_prompt ?? "未加载默认提示词", 120)}</strong>
          </div>
          <div class="overview-block">
            <span>Current Intent</span>
            <strong>{formatIntent(latestTrace?.intent)}</strong>
          </div>
          <div class="overview-block">
            <span>Active Actions</span>
            <strong>{activeActions.length ? activeActions.map((item) => item.label).join(" / ") : "Waiting for next turn"}</strong>
          </div>
        </section>

        <div class="chat-stream">
          <div class="message-list">
            {messages.length === 0 ? (
              <EmptyState />
            ) : (
              messages.map((message) => <ChatBubble key={message.id} message={message} />)
            )}
            {chat.loading ? (
              <div class="message message-assistant">
                <strong>Agent</strong>
                <p>处理中…</p>
              </div>
            ) : null}
          </div>
        </div>

        <div class="composer composer-dock">
          <div class="composer-head">
            <div>
              <strong>Ask the agent</strong>
              <p>默认会附带上下文快照；如果问题涉及当前画面、状态或外设，右侧会同步显示证据。</p>
            </div>
            <StatusPill tone={canSend ? "ready" : "idle"}>{canSend ? "Input ready" : "Waiting"}</StatusPill>
          </div>
          <textarea
            value={composer}
            onInput={(event) => setComposer((event.target as HTMLTextAreaElement).value)}
            placeholder={isBusy ? "Agent 正在处理上一条消息" : "输入问题"}
            disabled={isBusy}
          />
          <div class="composer-controls">
            <button class="accent" onClick={sendChat} disabled={!canSend}>发送</button>
          </div>
        </div>
      </section>

      <aside class="inspector">
        <section class="panel inspector-section">
          <header class="panel-head">
            <div>
              <h2>Response Context</h2>
              <p>当前回答的推理入口和执行动作。</p>
            </div>
          </header>
          <div class="trace-stack">
            <div class="trace-block">
              <span>Intent</span>
              <strong>{formatIntent(latestTrace?.intent)}</strong>
            </div>
            <ActionList actions={latestTrace?.actions ?? []} />
            <ToolCallList response={lastResponse} />
          </div>
        </section>

        <section class="panel inspector-section">
          <header class="panel-head">
            <div>
              <h2>Retrieved Sources</h2>
              <p>模型回答前拼接进上下文的检索结果。</p>
            </div>
          </header>
          {latestSources.length ? (
            <div class="source-list">
              {latestSources.map((source) => (
                <article class="source-item" key={source.id}>
                  <div class="source-head">
                    <strong>{source.title}</strong>
                    <span>{source.score}</span>
                  </div>
                  <p>{source.snippet}</p>
                </article>
              ))}
            </div>
          ) : (
            <p class="empty">本轮没有命中检索上下文。</p>
          )}
        </section>

        <section class="panel inspector-section">
          <header class="panel-head">
            <div>
              <h2>Capture and Devices</h2>
              <p>最新一轮返回的图像与外设快照。</p>
            </div>
          </header>
          <CapturePanel capture={latestCapture} />
          <PeripheralPanel fleet={latestFleet} />
        </section>
      </aside>
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

function EmptyState() {
  return (
    <section class="empty-state">
      <p class="eyebrow">Ready</p>
      <h3>从一个具体观察任务开始。</h3>
      <p>例如询问当前画面、障碍物、设备状态，或者让 agent 对最新图像和外设信息做交叉判断。</p>
      <div class="empty-state-grid">
        <div>
          <span>Visual reasoning</span>
          <strong>“看一下前方是否有障碍物。”</strong>
        </div>
        <div>
          <span>Cross-sensor check</span>
          <strong>“画面和外设状态是否一致？”</strong>
        </div>
      </div>
    </section>
  );
}

function StatusPill(props: { tone: "idle" | "busy" | "error" | "ready"; children: ComponentChildren }) {
  return <span class={`status-pill status-pill-${props.tone}`}>{props.children}</span>;
}

function Metric(props: { label: string; value: string }) {
  return (
    <div class="metric">
      <span>{props.label}</span>
      <strong>{props.value}</strong>
    </div>
  );
}

function ActionList(props: { actions: AgentAction[] }) {
  if (props.actions.length === 0) {
    return <p class="empty">本轮还没有动作轨迹。</p>;
  }

  return (
    <div class="trace-action-list">
      {props.actions.map((action) => (
        <div class={`trace-chip ${action.enabled ? "trace-chip-on" : ""}`} key={action.id}>
          <strong>{action.label}</strong>
          <span>{action.description}</span>
        </div>
      ))}
    </div>
  );
}

function ToolCallList(props: { response: AgentChatResponse | null }) {
  const toolCalls = props.response?.trace?.tool_calls ?? [];
  if (toolCalls.length === 0) {
    return <p class="empty">没有发生工具调用。</p>;
  }

  return (
    <div class="tool-call-list">
      {toolCalls.map((call, index) => (
        <article class="trace-block" key={`${call.name}-${index}`}>
          <div class="source-head">
            <strong>{call.name}</strong>
            <span>tool</span>
          </div>
          <pre>{JSON.stringify({ input: call.input, output: call.output }, null, 2)}</pre>
        </article>
      ))}
    </div>
  );
}

function CapturePanel(props: { capture: AgentChatResponse["capture"] }) {
  if (!props.capture) {
    return <p class="empty">当前没有新的捕获结果。</p>;
  }

  return (
    <div class="capture-stack">
      <div class="trace-block">
        <span>Capture Status</span>
        <strong>{props.capture.ok ? "ready" : "degraded"}</strong>
      </div>
      <div class="capture-grid">
        <Metric label="Resolution" value={props.capture.width && props.capture.height ? `${props.capture.width}×${props.capture.height}` : "--"} />
        <Metric label="Camera" value={props.capture.camera_sn || "--"} />
      </div>
      {props.capture.output ? <p class="path-note">{props.capture.output}</p> : null}
      {props.capture.error ? <p class="error-note">{props.capture.error}</p> : null}
    </div>
  );
}

function PeripheralPanel(props: { fleet: FleetSnapshot | null | undefined }) {
  const devices = props.fleet?.devices ?? [];
  if (devices.length === 0) {
    return <p class="empty">当前没有附带外设快照。</p>;
  }

  return (
    <div class="compact-device-list">
      {devices.map((device) => <CompactDevice key={device.name} device={device} />)}
    </div>
  );
}

function CompactDevice(props: { device: DeviceSnapshot }) {
  return (
    <article class="device compact-device">
      <div class="device-head">
        <div>
          <p class="device-kind">{props.device.kind}</p>
          <h3>{props.device.name}</h3>
        </div>
        <span>{props.device.driver}</span>
      </div>
      <p class="device-summary">{props.device.summary}</p>
    </article>
  );
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

function formatIntent(intent?: string) {
  if (!intent) {
    return "Not inferred yet";
  }
  return intent
    .split("_")
    .map((item) => item.charAt(0).toUpperCase() + item.slice(1))
    .join(" ");
}

function formatCount(count: number, label: string) {
  return `${count} ${label}${count === 1 ? "" : "s"}`;
}

function truncate(text: string, max: number) {
  if (text.length <= max) {
    return text;
  }
  return `${text.slice(0, max - 1)}…`;
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
    blocks.push(
      <p class="message-paragraph">
        {renderInline(paragraph.join(" "))}
      </p>
    );
    paragraph = [];
  };

  const flushList = () => {
    if (listItems.length === 0) {
      return;
    }
    blocks.push(
      <ul class="message-list-block">
        {listItems.map((item) => (
          <li>{renderInline(item)}</li>
        ))}
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
