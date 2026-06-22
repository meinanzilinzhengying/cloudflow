import { Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import Dashboard from "./pages/Dashboard";
import ProbeManage from "./pages/ProbeManage";
import AIAnalysis from "./pages/AIAnalysis";
import ExternalTools from "./pages/ExternalTools";
import ServiceHealth from "./pages/ServiceHealth";
import Alerts from "./pages/Alerts";

function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/probes" element={<ProbeManage />} />
        <Route path="/ai" element={<AIAnalysis />} />
        <Route path="/tools" element={<ExternalTools />} />
        <Route path="/services" element={<ServiceHealth />} />
        <Route path="/alerts" element={<Alerts />} />
      </Routes>
    </Layout>
  );
}

export default App;
