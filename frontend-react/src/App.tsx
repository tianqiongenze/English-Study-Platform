import React from "react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import LearningLayout from "./pages/LearningLayout";
import SpeechPanel from "./panels/SpeechPanel";
import WritingPanel from "./panels/WritingPanel";
import LearningPanel from "./panels/LearningPanel";
import TeacherLayout from "./pages/TeacherLayout";
import LessonPlan from "./pages/teacher/LessonPlan";
import Analytics from "./pages/teacher/Analytics";
import Grading from "./pages/teacher/Grading";
import Profile from "./pages/Profile";

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
          <Route path="speech" element={<SpeechPanel />} />
          <Route path="writing" element={<WritingPanel />} />
          <Route path="knowledge" element={<LearningPanel />} />
          <Route path="review" element={<LearningPanel mode="review" />} />
          <Route path="difficulty" element={<LearningPanel mode="difficulty" />} />
        </Route>
        <Route path="/teacher" element={<RequireAuth role="teacher"><TeacherLayout /></RequireAuth>}>
          <Route index element={<Navigate to="/teacher/lesson-plan" replace />} />
          <Route path="lesson-plan" element={<LessonPlan />} />
          <Route path="analytics" element={<Analytics />} />
          <Route path="grading" element={<Grading />} />
        </Route>
        <Route path="/profile" element={<RequireAuth><Profile /></RequireAuth>} />
        <Route path="*" element={<Navigate to="/learning/speech" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
