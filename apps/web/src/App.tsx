import React, { Suspense, lazy } from "react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import LearningLayout from "./pages/LearningLayout";
import TeacherLayout from "./pages/TeacherLayout";
import { SkeletonPage } from "./Skeleton";

// 路由懒加载(React.lazy):非首屏页面级组件按需分包。
// chunk 命名由 Vite 自动按模块名处理,无需 /* webpackChunkName */ 注释。
const SpeechPanel = lazy(() => import("./panels/SpeechPanel"));
const WritingPanel = lazy(() => import("./panels/WritingPanel"));
const LearningPanel = lazy(() => import("./panels/LearningPanel"));
const LessonPlan = lazy(() => import("./pages/teacher/LessonPlan"));
const Analytics = lazy(() => import("./pages/teacher/Analytics"));
const Grading = lazy(() => import("./pages/teacher/Grading"));
const Profile = lazy(() => import("./pages/Profile"));
const AdminDashboard = lazy(() => import("./pages/admin/AdminDashboard"));
const GovernanceDashboard = lazy(() => import("./pages/admin/GovernanceDashboard"));
const ContentLibrary = lazy(() => import("./pages/learning/ContentLibrary"));
const ContentAdmin = lazy(() => import("./pages/admin/ContentAdmin"));
const CommunityPage = lazy(() => import("./pages/community/CommunityPage"));
const RankingPage = lazy(() => import("./pages/ranking/RankingPage"));

/** 懒加载路由包裹器:加载期间展示页面级骨架屏 */
function Lazy({ children }: { children: React.JSX.Element }) {
  return <Suspense fallback={<SkeletonPage />}>{children}</Suspense>;
}

function RequireAuth({ children, role }: { children: React.JSX.Element; role?: string }) {
  const { user } = useAuth();
  if (!user) return <Navigate to="/auth/login" replace />;
  if (role && user.role !== role && user.role !== "admin") return <Navigate to="/learning/speech" replace />;
  return children;
}

export default function App() {
  return (
    <BrowserRouter basename="/web">
      <Routes>
        <Route path="/auth/login" element={<Login />} />
        <Route path="/learning" element={<RequireAuth><LearningLayout /></RequireAuth>}>
          <Route index element={<Navigate to="/learning/speech" replace />} />
          <Route path="speech" element={<Lazy><SpeechPanel /></Lazy>} />
          <Route path="writing" element={<Lazy><WritingPanel /></Lazy>} />
          <Route path="knowledge" element={<Lazy><LearningPanel /></Lazy>} />
          <Route path="review" element={<Lazy><LearningPanel mode="review" /></Lazy>} />
          <Route path="difficulty" element={<Lazy><LearningPanel mode="difficulty" /></Lazy>} />
          <Route path="content" element={<Lazy><ContentLibrary /></Lazy>} />
          <Route path="community" element={<Lazy><CommunityPage /></Lazy>} />
          <Route path="ranking" element={<Lazy><RankingPage /></Lazy>} />
        </Route>
        <Route path="/teacher" element={<RequireAuth role="teacher"><TeacherLayout /></RequireAuth>}>
          <Route index element={<Navigate to="/teacher/lesson-plan" replace />} />
          <Route path="lesson-plan" element={<Lazy><LessonPlan /></Lazy>} />
          <Route path="analytics" element={<Lazy><Analytics /></Lazy>} />
          <Route path="grading" element={<Lazy><Grading /></Lazy>} />
        </Route>
        <Route path="/profile" element={<RequireAuth><Lazy><Profile /></Lazy></RequireAuth>} />
        <Route path="/admin" element={<RequireAuth role="admin"><Lazy><AdminDashboard /></Lazy></RequireAuth>} />
        <Route path="/admin/content" element={<RequireAuth role="admin"><Lazy><ContentAdmin /></Lazy></RequireAuth>} />
        <Route path="/admin/governance" element={<RequireAuth role="admin"><Lazy><GovernanceDashboard /></Lazy></RequireAuth>} />
        <Route path="*" element={<Navigate to="/learning/speech" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
