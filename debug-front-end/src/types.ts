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
