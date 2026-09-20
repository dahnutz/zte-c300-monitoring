import { useEffect, useState } from "react";
import { formatTime, runs } from "./api";
import { ErrorBox, Pill } from "./Layout";
import { LineChart } from "./LineChart";
import type { CollectionRun } from "./types";

export function Runs() {
  const [rows, setRows] = useState<CollectionRun[] | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancel = false;
    runs(50)
      .then((data) => {
        if (!cancel) setRows(data);
      })
      .catch((err) => {
        if (!cancel) setError(err);
      });
    return () => {
      cancel = true;
    };
  }, []);

  if (error) return <ErrorBox error={error} />;
  if (!rows) return <p className="empty">Loading collection runs…</p>;

  const duration = rows
    .filter((r) => r.duration_ms)
    .map((r) => ({ t: Date.parse(r.started_at), v: (r.duration_ms as number) / 1000 }))
    .reverse();

  return (
    <>
      <h2>Poller runs</h2>
      <p className="sub">Cost of each sequential PON-list cycle. This is collector work, not per-ONU SNMP detail.</p>
      <div className="panel">
        <h3>Duration (seconds)</h3>
        <LineChart series={[{ key: "duration", color: "#e2a03f", points: duration }]} />
      </div>
      <div className="panel">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Started</th>
              <th>Status</th>
              <th>PONs ok</th>
              <th>PONs err</th>
              <th>ONUs</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((run) => (
              <tr key={run.id}>
                <td className="mono">{run.id}</td>
                <td>{formatTime(run.started_at)}</td>
                <td>
                  <Pill value={run.status} />
                </td>
                <td>{run.pons_ok}</td>
                <td>{run.pons_error}</td>
                <td>{run.onus_sampled}</td>
                <td>{run.duration_ms ? `${(run.duration_ms / 1000).toFixed(1)} s` : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
