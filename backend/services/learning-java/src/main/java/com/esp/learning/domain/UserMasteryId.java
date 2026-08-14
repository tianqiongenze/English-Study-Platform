package com.esp.learning.domain;

import java.io.Serializable;
import java.util.Objects;

public class UserMasteryId implements Serializable {
    private Long userId;
    private Long kpId;

    public UserMasteryId() {}
    public UserMasteryId(Long userId, Long kpId) { this.userId = userId; this.kpId = kpId; }

    public Long getUserId() { return userId; }
    public void setUserId(Long userId) { this.userId = userId; }
    public Long getKpId() { return kpId; }
    public void setKpId(Long kpId) { this.kpId = kpId; }

    @Override public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof UserMasteryId that)) return false;
        return Objects.equals(userId, that.userId) && Objects.equals(kpId, that.kpId);
    }
    @Override public int hashCode() { return Objects.hash(userId, kpId); }
}
