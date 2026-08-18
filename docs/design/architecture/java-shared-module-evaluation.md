# Java 公共包下沉评估(esp-common-java)

> Phase3 架构重构评估产物。结论:**暂不创建 esp-common-java 模块**。

## 1. 现状摸底

### 1.1 JWT 校验

| 服务 | 类 | 职责 | 实现方式 |
|------|-----|------|----------|
| bff-java | `JwtService` | 签发 + 校验 + 撤销 + JWKS | RS256 私钥签发,公钥校验,依赖 esp-cache 做 jti 撤销缓存 |
| learning-java | `JwtValidator` | 仅校验(公钥) | RS256 公钥 PEM 直校验,`@Value` 注入公钥 |
| teacher-java | `JwtValidator` | 仅校验(公钥) | 与 learning-java 几乎相同(RS256 公钥 PEM 直校验) |

**代码重复分析**:
- learning-java 和 teacher-java 的 `JwtValidator` 逻辑高度相似(解析公钥 + `parseClaimsJws`),但各自包名不同(`com.esp.learning.auth` vs `com.esp.teacher.config`),且有细微差异(teacher 的 `enabled()` 检查 `isBlank()`,learning 检查 `== null || isBlank()`).
- bff-java 的 `JwtService` 是完整的签发+校验+撤销+缓存,与下游校验器职责完全不同,下沉意义不大.
- 学习端/教师端的 JwtValidator 如果要共享,需要提取为 `esp-jwt-validator-spring-boot-starter`,包含 `JwtValidator` bean + `JwtAuthFilter` 自动装配.

### 1.2 Sentinel 规则

- bff-java: 通过 Nacos `dataId: bff-java-flow-rules` 动态下发,代码层面无规则定义.
- learning-java: 同上,`dataId: learning-java-flow-rules`.
- teacher-java: 同上,`dataId: teacher-java-flow-rules`.
- **结论**: Sentinel 规则零代码重复,全走 Nacos 热更新,无需下沉.

### 1.3 CRDB 双池数据源

- 已下沉到 `backend/shared/esp-rw-java/` 作为独立 Maven 模块.
- 3 个 Java 服务通过 Spring Boot starter 自动装配.
- **结论**: 已完成下沉,无需再动.

### 1.4 健康检查配置

- 各服务通过 `application.yml` 配置 `management.endpoints.web.exposure.include=health,info,prometheus`.
- 属于 Spring Boot 标准配置,无代码级重复.

## 2. 评估结论:暂不建 esp-common-java

### 理由

1. **代码重复量实际有限**: JWT 签发仅 bff 有,其他服务只是公钥验签引用.两个 JwtValidator 相似但有细微差异,强行统一会引入 starter 装订复杂度.

2. **Sentinel 规则零代码重复**:全部通过 Nacos dataId 动态下发,代码层面无规则定义可下沉.

3. **核心共享能力已被三套包覆盖**:
   - `esp-web-common`: GlobalExceptionHandler + TraceId
   - `esp-rw-java`: CRDB 读写分离双池
   - `esp-cache`: 缓存 + 幂等 + 事件发布

4. **强行下沉的代价**:
   - 需修改 3 个服务的 `pom.xml` 加依赖
   - 需要设计 `esp-jwt-validator-spring-boot-starter` 的自动装配(条件注解 + 属性绑定)
   - teacher-java 和 learning-java 的 `JwtAuthFilter` 也需要统一,改动面大
   - 当前只有 3 个 Java 服务,共享收益不足以覆盖重构成本

5. **多节点扩展时再评估**:当 Java 服务数 >5 时(如新增 gamification-java, notification-java 等),统一 auth 模块的收益才显著.

### 建议的未来时机

- 新增第 4+ 个需要 JWT 验签的 Java 服务时
- 或者现有 JwtValidator 需要增强(如添加 JWKS 动态获取、token 刷新等)时
- 届时创建 `esp-auth-java` 模块,包含:
  - `JwtValidator` (公钥验签)
  - `JwtAuthFilter` (Servlet Filter 自动装配)
  - 可选: `JwtService` (签发,仅 bff 引入)

## 3. 已有共享模块清单(不动)

| 模块 | 路径 | 职责 |
|------|------|------|
| esp-rw-java | `backend/shared/esp-rw-java/` | CRDB 读写分离双池 |
| esp-web-common | `platform/esp-web-common/` | GlobalExceptionHandler + TraceId |
| esp-cache | `platform/esp-cache/` | 缓存 + 幂等 + 事件发布 |

---

*评估日期: 2026-08-17*
*评估人: Phase3 架构重构 Agent*
