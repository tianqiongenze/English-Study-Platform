package com.esp.bff.graphql;

import graphql.GraphqlErrorException;
import graphql.ExecutionInput;
import graphql.GraphQLContext;
import graphql.schema.DataFetchingEnvironment;
import graphql.schema.idl.SchemaDirectiveWiring;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.graphql.execution.RuntimeWiringConfigurer;
import org.springframework.graphql.server.WebGraphQlInterceptor;
import org.springframework.graphql.server.WebGraphQlRequest;

/**
 * Spring for GraphQL 配置(Phase1 Task C,schema-first 试点,2026-08-21)。
 *
 * <p>职责:
 * <ul>
 *   <li>{@code @auth} directive wiring —— 标记需登录的 Query 字段,运行期校验
 *       GraphQLContext 中存在 userId(由 {@link GraphQLContextInterceptor} 从
 *       AuthFilter 注入的 X-User-Id header 取 JWT subject 写入 context)。</li>
 *   <li>注册 {@link AvatarDataLoader} 防排行榜 N+1(批量取用户头像)。</li>
 * </ul>
 *
 * <p>鉴权链路(与 REST 一致,ADR-12):
 * <pre>
 *   浏览器 → /graphql (8090 Spring MVC)
 *          → AuthFilter (BaseJwtAuthFilter): 校验 esp_token/Bearer → 注入 X-User-Id/X-Role
 *          → GraphQLContextInterceptor: 把 X-User-Id 写入 GraphQLContext
 *          → @QueryMapping fetcher 执行,@auth directive 校验 context.userId 非空
 * </pre>
 * userId 取自 JWT subject(非客户端 GraphQL 参数),防越权伪造 —— 与 EdgeAuthHandler 同源。
 *
 * <p>ASV(2026-08-21 WebFetch 核实 spring-graphql 2.0.4 API):
 * <ul>
 *   <li>{@code RuntimeWiringConfigurer#configure(RuntimeWiring.Builder)} 注册 directiveWiring + typeWiring</li>
 *   <li>{@code SchemaDirectiveWiring.onField(SchemaElementFieldEnvironment)} 返回 GraphQLFieldDefinition</li>
 *   <li>{@code DataFetchingEnvironment#getGraphQlContext()} 返回 GraphQLContext(Map-like)</li>
 *   <li>{@code BatchLoaderRegistry} 通过 {@code RuntimeWiringConfigurer} 的 {@code configure(BatchLoaderRegistry)}
 *       重载注册(本试点用 {@code @BatchMapping} 注解替代,见 AvatarDataLoader)</li>
 * </ul>
 *
 * <p>CLC(nil-safe):directive wiring 对 context.get("userId") 判空,缺 userId 时抛
 * GraphqlErrorException(403 语义),不返回 null 掩盖未授权(安全铁律:鉴权失败必拒绝不静默放行)。
 */
@Configuration
public class GraphQLConfig {

    private static final Logger log = LoggerFactory.getLogger(GraphQLConfig.class);

    /** GraphQLContext key:当前登录用户 ID(由 interceptor 从 X-User-Id header 写入)。 */
    public static final String CTX_USER_ID = "userId";
    public static final String CTX_ROLE = "role";

    /**
     * @auth directive 运行期 wiring:校验 GraphQLContext.userId 非空。
     *
     * <p>实现 SchemaDirectiveWiring.onField —— 对每个标记 @auth 的 FIELD_DEFINITION,
     * 包装原 DataFetcher:执行前校验 context.userId,空则抛 GraphqlErrorException。
     *
     * <p>ASV(javap 核实 graphql-java 25.0):
     * {@code SchemaDirectiveWiringEnvironment.getFieldDataFetcher()} 取原 fetcher,
     * {@code setFieldDataFetcher(DataFetcher)} 替换为包装后 fetcher,onField 返回
     * {@code env.getElement()}(被改写后的 GraphQLFieldDefinition)。
     */
    @Bean
    public SchemaDirectiveWiring authDirectiveWiring() {
        // ASV(javap 核实):SchemaDirectiveWiring 全是 default 方法,非 functional interface,
        // 不能用 lambda —— 显式匿名类只重写 onField。
        return new SchemaDirectiveWiring() {
            @Override
            public graphql.schema.GraphQLFieldDefinition onField(
                    graphql.schema.idl.SchemaDirectiveWiringEnvironment<graphql.schema.GraphQLFieldDefinition> fieldEnvironment) {
                graphql.schema.DataFetcher<?> originalFetcher = fieldEnvironment.getFieldDataFetcher();
                graphql.schema.DataFetcher<?> guardedFetcher = (DataFetchingEnvironment env) -> {
                    Object userId = env.getGraphQlContext().get(CTX_USER_ID);
                    if (userId == null || userId.toString().isBlank()) {
                        log.debug("[graphql @auth] rejected: no userId in context (field={})",
                                fieldEnvironment.getFieldDefinition().getName());
                        throw GraphqlErrorException.newErrorException()
                                .message("unauthorized: authentication required")
                                .build();
                    }
                    return originalFetcher.get(env);
                };
                fieldEnvironment.setFieldDataFetcher(guardedFetcher);
                return fieldEnvironment.getElement();
            }
        };
    }

    /**
     * RuntimeWiringConfigurer:按 directive 名注册 @auth wiring。
     *
     * <p>ASV(javap 核实):{@code RuntimeWiring.Builder.directive(String, SchemaDirectiveWiring)}
     * 按 directive 名注册(比无参 directiveWiring 更精确,只对 @auth 触发)。
     * {@code @BatchMapping} 注解的 AvatarDataLoader 由 AnnotatedControllerConfigurer
     * 自动注册,不需在此手动加 typeWiring。
     */
    @Bean
    public RuntimeWiringConfigurer graphQlRuntimeWiringConfigurer() {
        return (builder) -> builder.directive("auth", authDirectiveWiring());
    }

    /**
     * GraphQL 查询深度 + 复杂度护栏(RWT + 大厂级 GraphQL 治理)。
     *
     * <p>防恶意嵌套查询(如 entries{avatar{...}} 递归)与资源耗尽攻击(超大 query)。
     * ASV(javap 核实 spring-graphql 2.0.4 / spring-boot-graphql 4.0.7):
     * {@code GraphQlSourceBuilderCustomizer#customize(GraphQlSource.SchemaResourceBuilder)},
     * SchemaResourceBuilder 继承 {@code Builder.instrumentation(List<Instrumentation>)};
     * graphql-java 25.0 {@code MaxQueryDepthInstrumentation(int)} +
     * {@code MaxQueryComplexityInstrumentation(int)} 构造器取 int 阈值。
     *
     * <p>阈值:depth=10(试点 schema 最大嵌套 RankingResult.entries.avatar 仅 2 层,10 足够防御);
     * complexity=100(试点 5 Query 字段,单查询复杂度低,100 防滥用)。
     */
    @Bean
    public org.springframework.boot.graphql.autoconfigure.GraphQlSourceBuilderCustomizer
            graphQlDepthComplexityCustomizer() {
        return (builder) -> builder.instrumentation(java.util.List.of(
                new graphql.analysis.MaxQueryDepthInstrumentation(10),
                new graphql.analysis.MaxQueryComplexityInstrumentation(100)
        ));
    }

    /**
     * GraphQLContext 写入拦截器:把 AuthFilter 注入的 X-User-Id/X-Role header 写入
     * GraphQLContext,供 @auth directive 与 fetcher 读取。
     *
     * <p>ASV(2026-08-21 javap 核实 spring-graphql 2.0.4 / graphql-java 25.0):
     * <ul>
     *   <li>{@code WebGraphQlRequest.getHeaders()} → {@code HttpHeaders}(继承自 DefaultExecutionGraphQlRequest)</li>
     *   <li>{@code configureExecutionInput(BiFunction<ExecutionInput, Builder, ExecutionInput>)} 继承自父类,
     *       在 BiFunction 内 {@code executionInput.getGraphQLContext().put(k,v)} 写入 context</li>
     *   <li>{@code graphql.GraphQLContext.put(Object,Object)} 返回 GraphQLContext(Map-like,核实过)</li>
     * </ul>
     */
    @Bean
    public WebGraphQlInterceptor graphQlContextInterceptor() {
        return (webRequest, chain) -> {
            // 从 HTTP header 取 AuthFilter 注入的可信 X-User-Id(已校验 JWT)
            String userId = webRequest.getHeaders().getFirst("X-User-Id");
            String role = webRequest.getHeaders().getFirst("X-Role");
            // 经 configureExecutionInput 把 userId/role 写入 GraphQLContext(执行期 fetcher 可读)
            webRequest.configureExecutionInput((executionInput, builder) -> {
                GraphQLContext ctx = executionInput.getGraphQLContext();
                ctx.put(CTX_USER_ID, userId);
                ctx.put(CTX_ROLE, role);
                return builder.build();
            });
            return chain.next(webRequest);
        };
    }
}
