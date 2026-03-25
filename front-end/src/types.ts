export type ApiEnvelope<T> = {
  status: number;
  ok: boolean;
  durationMs: number;
  data: T;
};

export type UIConfig = {
  title: string;
  description: string;
  default_prompt: string;
};

export type HealthStatus = {
  ok: boolean;
  status_code: number;
  body: string;
};

export type CheckResult = {
  name: string;
  output: string;
};

export type DeviceSnapshot = {
  name: string;
  kind: string;
  driver: string;
  supports_capture: boolean;
  summary: string;
  checks?: CheckResult[];
  metadata?: Record<string, string>;
};

export type FleetSnapshot = {
  primary_capture_device: string;
  devices: DeviceSnapshot[];
};

export type CaptureResult = {
  ok: boolean;
  output?: string;
  width?: number;
  height?: number;
  camera_sn?: string;
  error?: string;
  raw_output?: string;
};

export type AnalyzeResponse = {
  result?: string;
  capture?: CaptureResult;
  peripherals?: FleetSnapshot;
  error?: string;
};

export type AgentCapability = {
  id: string;
  name: string;
  description: string;
};

export type AgentCapabilities = {
  name: string;
  description: string;
  capabilities: AgentCapability[];
};

export type AgentChatRequest = {
  message: string;
  history: Array<{ role: string; content: string }>;
  capture_fresh?: boolean;
  use_latest_image?: boolean;
  include_snapshot?: boolean;
};

export type AgentChatResponse = {
  reply?: string;
  capture?: CaptureResult;
  peripherals?: FleetSnapshot;
  error?: string;
};

export type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
};

export type ActivityLevel = "info" | "success" | "error";

export type ActivityEvent = {
  id: string;
  label: string;
  detail: string;
  level: ActivityLevel;
  at: string;
};
