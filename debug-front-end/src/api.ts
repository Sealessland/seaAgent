import type { FleetSnapshot, HealthStatus, UIConfig, DeviceSnapshot } from "./types";

async function requestJSON<T>(url: string): Promise<T> {
  const response = await fetch(url);
  return response.json() as Promise<T>;
}

export function loadUIConfig() {
  return requestJSON<UIConfig>("/api/config");
}

export function loadHealth() {
  return requestJSON<HealthStatus>("/api/health");
}

export function loadPrimaryStatus() {
  return requestJSON<DeviceSnapshot>("/api/camera/status");
}

export function latestPreviewURL() {
  return `/api/camera/latest.jpg?t=${Date.now()}`;
}

export function openPeripheralStream(onMessage: (snapshot: FleetSnapshot) => void) {
  const source = new EventSource("/api/peripherals/stream");
  source.onmessage = (event) => {
    onMessage(JSON.parse(event.data) as FleetSnapshot);
  };
  return source;
}
