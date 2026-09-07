import { NavLink, Route, Routes } from "react-router-dom";
import IncidentList from "./pages/IncidentList";
import IncidentDetail from "./pages/IncidentDetail";
import Awaiting from "./pages/Awaiting";
import Knowledge from "./pages/Knowledge";
import Integrations from "./pages/Integrations";

export default function App() {
  return (
    <div className="shell">
      <header className="top">
        <div className="brand">
          <span className="mark">OpsPilot</span>
          <span className="tag">incident command</span>
        </div>
        <nav>
          <NavLink to="/" end>
            Incidents
          </NavLink>
          <NavLink to="/awaiting">Approvals</NavLink>
          <NavLink to="/knowledge">Knowledge</NavLink>
          <NavLink to="/integrations">Integrations</NavLink>
        </nav>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<IncidentList />} />
          <Route path="/incidents/:id" element={<IncidentDetail />} />
          <Route path="/awaiting" element={<Awaiting />} />
          <Route path="/knowledge" element={<Knowledge />} />
          <Route path="/integrations" element={<Integrations />} />
        </Routes>
      </main>
    </div>
  );
}
