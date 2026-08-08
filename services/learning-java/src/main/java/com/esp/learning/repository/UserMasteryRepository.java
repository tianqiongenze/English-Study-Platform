package com.esp.learning.repository;

import com.esp.learning.domain.UserMastery;
import com.esp.learning.domain.UserMasteryId;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.Instant;
import java.util.List;

public interface UserMasteryRepository extends JpaRepository<UserMastery, UserMasteryId> {

    /** 到期复习清单:next_review_at <= now(读副本)。 */
    @Query("select m from UserMastery m where m.userId = :uid and m.nextReviewAt <= :now order by m.nextReviewAt asc")
    List<UserMastery> findDueReview(@Param("uid") Long userId, @Param("now") Instant now);

    /** 薄弱知识点:按掌握度升序(读副本)。 */
    @Query("select m from UserMastery m where m.userId = :uid order by m.masteryScore asc")
    List<UserMastery> findWeakest(@Param("uid") Long userId);
}
