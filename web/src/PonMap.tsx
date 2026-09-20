import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { formatPower, inventoryNotice, latestInventory } from "./api";
import { ErrorBox } from "./Layout";
import type { ONUSample } from "./types";

type Cell = {
  board: number;
  pon: number;
  total: number;
  online: number;
  minRx: number | null;
};

export function PonMap() {
  const navigate = useNavigate();
  const [rows, setRows] = useState<ONUSample[] | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState<unknown>(null);

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

  const byBoard = useMemo(() => {
    const map = new Map<string, Cell>();
    for (const row of rows ?? []) {
      const key = `${row.board}/${row.pon}`;
      const cell = map.get(key) ?? { board: row.board, pon: row.pon, total: 0, online: 0, minRx: null };
      cell.total += 1;
      if (row.status === "Online") cell.online += 1;
      if (row.rx_power !== null && (cell.minRx === null || row.rx_power < cell.minRx)) {
        cell.minRx = row.rx_power;
      }
      map.set(key, cell);
    }
    const groups = new Map<number, Cell[]>();
    for (const cell of map.values()) {
      const list = groups.get(cell.board) ?? [];
      list.push(cell);
      groups.set(cell.board, list);
    }
    return [...groups.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([board, cells]) => ({ board, cells: cells.sort((a, b) => a.pon - b.pon) }));
  }, [rows]);

  if (error) return <ErrorBox error={error} />;
  if (!rows) return <p className="empty">Loading PON occupancy…</p>;

  return (
    <>
      <h2>Boards / PON</h2>
      <p className="sub">{notice}</p>
      <p className="sub">Online / sampled in the last cycle. Weakest RX on that PON is shown when optics exist.</p>
      {byBoard.map((group) => (
        <div className="panel" key={group.board}>
          <h3>Slot {group.board}</h3>
          <div className="pon-grid">
            {group.cells.map((cell) => (
              <div
                key={`${cell.board}-${cell.pon}`}
                className={`pon-cell ${cell.online === 0 ? "off" : ""}`}
                onClick={() => navigate(`/onus?board=${cell.board}`)}
              >
                <div className="title">
                  PON {cell.pon}
                </div>
                <div className="ratio">
                  {cell.online}/{cell.total}
                </div>
                <div className="title">{formatPower(cell.minRx)}</div>
              </div>
            ))}
          </div>
        </div>
      ))}
    </>
  );
}
