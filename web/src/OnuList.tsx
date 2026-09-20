import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { formatSince, formatPower, inventoryNotice, latestInventory, rxClass } from "./api";
import { ErrorBox, Pill } from "./Layout";
import type { ONUSample } from "./types";

export function OnuList() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [rows, setRows] = useState<ONUSample[] | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState<unknown>(null);
  const q = params.get("q") ?? "";
  const status = params.get("status") ?? "";
  const board = params.get("board") ?? "";
  const link = params.get("eth") ?? "";

  useEffect(() => {
    let cancel = false;
    latestInventory()
      .then(({ run, rows: data }) => {
        if (!cancel) { setRows(data); setNotice(inventoryNotice(run, data)); }
      })
      .catch((err) => {
        if (!cancel) setError(err);
      });
    return () => {
      cancel = true;
    };
  }, []);

  const filtered = useMemo(() => {
    if (!rows) return [];
    const needle = q.trim().toLowerCase();
    return rows.filter((row) => {
      if (status && row.status !== status) return false;
      if (board && String(row.board) !== board) return false;
      if (link && (row.eth_link_state || "") !== link) return false;
      if (!needle) return true;
      return (
        row.serial_number.toLowerCase().includes(needle) ||
        row.name.toLowerCase().includes(needle) ||
        row.onu_type.toLowerCase().includes(needle)
      );
    });
  }, [rows, q, status, board, link]);

  const boards = [...new Set((rows ?? []).map((r) => r.board))].sort((a, b) => a - b);
  const statuses = [...new Set((rows ?? []).map((r) => r.status))].sort();

  function set(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  }

  if (error) return <ErrorBox error={error} />;
  if (!rows) return <p className="empty">Loading ONU inventory from the last cycle…</p>;

  return (
    <>
      <h2>ONUs</h2>
      <p className="sub">{notice}</p>
      <p className="sub">Latest poller snapshot · {filtered.length} of {rows.length} shown. Identity is serial.</p>
      <div className="toolbar">
        <input
          placeholder="Search serial, name, type"
          value={q}
          onChange={(e) => set("q", e.target.value)}
        />
        <select value={status} onChange={(e) => set("status", e.target.value)}>
          <option value="">All status</option>
          {statuses.map((s) => (
            <option key={s}>{s}</option>
          ))}
        </select>
        <select value={board} onChange={(e) => set("board", e.target.value)}>
          <option value="">All boards</option>
          {boards.map((b) => (
            <option key={b} value={b}>
              slot {b}
            </option>
          ))}
        </select>
        <select value={link} onChange={(e) => set("eth", e.target.value)}>
          <option value="">All UNI</option>
          <option value="up">UNI up</option>
          <option value="down">UNI down</option>
        </select>
      </div>
      <div className="panel">
        <table>
          <thead>
            <tr>
              <th>Serial</th>
              <th>Name</th>
              <th>Type</th>
              <th>Board/PON/ID</th>
              <th>Status</th>
              <th>Since</th>
              <th>RX</th>
              <th>TX</th>
              <th>UNI</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((row) => (
              <tr key={row.serial_number || `${row.board}-${row.pon}-${row.onu_id}`} className="clickable" onClick={() => navigate(`/onus/${encodeURIComponent(row.serial_number)}`)}>
                <td className="mono">{row.serial_number || "—"}</td>
                <td>{row.name}</td>
                <td>{row.onu_type}</td>
                <td className="mono">
                  {row.board}/{row.pon}/{row.onu_id}
                </td>
                <td>
                  <Pill value={row.status} />
                </td>
                <td className="mono" title={row.status_changed_at || ""}>
                  {formatSince(row.status_changed_at)}
                </td>
                <td className={rxClass(row.rx_power, row.status)}>{formatPower(row.rx_power)}</td>
                <td>{formatPower(row.tx_power)}</td>
                <td>
                  <Pill value={row.eth_link_state || row.eth_status || "—"} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {filtered.length === 0 && <div className="empty">No ONUs match the filters.</div>}
      </div>
    </>
  );
}
