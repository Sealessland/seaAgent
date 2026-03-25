import {
  captureAndAnalyze,
  captureFrame,
  checkCameraStatus,
  checkHealth,
  latestPreviewURL,
  loadUIConfig,
} from "./api.js";

const titleEl = document.getElementById("app-title");
const descriptionEl = document.getElementById("app-description");
const statusEl = document.getElementById("status-output");
const analyzeEl = document.getElementById("analyze-output");
const promptEl = document.getElementById("prompt");
const previewEl = document.getElementById("preview");

function renderResult(target, result) {
  target.textContent = JSON.stringify(result, null, 2);
}

function renderError(target, err) {
  target.textContent = err instanceof Error ? err.message : String(err);
}

function refreshPreview() {
  previewEl.src = latestPreviewURL();
}

async function initUI() {
  try {
    const config = await loadUIConfig();
    document.title = config.title;
    titleEl.textContent = config.title;
    descriptionEl.textContent = config.description;
    promptEl.value = config.default_prompt;
  } catch (err) {
    renderError(statusEl, err);
  }
}

async function runAction(target, pendingText, action, afterSuccess) {
  target.textContent = pendingText;
  try {
    const result = await action();
    renderResult(target, result);
    if (afterSuccess) {
      afterSuccess(result);
    }
  } catch (err) {
    renderError(target, err);
  }
}

document.getElementById("health").addEventListener("click", () => {
  runAction(statusEl, "检查 vLLM 中…", checkHealth);
});

document.getElementById("status").addEventListener("click", () => {
  runAction(statusEl, "检查摄像头状态中…", checkCameraStatus);
});

document.getElementById("capture").addEventListener("click", () => {
  runAction(statusEl, "抓帧中…", captureFrame, (result) => {
    if (result.ok) {
      refreshPreview();
    }
  });
});

document.getElementById("analyze").addEventListener("click", () => {
  runAction(
    analyzeEl,
    "抓帧并调用 agent 中…",
    () => captureAndAnalyze(promptEl.value),
    (result) => {
      if (result.data && result.data.capture && result.data.capture.ok) {
        refreshPreview();
      }
    },
  );
});

initUI();
