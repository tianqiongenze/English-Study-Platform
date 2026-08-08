package com.esp.bff.handler;

import com.esp.bff.auth.JwtService;
import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.UserRegisteredEvent;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
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
        String role = Objects.toString(body.getOrDefault("role", "learner"));
        String hash = encoder.encode(password);
        try {
            jdbc.update("INSERT INTO users (role,name,email,pwd_hash) VALUES (?,?,?,?)", role, name, email, hash);
        } catch (Exception dup) {
            return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", "email exists"));
        }
        Long id = jdbc.queryForObject("SELECT id FROM users WHERE email=?", Long.class, email);
        String token = jwt.issue(id, email, name, role);
        // 新建用户即缓存(后续登录/鉴权可直读 L1/L2,免回查 TiDB)
        cacheService.put(CACHE_USER, "user:" + id,
                Map.of("id", id, "email", email, "name", name, "role", role), 300);
        // EDA:发 user.registered 事件 → learning-java 初始化 mastery(补死代码断链)
        eventPublisher.publish(EventTopics.USER_REGISTERED,
                new UserRegisteredEvent(id, email, role));
        return ResponseEntity.ok(Map.of("token", token, "user", Map.of("id", id, "email", email, "name", name, "role", role)));
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
            // 登录成功缓存用户画像(5min L2 TTL);鉴权/聚合请求可走缓存
            Map<String, Object> userView = Map.of("id", id, "email", email, "name", name, "role", role);
            cacheService.put(CACHE_USER, "user:" + id, userView, 300);
            return ResponseEntity.ok(Map.of("token", token, "user", userView));
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
        return ResponseEntity.ok(Map.of("token", token, "user", Map.of("id", anonId, "name", "微信用户", "role", "learner")));
    }
}
