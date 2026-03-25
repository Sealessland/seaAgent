async function parseJSONResponse(response) {
  const text = await response.text();
  if (!text) {
    return null;
  }

  try {
    return JSON.parse(text);
  } catch (err) {
    throw new Error(`invalid JSON response: ${err.message}`);
  }
}

export async function requestJSON(url, options) {
  const response = await fetch(url, options);
  const data = await parseJSONResponse(response);
  return {
    status: response.status,
    ok: response.ok,
    data,
  };
}

export function checkHealth() {
  return requestJSON("/api/health");
}

export async function loadUIConfig() {
  const result = await requestJSON("/api/config");
  return result.data;
}

export function checkCameraStatus() {
  return requestJSON("/api/camera/status");
}

export function captureFrame() {
  return requestJSON("/api/camera/capture");
}

export function captureAndAnalyze(prompt) {
  return requestJSON("/api/capture-and-analyze", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt }),
  });
}

export function latestPreviewURL() {
  return `/api/camera/latest.jpg?t=${Date.now()}`;
}
