import { useEffect, useState } from "react";
import { discoveryCount, formatTime, unauth } from "./api";
import { ErrorBox, Pill } from "./Layout";
import type { UnauthList } from "./types";

export function Unauth() {
  const [list, setList] = useState<UnauthList | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancel = false;
    unauth()
      .then((data) => {
        if (!cancel) setList(data);
      })
      .catch((err) => {
        if (!cancel) setError(err);
      });
    return () => {
      cancel = true;
    };
  }, []);

  if (error) return <ErrorBox error={error} />;
  if (!list) return <p className="empty">Loading unconfigured-ONU discovery…</p>;

  return (
    <>
      <h2>Unconfigured ONUs</h2>
      <p className="sub">
        OLT-seen serials that are not in the provisioned inventory.
        Discovery {list.status} · {formatTime(list.observed_at)}
        {list.oid ? ` · ${list.oid}` : ""}.
      </p>
      {list.message && <p className="sub">{list.message}</p>}
      <div className="cards">
        <div className="card">
          <div className="label">Current</div>
          <div className="value">{discoveryCount(list)}</div>
        </div>
        <div className="card">
          <div className="label">Walk status</div>
          <div className="value"><Pill value={list.status} /></div>
        </div>
      </div>
      {list.status !== "ok" && list.status !== "empty" && list.onus.length > 0 && (
        <p className="sub">Last-known rows below are stale; the latest discovery did not confirm them.</p>
      )}
      <div className="panel">
        <table>
          <thead>
            <tr>
              <th>Serial</th>
              <th>Board / PON</th>
              <th>Type</th>
              <th>First seen</th>
              <th>Last seen</th>
            </tr>
          </thead>
          <tbody>
            {list.onus.map((onu) => (
              <tr key={onu.serial_number}>
                <td className="mono">{onu.serial_number}</td>
                <td className="mono">{onu.board || "—"}/{onu.pon || "—"}</td>
                <td>{onu.onu_type || "—"}</td>
                <td className="mono">{formatTime(onu.first_seen_at)}</td>
                <td className="mono">{formatTime(onu.last_seen_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.onus.length === 0 && (
          <div className="empty">
            {list.status !== "ok" && list.status !== "empty"
              ? "Discovery is not available or confirmed. Empty is not a verified zero."
              : "No unconfigured ONUs in the last discovery."}
          </div>
        )}
      </div>
    </>
  );
}
