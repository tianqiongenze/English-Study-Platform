import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import Home from "./pages/Home";
import Speech from "./pages/Speech";
import Writing from "./pages/Writing";
import Learning from "./pages/Learning";
import "./styles.css";

function Require({ children }: { children: React.JSX.Element }) {
  const user = useAuth((s) => s.user);
  return user ? children : <Navigate to="/login" replace />;
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/" element={<Require><Home /></Require>}>
          <Route index element={<Navigate to="/speech" replace />} />
          <Route path="speech" element={<Speech />} />
          <Route path="writing" element={<Writing />} />
          <Route path="learning" element={<Learning />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  </React.StrictMode>
);
