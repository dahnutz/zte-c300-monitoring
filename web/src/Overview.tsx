import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { discoveryCount, inventoryNotice, formatPower, formatTime, latestInventory, rxClass, unauth, version, type CollectorVersion } from "./api";
import { ErrorBox, Pill } from "./Layout";
import type { CountResult, ONUSample, UnauthList } from "./types";

export function Overview() {
  const navigate = useNavigate();
  const [status, setStatus] = useState<CountResult | null>(null);
  const [eth, setEth] = useState<CountResult | null>(null);
  const [rows, setRows] = useState<ONUSample[]>([]);
  const [ver, setVer] = useState<CollectorVersion | null>(null);
  const [found, setFound] = useState<UnauthList | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancel = false;
    (async () => {
      try {
        const [{ run, rows: list }, v, discovered] = await Promise.all([
          latestInventory(),
          version().catch(() => null),
          unauth().catch(() => null),
        ]);
        if (cancel) return;
        const tally = (key: (row: ONUSample) => string) => {
          const map = new Map<string, number>();
          for (const row of list) {
            const k = key(row) || "(empty)";
            map.set(k, (map.get(k) ?? 0) + 1);
          }
          return {
            group_by: "",
            run_id: run?.id,
            total: list.length,
            from: run?.started_at ?? list.at(-1)?.time ?? "",
            to: run?.finished_at ?? list[0]?.time ?? "",
            counts: [...map.entries()].map(([k, count]) => ({ key: k, count })).sort((a, b) => b.count - a.count),
          };
        };
        setStatus(tally((row) => row.status));
        setEth(tally((row) => row.eth_link_state || row.eth_status || ""));
        setRows(list);
        setNotice(inventoryNotice(run, list));
        setVer(v);
        setFound(discovered);
      } catch (err) {
        if (!cancel) setError(err);
      }
    })();
    return () => {
      cancel = true;
    };
  }, []);

  if (error) return <ErrorBox error={error} />;
  if (!status) return <p className="empty">Loading last poller cycle…</p>;

  const n = (key: string) => status.counts.find((c) => c.key === key)?.count ?? 0;
  const ethUp = eth?.counts.find((c) => c.key === "up")?.count ?? 0;
  const allWeak = rows
    .filter((r) => r.status === "Online" && r.rx_power !== null && r.rx_power <= -25)
    .sort((a, b) => (a.rx_power ?? 0) - (b.rx_power ?? 0))
    ;
  const weak = allWeak.slice(0, 15);
  const offline = rows.filter((r) => r.status !== "Online").slice(0, 20);

  return (
    <>
      <h2>Last finished poller cycle</h2>
      <p className="sub">{notice}</p>
      <p className="sub">
        Timescale snapshot {formatTime(status.from)} → {formatTime(status.to)}
        {status.run_id ? ` · run ${status.run_id}` : ""}
        {ver ? ` · collector ${ver.version}` : ""}. No live SNMP from this page.
      </p>
      <div className="cards">
        <div className="card">
          <div className="label">ONUs sampled</div>
          <div className="value">{status.total}</div>
        </div>
        <div className="card online">
          <div className="label">Online</div>
          <div className="value">{n("Online")}</div>
        </div>
        <div className="card offline">
          <div className="label">Offline / other</div>
          <div className="value">{status.total - n("Online")}</div>
          <div className="hint">
            Offline {n("Offline")}
            {n("LOS") ? ` · LOS ${n("LOS")}` : ""}
          </div>
        </div>
        <div className="card">
          <div className="label">UNI up</div>
          <div className="value">{ethUp}</div>
          <div className="hint">any Ethernet port up</div>
        </div>
        <div className="card warn">
          <div className="label">Weak RX</div>
          <div className="value">{allWeak.length}</div>
          <div className="hint">online ≤ −25 dBm (this page)</div>
        </div>
        <div className="card" onClick={() => navigate("/unauth")} style={{ cursor: "pointer" }}>
          <div className="label">Unconfigured</div>
          <div className="value">{discoveryCount(found)}</div>
          <div className="hint">{found?.status || "no discovery yet"}</div>
        </div>
      </div>

      <div className="grid-2">
        <div className="panel">
          <h3>Status</h3>
          <table>
            <tbody>
              {status.counts.map((c) => (
                <tr key={c.key}>
                  <td>
                    <Pill value={c.key} />
                  </td>
                  <td>{c.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="panel">
          <h3>Ethernet link (summary)</h3>
          <table>
            <tbody>
              {(eth?.counts ?? []).map((c) => (
                <tr key={c.key}>
                  <td>
                    <Pill value={c.key} />
                  </td>
                  <td>{c.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="grid-2">
        <div className="panel">
          <h3>Weakest online RX</h3>
          {weak.length === 0 ? (
            <div className="empty">No online ONU at or below −25 dBm in this cycle.</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Serial</th>
                  <th>Name</th>
                  <th>Pos</th>
                  <th>RX</th>
                </tr>
              </thead>
              <tbody>
                {weak.map((row) => (
                  <tr key={row.serial_number} className="clickable" onClick={() => navigate(`/onus/${row.serial_number}`)}>
                    <td className="mono">{row.serial_number}</td>
                    <td>{row.name}</td>
                    <td className="mono">
                      {row.board}/{row.pon}/{row.onu_id}
                    </td>
                    <td className={rxClass(row.rx_power, row.status)}>{formatPower(row.rx_power)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
        <div className="panel">
          <h3>Not online</h3>
          {offline.length === 0 ? (
            <div className="empty">All sampled ONUs were Online.</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Serial</th>
                  <th>Name</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {offline.map((row) => (
                  <tr key={row.serial_number} className="clickable" onClick={() => navigate(`/onus/${row.serial_number}`)}>
                    <td className="mono">{row.serial_number}</td>
                    <td>{row.name}</td>
                    <td>
                      <Pill value={row.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <p className="sub">
            <Link to="/onus">Full inventory</Link>
          </p>
        </div>
      </div>
    </>
  );
}
