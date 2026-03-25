import type { ComponentChildren } from "preact";
import { useEffect, useMemo, useState } from "preact/hooks";
import {
  analyzeFrame,
  captureFrame,
  latestPreviewURL,
  loadAgentCapabilities,
  loadHealth,
  loadPeripherals,
  loadUIConfig,
  sendAgentChat
} from "./api";
import type {
  ActivityEvent,
  ActivityLevel,
  AgentCapabilities,
  AgentChatResponse,
  ChatMessage,
  FleetSnapshot,
  HealthStatus,
  UIConfig
} from "./types";

type AsyncState<T> = {
  loading: boolean;
  data: T | null;
  error: string | null;
  durationMs?: number;
};

function createActivity(label: string, detail: string, level: ActivityLevel): ActivityEvent {
  return {
    id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    label,
    detail,
    level,
    at: new Date().toLocaleTimeString("zh-CN", { hour12: false })
  };
}

function statusTone(ok: boolean | undefined) {
  if (ok === undefined) {
    return "idle";
  }
  return ok ? "good" : "bad";
}

export function App() {
  const [ui, setUI] = useState<AsyncState<UIConfig>>({ loading: true, data: null, error: null });
  const [health, setHealth] = useState<AsyncState<HealthStatus>>({ loading: false, data: null, error: null });
  const [caps, setCaps] = useState<AsyncState<AgentCapabilities>>({ loading: false, data: null, error: null });
  const [fleet, setFleet] = useState<AsyncState<FleetSnapshot>>({ loading: false, data: null, error: null });
  const [chat, setChat] = useState<AsyncState<AgentChatResponse>>({ loading: false, data: null, error: null });
  const [composer, setComposer] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [previewURL, setPreviewURL] = useState("");
  const [useLatestImage, setUseLatestImage] = useState(true);
  const [captureFresh, setCaptureFresh] = useState(false);
  const [includeSnapshot, setIncludeSnapshot] = useState(true);
  const [activity, setActivity] = useState<ActivityEvent[]>([]);

  function pushActivity(label: string, detail: string, level: ActivityLevel) {
    setActivity((current) => [createActivity(label, detail, level), ...current].slice(0, 12));
  }

  useEffect(() => {
    void (async () => {
      try {
        const [uiResult, capsResult] = await Promise.all([loadUIConfig(), loadAgentCapabilities()]);
        setUI({ loading: false, data: uiResult.data, error: null, durationMs: uiResult.durationMs });
        setCaps({ loading: false, data: capsResult.data, error: null, durationMs: capsResult.durationMs });
        setComposer(uiResult.data.default_prompt);
        document.title = uiResult.data.title;
        pushActivity("boot", `Agent console ready in ${Math.max(uiResult.durationMs, capsResult.durationMs)}ms`, "success");
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setUI({ loading: false, data: null, error: message });
        pushActivity("boot", message, "error");
      }
    })();
  }, []);

  const history = useMemo(
    () =>
      messages
        .filter((message) => message.role !== "system")
        .map((message) => ({ role: message.role, content: message.content })),
    [messages]
  );

  async function refreshHealth() {
    setHealth((current) => ({ ...current, loading: true, error: null }));
    try {
      const result = await loadHealth();
      setHealth({ loading: false, data: result.data, error: null, durationMs: result.durationMs });
      pushActivity("health", `vLLM responded in ${result.durationMs}ms`, result.ok ? "success" : "error");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setHealth({ loading: false, data: null, error: message });
      pushActivity("health", message, "error");
    }
  }

  async function refreshFleet() {
    setFleet((current) => ({ ...current, loading: true, error: null }));
    try {
      const result = await loadPeripherals();
      setFleet({ loading: false, data: result.data, error: null, durationMs: result.durationMs });
      pushActivity("peripherals", `${result.data.devices.length} devices refreshed`, "success");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setFleet({ loading: false, data: null, error: message });
      pushActivity("peripherals", message, "error");
    }
  }

  async function runCapture() {
    try {
      const result = await captureFrame();
      if (!result.ok || !result.data.ok) {
        pushActivity("capture", result.data.error || "capture failed", "error");
        return;
      }
      setPreviewURL(latestPreviewURL());
      pushActivity("capture", `Frame captured in ${result.durationMs}ms`, "success");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      pushActivity("capture", message, "error");
    }
  }

  async function runAnalyzeSnapshot() {
    try {
      const result = await analyzeFrame(composer);
      if (result.data.capture?.ok) {
        setPreviewURL(latestPreviewURL());
      }
      if (result.data.peripherals) {
        setFleet({ loading: false, data: result.data.peripherals, error: null });
      }
      pushActivity("analyze", result.data.error || `Snapshot analysis in ${result.durationMs}ms`, result.data.error ? "error" : "success");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      pushActivity("analyze", message, "error");
    }
  }

  async function sendChat() {
    const content = composer.trim();
    if (!content) {
      return;
    }

    const nextUserMessage: ChatMessage = {
      id: `${Date.now()}-user`,
      role: "user",
      content
    };
    setMessages((current) => [...current, nextUserMessage]);
    setChat((current) => ({ ...current, loading: true, error: null }));

    try {
      const result = await sendAgentChat({
        message: content,
        history,
        capture_fresh: captureFresh,
        use_latest_image: useLatestImage,
        include_snapshot: includeSnapshot
      });

      setChat({ loading: false, data: result.data, error: null, durationMs: result.durationMs });
      if (result.data.capture?.ok) {
        setPreviewURL(latestPreviewURL());
      }
      if (result.data.peripherals) {
        setFleet({ loading: false, data: result.data.peripherals, error: null });
      }

      setMessages((current) => [
        ...current,
        {
          id: `${Date.now()}-assistant`,
          role: "assistant",
          content: result.data.reply || result.data.error || "Agent returned no reply."
        }
      ]);
      setComposer("");
      pushActivity("chat", result.data.error || `Agent replied in ${result.durationMs}ms`, result.data.error ? "error" : "success");
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setChat({ loading: false, data: null, error: message });
      setMessages((current) => [
        ...current,
        {
          id: `${Date.now()}-error`,
          role: "assistant",
          content: `Agent error: ${message}`
        }
      ]);
      pushActivity("chat", message, "error");
    }
  }

  const title = ui.data?.title ?? "Jetson Agent Console";
  const description = ui.data?.description ?? "Chat-first interface for multimodal perception and embedded peripheral context.";

  return (
    <main class="console-shell">
      <section class="hero">
        <div class="hero-copy">
          <p class="eyebrow">Blue-Purple Agent Frontend</p>
          <h1>{title}</h1>
          <p class="lede">{description}</p>
        </div>
        <div class="hero-stats">
          <MetricCard label="Agent" value={caps.data?.name ?? "loading"} tone={statusTone(Boolean(caps.data))} />
          <MetricCard label="vLLM" value={health.durationMs ? `${health.durationMs} ms` : "idle"} tone={statusTone(health.data?.ok)} />
          <MetricCard label="Peripheral fleet" value={fleet.data ? `${fleet.data.devices.length} online` : "idle"} tone={statusTone(Boolean(fleet.data))} />
        </div>
      </section>

      <section class="console-grid">
        <section class="chat-panel panel">
          <header class="panel-head">
            <div>
              <h2>Agent Dialogue</h2>
              <p>对话、追问、带图像上下文的现场分析都走这里。</p>
            </div>
            <div class="panel-actions">
              <button onClick={refreshHealth}>检查模型</button>
              <button onClick={refreshFleet}>刷新外设</button>
              <button onClick={runCapture}>抓一帧</button>
            </div>
          </header>

          <div class="chat-stream">
            {messages.length === 0 ? (
              <div class="message message-system">
                <strong>Agent</strong>
                <p>输入问题后，agent 会结合最近图片、实时抓帧和外设快照回复。</p>
              </div>
            ) : (
              messages.map((message) => <ChatBubble key={message.id} message={message} />)
            )}
            {chat.loading ? (
              <div class="message message-assistant">
                <strong>Agent</strong>
                <p>正在思考并请求图像/外设上下文…</p>
              </div>
            ) : null}
          </div>

          <div class="composer">
            <textarea
              value={composer}
              onInput={(event) => setComposer((event.target as HTMLTextAreaElement).value)}
              placeholder="向 agent 提问，例如：当前前方是否有可疑障碍？雷达和深度相机状态是否一致？"
            />
            <div class="composer-controls">
              <label><input type="checkbox" checked={useLatestImage} onChange={(event) => setUseLatestImage((event.target as HTMLInputElement).checked)} /> 使用最近图像</label>
              <label><input type="checkbox" checked={captureFresh} onChange={(event) => setCaptureFresh((event.target as HTMLInputElement).checked)} /> 先抓新图</label>
              <label><input type="checkbox" checked={includeSnapshot} onChange={(event) => setIncludeSnapshot((event.target as HTMLInputElement).checked)} /> 附带外设快照</label>
              <button class="accent" onClick={sendChat}>发送</button>
            </div>
          </div>
        </section>

        <aside class="side-column">
          <Panel
            title="Capabilities"
            subtitle="先把 agent 基础能力补齐，再扩展工具链。"
            actions={<button onClick={runAnalyzeSnapshot}>单次分析</button>}
          >
            <div class="capability-list">
              {caps.data?.capabilities?.map((capability) => (
                <div class="capability" key={capability.id}>
                  <strong>{capability.name}</strong>
                  <p>{capability.description}</p>
                </div>
              )) || <div class="empty">Capabilities not loaded.</div>}
            </div>
          </Panel>

          <Panel title="Live Preview" subtitle="最近采集图像和对话里的视觉上下文。">
            <div class="preview-card">
              <img class="preview" src={previewURL} alt="Latest capture preview" />
            </div>
          </Panel>

          <Panel title="Activity" subtitle="基础可观测性：动作耗时、成功和失败。">
            <div class="activity-list">
              {activity.length === 0 ? <div class="empty">No events yet.</div> : activity.map((entry) => <ActivityRow key={entry.id} entry={entry} />)}
            </div>
          </Panel>
        </aside>
      </section>

      <section class="bottom-grid">
        <Panel title="Peripheral Fleet" subtitle="所有外设统一注入，便于 agent 消费。">
          <div class="device-grid">
            {fleet.data?.devices?.length ? fleet.data.devices.map((device) => <DeviceCard key={device.name} device={device} />) : <div class="empty">Load the fleet snapshot to inspect devices.</div>}
          </div>
        </Panel>
      </section>
    </main>
  );
}

function Panel(props: { title: string; subtitle: string; actions?: ComponentChildren; children: ComponentChildren }) {
  return (
    <section class="panel">
      <header class="panel-head">
        <div>
          <h2>{props.title}</h2>
          <p>{props.subtitle}</p>
        </div>
        <div class="panel-actions">{props.actions}</div>
      </header>
      {props.children}
    </section>
  );
}

function MetricCard(props: { label: string; value: string; tone: string }) {
  return (
    <div class={`metric metric-${props.tone}`}>
      <span>{props.label}</span>
      <strong>{props.value}</strong>
    </div>
  );
}

function ChatBubble(props: { message: ChatMessage }) {
  return (
    <article class={`message message-${props.message.role}`}>
      <strong>{props.message.role === "user" ? "You" : "Agent"}</strong>
      <p>{props.message.content}</p>
    </article>
  );
}

function ActivityRow(props: { entry: ActivityEvent }) {
  return (
    <div class={`activity activity-${props.entry.level}`}>
      <div>
        <strong>{props.entry.label}</strong>
        <p>{props.entry.detail}</p>
      </div>
      <span>{props.entry.at}</span>
    </div>
  );
}

function DeviceCard(props: { device: FleetSnapshot["devices"][number] }) {
  return (
    <article class="device">
      <div class="device-head">
        <div>
          <p class="device-kind">{props.device.kind}</p>
          <h3>{props.device.name}</h3>
        </div>
        <span>{props.device.driver}</span>
      </div>
      <p class="device-summary">{props.device.summary}</p>
      <pre>{JSON.stringify({ metadata: props.device.metadata, checks: props.device.checks }, null, 2)}</pre>
    </article>
  );
}
