import type {
  AgentCapabilities,
  AgentChatRequest,
  AgentChatResponse,
  AnalyzeResponse,
  ApiEnvelope,
  CaptureResult,
  DeviceSnapshot,
  FleetSnapshot,
  HealthStatus,
  UIConfig
} from "./types";

async function parseJSON<T>(response: Response): Promise<T> {
  const text = await response.text();
  if (!text) {
    return null as T;
  }
  return JSON.parse(text) as T;
}

async function requestJSON<T>(url: string, options?: RequestInit): Promise<ApiEnvelope<T>> {
  const startedAt = performance.now();
  const response = await fetch(url, options);
  const data = await parseJSON<T>(response);
  return {
    status: response.status,
    ok: response.ok,
    durationMs: Number((performance.now() - startedAt).toFixed(1)),
    data
  };
}

export function loadUIConfig() {
  return requestJSON<UIConfig>("/api/config");
}

export function loadHealth() {
  return requestJSON<HealthStatus>("/api/health");
}

export function loadAgentCapabilities() {
  return requestJSON<AgentCapabilities>("/api/agent/capabilities");
}

export function loadPeripherals() {
  return requestJSON<FleetSnapshot>("/api/peripherals");
}

export function loadPrimaryStatus() {
  return requestJSON<DeviceSnapshot>("/api/camera/status");
}

export function captureFrame() {
  return requestJSON<CaptureResult>("/api/camera/capture");
}

export function analyzeFrame(prompt: string) {
  return requestJSON<AnalyzeResponse>("/api/capture-and-analyze", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt })
  });
}

export function sendAgentChat(payload: AgentChatRequest) {
  return requestJSON<AgentChatResponse>("/api/agent/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  });
}

export async function streamAgentChat(
  payload: AgentChatRequest,
  onEvent: (event: { event: string; data: any }) => void
) {
  const response = await fetch("/api/agent/chat/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  });

  if (!response.body) {
    throw new Error("stream response body is empty");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split("\n\n");
    buffer = frames.pop() || "";

    for (const frame of frames) {
      const lines = frame.split("\n");
      let event = "message";
      let data = "";
      for (const line of lines) {
        if (line.startsWith("event: ")) {
          event = line.slice(7);
        } else if (line.startsWith("data: ")) {
          data += line.slice(6);
        }
      }
      if (data) {
        onEvent({ event, data: JSON.parse(data) });
      }
    }
  }
}

export function latestPreviewURL() {
  return `/api/camera/latest.jpg?t=${Date.now()}`;
}
