type Series = {
  key: string;
  color: string;
  points: { t: number; v: number }[];
};

export function LineChart({ series, height = 260 }: { series: Series[]; height?: number }) {
  const width = 900;
  const pad = { l: 48, r: 12, t: 12, b: 28 };
  const all = series.flatMap((s) => s.points);
  if (all.length < 2) {
    return <div className="empty">Not enough samples to graph yet.</div>;
  }
  const tMin = Math.min(...all.map((p) => p.t));
  const tMax = Math.max(...all.map((p) => p.t));
  const vMin = Math.min(...all.map((p) => p.v));
  const vMax = Math.max(...all.map((p) => p.v));
  const vPad = Math.max(0.5, (vMax - vMin) * 0.12);
  const yMin = vMin - vPad;
  const yMax = vMax + vPad;
  const x = (t: number) => pad.l + ((t - tMin) / Math.max(1, tMax - tMin)) * (width - pad.l - pad.r);
  const y = (v: number) => pad.t + ((yMax - v) / Math.max(0.01, yMax - yMin)) * (height - pad.t - pad.b);
  const ticks = 4;
  const yTicks = Array.from({ length: ticks + 1 }, (_, i) => yMin + ((yMax - yMin) * i) / ticks);

  return (
    <svg className="chart" viewBox={`0 0 ${width} ${height}`} role="img">
      {yTicks.map((tick) => (
        <g key={tick}>
          <line x1={pad.l} x2={width - pad.r} y1={y(tick)} y2={y(tick)} stroke="#2a3848" />
          <text x={pad.l - 6} y={y(tick) + 4} textAnchor="end" fill="#8b9aab" fontSize="11">
            {tick.toFixed(1)}
          </text>
        </g>
      ))}
      {series.map((s) => {
        const d = s.points.map((p, i) => `${i === 0 ? "M" : "L"} ${x(p.t)} ${y(p.v)}`).join(" ");
        return <path key={s.key} d={d} fill="none" stroke={s.color} strokeWidth="2" />;
      })}
      <text x={pad.l} y={height - 6} fill="#8b9aab" fontSize="11">
        {new Date(tMin).toISOString().slice(11, 16)} UTC
      </text>
      <text x={width - pad.r} y={height - 6} textAnchor="end" fill="#8b9aab" fontSize="11">
        {new Date(tMax).toISOString().slice(11, 16)} UTC
      </text>
    </svg>
  );
}
