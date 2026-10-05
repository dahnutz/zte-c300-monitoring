import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { duplicateSerials, formatTime } from "./api";
import { ErrorBox, Pill } from "./Layout";
import type { DuplicateSerialList } from "./types";

export function Duplicates() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const scope = params.get("scope") === "history" ? "history" : "current";
  const [list, setList] = useState<DuplicateSerialList | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancel = false;
    duplicateSerials(scope)
      .then((data) => {
        if (!cancel) setList(data);
      })
      .catch((err) => {
        if (!cancel) setError(err);
      });
    return () => {
      cancel = true;
    };
  }, [scope]);

  function setScope(next: "current" | "history") {
    const q = new URLSearchParams(params);
    if (next === "current") q.delete("scope");
    else q.set("scope", next);
    setList(null);
    setParams(q, { replace: true });
  }

  if (error) return <ErrorBox error={error} />;
  if (!list) return <p className="empty">Loading duplicate serials from Timescale…</p>;

  return (
    <>
      <h2>Duplicate serials</h2>
      <p className="sub">
        Same SN seen at more than one board/PON/ONU-ID. Timescale only — no SNMP.
        Use this to find migrations that still leave a stale provisioned ONU, then
        delete the leftover on the OLT.
      </p>
      <p className="sub">
        {list.run_id ? `Latest finished run ${list.run_id} · ${formatTime(list.from)} → ${formatTime(list.to)}. ` : "No finished collection run yet. "}
        This page does not delete OLT config or history rows.
      </p>
      <div className="toolbar">
        <select value={scope} onChange={(e) => setScope(e.target.value === "history" ? "history" : "current")}>
          <option value="current">In the last cycle</option>
          <option value="history">Moved in history</option>
        </select>
      </div>
      <div className="cards">
        <div className="card warn">
          <div className="label">{scope === "current" ? "Serials on more than one port now" : "Serials that changed port"}</div>
          <div className="value">{list.count}</div>
        </div>
      </div>
      <div className="panel">
        <table>
          <thead>
            <tr>
              <th>Serial</th>
              <th>Name</th>
              <th>Ports</th>
              <th>Now</th>
              <th>Positions</th>
            </tr>
          </thead>
          <tbody>
            {list.serials.map((row) => (
              <tr key={row.serial_number} className="clickable" onClick={() => navigate(`/onus/${encodeURIComponent(row.serial_number)}`)}>
                <td className="mono">{row.serial_number}</td>
                <td>{row.name || "—"}</td>
                <td>{row.position_count}</td>
                <td>{row.current_count}</td>
                <td>
                  {row.positions.map((pos) => (
                    <div key={`${pos.board}/${pos.pon}/${pos.onu_id}`}>
                      <span className="mono">{pos.board}/{pos.pon}/{pos.onu_id}</span>
                      {" "}
                      <Pill value={pos.status || "unknown"} />
                      {pos.in_latest_run ? " · last cycle" : " · older"}
                      {" · "}
                      {formatTime(pos.last_seen)}
                    </div>
                  ))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.serials.length === 0 && (
          <div className="empty">
            {scope === "current"
              ? "No serial appears on more than one port in the last finished cycle."
              : "No serial has been observed at more than one port in stored history."}
          </div>
        )}
      </div>
    </>
  );
}
