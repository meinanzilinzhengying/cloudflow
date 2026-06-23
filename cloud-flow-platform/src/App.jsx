import { Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import Dashboard from "./pages/Dashboard";
import Topology from "./pages/Topology";
import AIAnalysis from "./pages/AIAnalysis";
import ExternalTools from "./pages/ExternalTools";
import ServiceHealth from "./pages/ServiceHealth";
import Alerts from "./pages/Alerts";
import Logs from "./pages/Logs";
import Config from "./pages/Config";

function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/topology" element={<Topology />} />
        <Route path="/services" element={<ServiceHealth />} />
        <Route path="/alerts" element={<Alerts />} />
        <Route path="/ai" element={<AIAnalysis />} />
        <Route path="/tools" element={<ExternalTools />} />
        <Route path="/logs" element={<Logs />} />
        <Route path="/config" element={<Config />} />
      </Routes>
    </Layout>
  );
}

export default App;
