import { NavLink, Outlet } from "react-router-dom";

const links = [
  { to: "/", label: "Overview", end: true },
  { to: "/onus", label: "ONUs" },
  { to: "/pons", label: "Boards / PON" },
  { to: "/unauth", label: "Unconfigured" },
  { to: "/runs", label: "Poller runs" },
];

export function Layout() {
  return (
    <div className="shell">
      <nav className="side">
        <div className="brand">
          <h1>PON Monitor</h1>
          <p>ONU observations</p>
        </div>
        <div>
          {links.map((link) => (
            <NavLink key={link.to} to={link.to} end={link.end} className={({ isActive }) => (isActive ? "active" : "")}>
              {link.label}
            </NavLink>
          ))}
        </div>
        <div className="soon">
          Showing stored observations. Check the collection time before
          treating a device state as current.
        </div>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  );
}

export function Pill({ value }: { value?: string }) {
  const text = value || "unknown";
  const cls = text.replace(/\s+/g, "");
  return <span className={`pill ${cls}`}>{text}</span>;
}

export function ErrorBox({ error }: { error: unknown }) {
  return <div className="error">{error instanceof Error ? error.message : String(error)}</div>;
}
