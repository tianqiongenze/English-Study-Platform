package com.esp.teacher.repository;

import com.esp.teacher.domain.Submission;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.util.List;

public interface SubmissionRepository extends JpaRepository<Submission, Long> {

    /** 按 assignment 取所有提交(读副本)。 */
    List<Submission> findByAssignmentId(Long assignmentId);

    /** 共性难题:按 class 聚合 auto_score 均值最低的作业(走 TiFlash OLAP)。 */
    @Query(value = "select /*+ read_from_storage(tiflash[submissions,assignments]) */ " +
            "a.id, a.content, avg(s.auto_score) as avg_score, count(s.id) as cnt " +
            "from submissions s join assignments a on s.assignment_id = a.id " +
            "where a.class_id = :classId " +
            "group by a.id, a.content order by avg_score asc", nativeQuery = true)
    List<Object[]> classWeakness(@Param("classId") Long classId);
}
