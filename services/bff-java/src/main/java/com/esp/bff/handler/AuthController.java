package com.esp.bff.handler;

import com.esp.bff.auth.JwtService;
import com.esp.bff.auth.TokenRevokedException;
import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.UserRegisteredEvent;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.web.bind.annotation.*;

import java.util.Map;
import java.util.Objects;

/**
 * 认证端点(/api/auth/**)。BFF 私钥签发 JWT,下游公钥(共享 secret)校验。
 * - POST /api/auth/register {email,password,name,role} → 建用户 + 签 token
 * - POST /api/auth/login    {email,password}            → 校验 + 签 token
 * - POST /api/auth/wx-login {code}                       → 小程序 wx.login 换 JWT(本期用 code 作匿名身份)
 *
 * 多级缓存(esp-cache):login 成功后把 user 信息缓存(L1 Caffeine + L2 Redis),
 * 供后续边缘请求快速取用户画像,避免每次回查 TiDB。register 改写用户 → 失效该用户缓存。
 */
@RestController
@RequestMapping("/api/auth")
public class AuthController {

    private static final String CACHE_USER = "bff_user";

    private final JdbcTemplate jdbc;
    private final JwtService jwt;
    private final CacheService cacheService;
    private final EventPublisher eventPublisher;
    private final BCryptPasswordEncoder encoder = new BCryptPasswordEncoder();

    @Value("${wx.login.enabled:false}")
    private boolean wxLoginEnabled;

    public AuthController(JdbcTemplate jdbc, JwtService jwt, CacheService cacheService, EventPublisher eventPublisher) {
        this.jdbc = jdbc;
        this.jwt = jwt;
        this.cacheService = cacheService;
        this.eventPublisher = eventPublisher;
    }

    @PostMapping("/register")
    @DataSource(DataSourceType.WRITE)
    public ResponseEntity<?> register(@RequestBody Map<String, Object> body) {
        // 入参校验:email/password 必填且非空字符串。
        // 修复:String.valueOf(null)→"null" 字符串穿透 SQL,会写入 email='null' 脏行并被 login 命中。
        String email = Objects.toString(body.get("email"), null);
        String password = Objects.toString(body.get("password"), null);
        if (email == null || email.isBlank() || password == null || password.isBlank()) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST).body(Map.of("error", "email and password required"));
        }
        String name = Objects.toString(body.get("name"), email.split("@")[0]);
        // P0-4 权限提升修复:服务端强制 learner,忽略请求体 role。teacher/admin 仅后台运营开通,用户不可自注册高权角色。
        String role = "learner";
        String hash = encoder.encode(password);
        try {
            jdbc.update("INSERT INTO users (role,name,email,pwd_hash) VALUES (?,?,?,?)", role, name, email, hash);
        } catch (Exception dup) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "email exists"));
        }
        Long id = jdbc.queryForObject("SELECT id FROM users WHERE email=?", Long.class, email);
        String token = jwt.issue(id, email, name, role);
        String refresh = jwt.issueRefresh(id, email, name, role);   // P1-9:注册即发 refresh(与 login 对齐)
        // 新建用户即缓存(后续登录/鉴权可直读 L1/L2,免回查 TiDB)
        cacheService.put(CACHE_USER, "user:" + id,
                Map.of("id", id, "email", email, "name", name, "role", role), 300);
        // EDA:发 user.registered 事件 → learning-java 初始化 mastery(补死代码断链)
        eventPublisher.publish(EventTopics.USER_REGISTERED,
                new UserRegisteredEvent(id, email, role));
        return ResponseEntity.ok(Map.of(
                "token", token,
                "refreshToken", refresh,
                "expiresIn", jwt.getAccessTtlSeconds(),
                "user", Map.of("id", id, "email", email, "name", name, "role", role)));
    }

    @PostMapping("/login")
    @DataSource(value = DataSourceType.READ, consistent = true)
    public ResponseEntity<?> login(@RequestBody Map<String, Object> body) {
        // 入参校验:email/password 必填。
        // 修复:String.valueOf(null)→"null" 穿透 SQL,命中种子 email='null' 脏行 → 误返 200+token。
        String email = Objects.toString(body.get("email"), null);
        String pwd = Objects.toString(body.get("password"), null);
        if (email == null || email.isBlank() || pwd == null || pwd.isBlank()) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST).body(Map.of("error", "email and password required"));
        }
        try {
            Map<String, Object> row = jdbc.queryForMap("SELECT id,name,role,pwd_hash FROM users WHERE email=?", email);
            if (!encoder.matches(pwd, String.valueOf(row.get("pwd_hash")))) {
                return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of("error", "bad credentials"));
            }
            long id = ((Number) row.get("id")).longValue();
            String name = String.valueOf(row.get("name"));
            String role = String.valueOf(row.get("role"));
            String token = jwt.issue(id, email, name, role);
            String refresh = jwt.issueRefresh(id, email, name, role);
            // 登录成功缓存用户画像(5min L2 TTL);鉴权/聚合请求可走缓存
            Map<String, Object> userView = Map.of("id", id, "email", email, "name", name, "role", role);
            cacheService.put(CACHE_USER, "user:" + id, userView, 300);
            return ResponseEntity.ok(Map.of(
                    "token", token,
                    "refreshToken", refresh,
                    "expiresIn", jwt.getAccessTtlSeconds(),
                    "user", userView));
        } catch (Exception e) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of("error", "bad credentials"));
        }
    }

    /** 小程序 wx.login code 换 JWT(ADR-14)。生产调微信 jscode2session;本期接受 code 作匿名身份占位。 */
    @PostMapping("/wx-login")
    public ResponseEntity<?> wxLogin(@RequestBody Map<String, Object> body) {
        String code = String.valueOf(body.getOrDefault("code", ""));
        if (!wxLoginEnabled && code.isBlank()) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST).body(Map.of("error", "code required"));
        }
        long anonId = Math.abs(("wx" + code).hashCode()) % 1_000_000L + 1L;
        String token = jwt.issue(anonId, "wx_" + anonId, "微信用户", "learner");
        String refresh = jwt.issueRefresh(anonId, "wx_" + anonId, "微信用户", "learner");
        return ResponseEntity.ok(Map.of(
                "token", token,
                "refreshToken", refresh,
                "expiresIn", jwt.getAccessTtlSeconds(),
                "user", Map.of("id", anonId, "name", "微信用户", "role", "learner")));
    }

    /**
     * P1-9 刷新 access token:用 refresh token 换新 access(免重新登录)。
     * 旧 access 会被拉黑(jti 黑名单),实现一次性刷新轮换(refresh rotation)防重放。
     */
    @PostMapping("/refresh")
    public ResponseEntity<?> refresh(@RequestBody Map<String, Object> body) {
        String refreshToken = Objects.toString(body.get("refreshToken"), null);
        if (refreshToken == null || refreshToken.isBlank()) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST).body(Map.of("error", "refreshToken required"));
        }
        try {
            Claims claims = jwt.parse(refreshToken);
            if (!"refresh".equals(claims.get("typ"))) {
                return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of("error", "not a refresh token"));
            }
            long userId = Long.parseLong(claims.getSubject());
            String email = String.valueOf(claims.get("email"));
            String name = String.valueOf(claims.get("name"));
            String role = String.valueOf(claims.get("role"));
            // 拉黑旧 refresh(一次性,防重放)
            jwt.revoke(claims);
            String newAccess = jwt.issue(userId, email, name, role);
            String newRefresh = jwt.issueRefresh(userId, email, name, role);
            return ResponseEntity.ok(Map.of(
                    "token", newAccess,
                    "refreshToken", newRefresh,
                    "expiresIn", jwt.getAccessTtlSeconds()));
        } catch (TokenRevokedException e) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of("error", "refresh token revoked"));
        } catch (JwtException e) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(Map.of("error", "invalid refresh token"));
        }
    }

    /**
     * P1-9 注销:拉黑当前 access token 的 jti,立即生效(原方案 logout 后旧 token 仍可用 1h)。
     * 客户端调用后应同时丢弃本地 token。
     */
    @PostMapping("/logout")
    public ResponseEntity<?> logout(@RequestHeader(value = "Authorization", required = false) String auth) {
        if (auth == null || !auth.startsWith("Bearer ")) {
            return ResponseEntity.status(HttpStatus.BAD_REQUEST).body(Map.of("error", "Authorization header required"));
        }
        try {
            Claims claims = jwt.parse(auth.substring(7));
            jwt.revoke(claims);
            return ResponseEntity.ok(Map.of("status", "logged out"));
        } catch (Exception e) {
            // token 已无效也算注销成功(幂等)
            return ResponseEntity.ok(Map.of("status", "logged out"));
        }
    }
}
