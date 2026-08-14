import React, { Suspense, lazy } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import ErrorBoundary from "./ErrorBoundary";
import { SkeletonPage } from "./Skeleton";
import "./styles.css";

// 路由懒加载(React.lazy):非首屏页面级组件按需分包。
// chunk 命名由 Vite 自动按模块名处理,无需 /* webpackChunkName */ 注释。
const Home = lazy(() => import("./pages/Home"));
const Speech = lazy(() => import("./pages/Speech"));
const Writing = lazy(() => import("./pages/Writing"));
const Learning = lazy(() => import("./pages/Learning"));
const Profile = lazy(() => import("./pages/Profile"));

/** 懒加载路由包裹器:加载期间展示页面级骨架屏 */
function Lazy({ children }: { children: React.JSX.Element }) {
  return <Suspense fallback={<SkeletonPage />}>{children}</Suspense>;
}

function Require({ children }: { children: React.JSX.Element }) {
  const user = useAuth((s) => s.user);
  return user ? children : <Navigate to="/login" replace />;
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <BrowserRouter basename="/h5">
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<Require><Lazy><Home /></Lazy></Require>}>
            <Route index element={<Navigate to="/speech" replace />} />
            <Route path="speech" element={<Lazy><Speech /></Lazy>} />
            <Route path="writing" element={<Lazy><Writing /></Lazy>} />
            <Route path="learning" element={<Lazy><Learning /></Lazy>} />
            <Route path="profile" element={<Lazy><Profile /></Lazy>} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </ErrorBoundary>
  </React.StrictMode>
);
