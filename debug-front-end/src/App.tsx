import { useEffect, useState } from "preact/hooks";
import { latestPreviewURL, loadHealth, loadPrimaryStatus, loadUIConfig, openPeripheralStream } from "./api";
import type { DeviceSnapshot, FleetSnapshot, HealthStatus, UIConfig } from "./types";

export function App() {
  const [ui, setUI] = useState<UIConfig | null>(null);
  const [health, setHealth] = useState<HealthStatus | null>(null);
  const [primary, setPrimary] = useState<DeviceSnapshot | null>(null);
  const [fleet, setFleet] = useState<FleetSnapshot | null>(null);
  const [previewURL, setPreviewURL] = useState("");
  const [lastUpdate, setLastUpdate] = useState("--");

  useEffect(() => {
    void loadUIConfig().then((config) => {
      setUI(config);
      document.title = `${config.title} Debug`;
    });

    void refreshSideData();
    setPreviewURL(latestPreviewURL());

    const source = openPeripheralStream((snapshot) => {
      setFleet(snapshot);
      setLastUpdate(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
      setPreviewURL(latestPreviewURL());
    });

    const timer = window.setInterval(() => {
      void refreshSideData();
      setPreviewURL(latestPreviewURL());
    }, 3000);

    return () => {
      source.close();
      window.clearInterval(timer);
    };
  }, []);

  async function refreshSideData() {
    const [nextHealth, nextPrimary] = await Promise.all([
      loadHealth().catch(() => null),
      loadPrimaryStatus().catch(() => null)
    ]);
    setHealth(nextHealth);
    setPrimary(nextPrimary);
  }

  return (
    <main class="debug-shell">
      <section class="hero panel">
        <div>
          <p class="eyebrow">Debug</p>
          <h1>{ui?.title ?? "Jetson Peripheral Debug"}</h1>
          <p class="lede">外设状态</p>
        </div>
        <div class="hero-metrics">
          <Metric title="Last update" value={lastUpdate} />
          <Metric title="Primary device" value={fleet?.primary_capture_device ?? primary?.name ?? "--"} />
          <Metric title="Health" value={health?.ok ? "ready" : "degraded"} />
        </div>
      </section>

      <section class="debug-grid">
        <section class="panel">
          <header class="panel-head">
            <div>
              <h2>Live Frame</h2>
              <p>最新图像</p>
            </div>
          </header>
          <div class="preview-card">
            <img class="preview" src={previewURL} alt="Latest capture preview" />
          </div>
        </section>

        <section class="panel">
          <header class="panel-head">
            <div>
              <h2>Health Snapshot</h2>
              <p>健康状态</p>
            </div>
          </header>
          <pre>{JSON.stringify({ health, primary }, null, 2)}</pre>
        </section>
      </section>

      <section class="panel">
        <header class="panel-head">
          <div>
            <h2>Peripheral Feed</h2>
            <p>TODO: 注册更多外设</p>
          </div>
        </header>
        <div class="device-grid">
          {fleet?.devices?.length ? fleet.devices.map((device) => <DeviceCard key={device.name} device={device} />) : <div class="empty">Waiting…</div>}
        </div>
      </section>
    </main>
  );
}

function Metric(props: { title: string; value: string }) {
  return (
    <div class="metric">
      <span>{props.title}</span>
      <strong>{props.value}</strong>
    </div>
  );
}

function DeviceCard(props: { device: DeviceSnapshot }) {
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
