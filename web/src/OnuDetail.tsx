import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ethEvents, formatPower, formatSince, formatTime, liveOnu, samples, statusEvents } from "./api";
import { ErrorBox, Pill } from "./Layout";
import { LineChart } from "./LineChart";
import type { EthEvent, EthPort, LiveOnu, ONUSample, StatusEvent } from "./types";

export function OnuDetail() {
  const { serial = "" } = useParams();
  return <OnuDetailView key={serial} serial={serial} />;
}

function OnuDetailView({ serial }: { serial: string }) {
  const request = useRef<AbortController | null>(null);
  const portRequest = useRef(0);
  useEffect(() => () => { request.current?.abort(); portRequest.current++; }, []);
  const [portError, setPortError] = useState("");
  const [rows, setRows] = useState<ONUSample[] | null>(null);
  const [events, setEvents] = useState<StatusEvent[]>([]);
  const [live, setLive] = useState<LiveOnu | null>(null);
  const [liveError, setLiveError] = useState<string>("");
  const [liveBusy, setLiveBusy] = useState(false);
  const [port, setPort] = useState<number | null>(null);
  const [flaps, setFlaps] = useState<EthEvent[]>([]);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancel = false;
    Promise.all([samples({ serial, limit: 2000 }), statusEvents({ serial, limit: 20 })])
      .then(([data, ev]) => {
        if (cancel) return;
        setRows(data.slice().reverse());
        setEvents(ev);
      })
      .catch((err) => {
        if (!cancel) setError(err);
      });
    return () => {
      cancel = true;
    };
  }, [serial]);

  if (error) return <ErrorBox error={error} />;
  if (!rows) return <p className="empty">Loading optical history…</p>;
  if (rows.length === 0) {
    return (
      <>
        <h2>{serial}</h2>
        <p className="empty">No stored samples for this serial in the default window (last 24 hours, max 2000 points).</p>
        <Link to="/onus">Back to inventory</Link>
      </>
    );
  }

  const latest = rows[rows.length - 1];
  const rx = rows.filter((r) => r.rx_power !== null).map((r) => ({ t: Date.parse(r.time), v: r.rx_power as number }));
  const tx = rows.filter((r) => r.tx_power !== null).map((r) => ({ t: Date.parse(r.time), v: r.tx_power as number }));
  const last10 = rows.slice().reverse().slice(0, 10);
  const ports = displayPorts(latest);
  const since = latest.status_changed_at;

  async function refreshNow() {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLive(null);
    setLiveBusy(true);
    setLiveError("");
    try {
      const result = await liveOnu(latest.board, latest.pon, latest.onu_id, serial, controller.signal);
      if (!controller.signal.aborted) setLive(result);
    } catch (err) {
      if (!controller.signal.aborted) setLiveError(err instanceof Error ? err.message : String(err));
    } finally {
      if (!controller.signal.aborted) setLiveBusy(false);
    }
  }

  async function openPort(n: number) {
    setPort(n);
    setFlaps([]);
    setPortError("");
    const sequence = ++portRequest.current;
    try {
      const result = await ethEvents({ serial, port: n, limit: 30 });
      if (sequence === portRequest.current) setFlaps(result);
    } catch (err) {
      if (sequence === portRequest.current) setPortError(String(err));
    }
  }

  return (
    <>
      <h2>{latest.name || serial}</h2>
      <p className="sub">
        <span className="mono">{serial}</span> · last sample {formatTime(latest.time)} · {rows.length} stored points
        (API cap 2000).
      </p>
      <div className="toolbar">
        <button type="button" onClick={refreshNow} disabled={liveBusy}>
          {liveBusy ? "Querying OLT…" : "Query now"}
        </button>
        <span className="hint">On-demand SNMP reads for this ONU. History below is unchanged.</span>
      </div>
      {liveError && <ErrorBox error={liveError} />}
      {live && (
        <div className="live-box">
          <h3>On-demand read · collected {formatTime(live.observed_at)}</h3>
          <p className="hint">Collection time is not the device sensor timestamp. These values do not auto-refresh.</p>
          <div className="meta">
            <div>
              <div className="k">Status</div>
              <div className="v"><Pill value={live.status} /></div>
            </div>
            <div>
              <div className="k">RX / TX</div>
              <div className="v">{live.status === "Online" ? live.rx_power || "—" : "—"} / {live.status === "Online" ? live.tx_power || "—" : "—"}</div>
            </div>
            <div>
              <div className="k">UNI</div>
              <div className="v">{live.ethernet.ports.map((p) => `${p.port_index}:${p.link_state}`).join(" ") || live.ethernet.status}</div>
            </div>
          </div>
        </div>
      )}
      <div className="meta">
        <div>
          <div className="k">Position</div>
          <div className="v mono">
            {latest.board}/{latest.pon}/{latest.onu_id}
          </div>
        </div>
        <div>
          <div className="k">Type</div>
          <div className="v">{latest.onu_type || "—"}</div>
        </div>
        <div>
          <div className="k">Status</div>
          <div className="v">
            <Pill value={latest.status} />
            <div className="hint">
              since {formatTime(since)} ({formatSince(since)})
              {latest.previous_status ? ` · was ${latest.previous_status}` : ""}
            </div>
          </div>
        </div>
        <div>
          <div className="k">RX / TX</div>
          <div className="v">
            {formatPower(latest.rx_power)} / {formatPower(latest.tx_power)}
          </div>
        </div>
      </div>

      <div className="panel" style={{ marginTop: 16 }}>
        <h3>Ethernet UNI (stored sample)</h3>
        {portError && <ErrorBox error={portError} />}
        <div className="eth-ports">
          {ports.map((p) => (
            <button key={p.port} type="button" className={`eth-port ${port === p.port ? "active" : ""}`} onClick={() => openPort(p.port)}>
              <div className="title">eth {p.port}</div>
              <Pill value={p.link || "unknown"} />
              <div className="hint">{p.admin || "unknown"}{p.speed_mbps ? ` · ${p.speed_mbps}M` : ""}</div>
            </button>
          ))}
          {ports.length === 0 && <div className="empty">No UNI rows in the last sample.</div>}
        </div>
        {port !== null && (
          <>
            <h3 style={{ marginTop: 16 }}>eth {port} flaps</h3>
            <table>
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Link</th>
                  <th>Was</th>
                  <th>Admin</th>
                </tr>
              </thead>
              <tbody>
                {flaps.map((ev) => (
                  <tr key={ev.time + ev.port + ev.link_state}>
                    <td className="mono">{formatTime(ev.time)}</td>
                    <td><Pill value={ev.link_state} /></td>
                    <td>{ev.previous_link || "—"}</td>
                    <td>{ev.admin_state || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {flaps.length === 0 && <div className="empty">No stored UNI changes for this port.</div>}
          </>
        )}
      </div>

      <div className="panel">
        <h3>Optical history (dBm)</h3>
        <LineChart
          series={[
            { key: "RX", color: "#6cb3ff", points: rx },
            { key: "TX", color: "#3dba7c", points: tx },
          ]}
        />
        <div className="legend">
          <span>
            <i className="swatch" style={{ background: "#6cb3ff" }} />
            RX downstream
          </span>
          <span>
            <i className="swatch" style={{ background: "#3dba7c" }} />
            TX upstream
          </span>
        </div>
        <h3 style={{ marginTop: 16 }}>Last 10 samples</h3>
        <table>
          <thead>
            <tr>
              <th>Time</th>
              <th>RX</th>
              <th>TX</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {last10.map((row) => (
              <tr key={row.time}>
                <td className="mono">{formatTime(row.time)}</td>
                <td>{formatPower(row.rx_power)}</td>
                <td>{formatPower(row.tx_power)}</td>
                <td><Pill value={row.status} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {events.length > 0 && (
        <div className="panel">
          <h3>Status changes</h3>
          <table>
            <thead>
              <tr>
                <th>Time</th>
                <th>Status</th>
                <th>Was</th>
              </tr>
            </thead>
            <tbody>
              {events.map((ev) => (
                <tr key={ev.time + ev.status}>
                  <td className="mono">{formatTime(ev.time)}</td>
                  <td><Pill value={ev.status} /></td>
                  <td>{ev.previous_status || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="sub">
        <Link to="/onus">← Inventory</Link>
      </p>
    </>
  );
}

function displayPorts(row: ONUSample): EthPort[] {
  if (row.eth_ports && row.eth_ports.length > 0) {
    return row.eth_ports;
  }
  return [];
}
