import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./Layout";
import { OnuDetail } from "./OnuDetail";
import { OnuList } from "./OnuList";
import { Overview } from "./Overview";
import { PonMap } from "./PonMap";
import { Runs } from "./Runs";
import { Unauth } from "./Unauth";

export function App() {
  return (
    <HashRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Overview />} />
          <Route path="onus" element={<OnuList />} />
          <Route path="onus/:serial" element={<OnuDetail />} />
          <Route path="pons" element={<PonMap />} />
          <Route path="unauth" element={<Unauth />} />
          <Route path="runs" element={<Runs />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  );
}
