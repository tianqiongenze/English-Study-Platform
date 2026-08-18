#!/usr/bin/env bash
# ESP → Obsidian 增量同步脚本(可重复执行)
#
# 把 ESP 仓库的文档/踩坑铁律/设计/测试同步到 Obsidian vault 的
#   人工智能项目/ESP平台/ 目录(上次同步路径,会话22-10 建立)。
#
# 同步内容:
#   1. docs/PROGRESS.md                         → 进度同步.md
#   2. memory/* 铁律 + archive                  → 关键坑记录.md / 上下文工程与铁律.md
#   3. docs/design/* ADR + 设计文档              → 多个聚合笔记(见 MAPPING)
#   4. docs/test/* 测试文档                     → 测试体系.md / 测试报告集.md / 人工测试用例与操作指南.md
#   5. docs/progress/archive/*                  → 进度归档集.md(索引)
#
# 增量策略:
#   - 每个源文件算 sha256;聚合笔记 frontmatter 记 source_hash。
#   - 源 hash 集合变 → 重新生成该笔记(保留 frontmatter created,更新 updated + source_hash)。
#   - 源未变 → 跳过(打印 SKIP)。Obsidian 已有的笔记不会被无意义重写。
#   - 不双向同步:Obsidian 是镜像,仓库 docs/ 是 single source of truth。
#
# 用法:
#   scripts/sync_obsidian.sh            # 增量同步(默认)
#   scripts/sync_obsidian.sh --force    # 强制全量重写(忽略 hash,所有笔记重生成)
#   scripts/sync_obsidian.sh --check    # 只检查哪些笔记需要更新,不写
#
# 触发时机:有重要更新或里程碑时运行(用户要求)。
set -euo pipefail

REPO="/Users/joelgong/claude-Projects/English-Study-Platform"
VAULT="/Users/joelgong/Library/Mobile Documents/iCloud~md~obsidian/Documents/JoelGong"
ESP_DIR="$VAULT/人工智能项目/ESP平台"
MEMORY_DIR="/Users/joelgong/.claude/projects/-Users-joelgong-claude-Projects-English-Study-Platform/memory"

MODE="incremental"
case "${1:-}" in
  --force) MODE="force" ;;
  --check) MODE="check" ;;
  --help|-h) sed -n '2,30p' "$0"; exit 0 ;;
esac

mkdir -p "$ESP_DIR"

# ---------- 工具函数 ----------
# 算一组源文件的聚合 hash(文件路径列表 → 单个 sha256)
aggregate_hash() {
  local h=""
  for f in "$@"; do
    if [[ -f "$f" ]]; then
      local digest; digest=$(shasum -a 256 "$f" | awk '{print $1}')
      h+="${digest} ${f}"$'\n'
    fi
  done
  if [[ -z "$h" ]]; then echo "none"; else printf '%s' "$h" | shasum -a 256 | awk '{print $1}'; fi
}

# 从现有 Obsidian 笔记读 source_hash frontmatter 字段
read_source_hash() {
  local note="$1"
  [[ -f "$note" ]] || { echo "none"; return; }
  awk -F': ' '/^source_hash:/ {print $2; exit}' "$note" 2>/dev/null || echo "none"
}

# 生成笔记:参数 note_name tags_csv source_desc source_hash正文文件
# 正文文件首行应是 # 标题;本函数在前面拼 frontmatter。
write_note() {
  local name="$1" tags="$2" source="$3" new_hash="$4" body_file="$5"
  local note="$ESP_DIR/$name"
  local created
  if [[ -f "$note" ]]; then
    created=$(awk -F': ' '/^created:/ {print $2; exit}' "$note" 2>/dev/null || date +%F)
  else
    created=$(date +%F)
  fi
  local updated; updated=$(date +%F)

  if [[ "$MODE" == "check" ]]; then
    echo "WOULD-UPDATE: $name"
    return
  fi

  {
    printf -- '---\n'
    printf 'tags: [%s]\n' "$tags"
    printf 'created: %s\n' "$created"
    printf 'updated: %s\n' "$updated"
    printf 'source: %s\n' "$source"
    printf 'source_hash: %s\n' "$new_hash"
    printf -- '---\n\n'
    cat "$body_file"
  } > "$note"
  echo "UPDATED: $name"
}

# (增量判定 + 写入封装已内联到各 sync_* 函数,无需通用 sync_note)

# ---------- 各聚合笔记的同步函数 ----------
# 每个函数:列出源文件 → 算聚合 hash → 对比 Obsidian 现有 → 变则重生成正文 → write_note

sync_progress() {
  local src="$REPO/docs/PROGRESS.md"
  local new_hash; new_hash=$(aggregate_hash "$src")
  local note="$ESP_DIR/进度同步.md"
  local old_hash; old_hash=$(read_source_hash "$note")
  if [[ "$MODE" != "force" && "$new_hash" == "$old_hash" ]]; then echo "SKIP: 进度同步.md (源未变)"; return; fi

  local tmp; tmp=$(mktemp)
  {
    echo "# 进度同步"
    echo
    echo "> 本笔记是**会话级进度快照**,源 \`docs/PROGRESS.md\`(仓库内,历史归 \`docs/progress/archive/\`)。"
    echo "> 双轨落盘:PROGRESS.md(文件长期记忆)+ Redis Cluster \`esp:ctx:*\`(跨会话运行态锚点)。"
    echo "> 本笔记由 \`scripts/sync_obsidian.sh\` 自动同步;源变才更新。"
    echo
    echo "## 源文件全文(docs/PROGRESS.md)"
    echo
    echo '```markdown'
    cat "$src"
    echo '```'
    echo
    echo "关联:[[00-ESP平台总览]]、[[关键坑记录]]、[[进度归档集]]、[[如何持续同步到 Obsidian]]。"
  } > "$tmp"
  write_note "进度同步.md" "ESP, 进度, 恢复锚点, 上下文工程" \
    "docs/PROGRESS.md" "$new_hash" "$tmp"
  rm -f "$tmp"
}

sync_progress_archive() {
  local srcs=()
  while IFS= read -r f; do srcs+=("$f"); done < <(ls "$REPO"/docs/progress/archive/*.md 2>/dev/null | sort)
  local new_hash; new_hash=$(aggregate_hash "${srcs[@]}")
  local note="$ESP_DIR/进度归档集.md"
  local old_hash; old_hash=$(read_source_hash "$note")
  if [[ "$MODE" != "force" && "$new_hash" == "$old_hash" ]]; then echo "SKIP: 进度归档集.md (源未变)"; return; fi

  local tmp; tmp=$(mktemp)
  {
    echo "# 进度归档集"
    echo
    echo "> ESP 项目 \`docs/progress/archive/\` 下历史归档文件的里程碑索引。"
    echo "> 每条一行核心进展,按时间顺序组织,备查用。全文见源文件。"
    echo "> 本笔记由 \`scripts/sync_obsidian.sh\` 自动同步。"
    echo
    echo "## 归档索引(按时间顺序)"
    echo
    echo "| 归档文件 | 核心进展(首段摘要) |"
    echo "|---|---|"
    for f in "${srcs[@]}"; do
      local bn; bn=$(basename "$f")
      # 取首段非空非标题行作为摘要(grep 可能无匹配返回1,加 || true 防 set -e 退出)
      local summary; summary=$(grep -vE '^(#|>|\s*$|\|)' "$f" | head -1 | cut -c1-80 || true)
      echo "| \`$bn\` | ${summary:-（见源文件）} |"
    done
    echo
    echo "## 源文件清单"
    echo
    for f in "${srcs[@]}"; do echo "- \`$(basename "$f")\`"; done
    echo
    echo "关联:[[进度同步]] [[00-ESP平台总览]] [[关键坑记录]]。"
  } > "$tmp"
  write_note "进度归档集.md" "ESP, 进度, 归档, 里程碑" \
    "docs/progress/archive/*.md" "$new_hash" "$tmp"
  rm -f "$tmp"
}

sync_pitfalls() {
  # 关键坑记录:聚合 memory 活跃坑铁律(esp-never-*/esp-shared-infra/esp-*trap 等)+ memory/archive
  local srcs=(
    "$MEMORY_DIR/esp-shared-infra-ironrule.md"
    "$MEMORY_DIR/esp-never-stop-colima-alongside-rd.md"
    "$MEMORY_DIR/esp-never-break-k3s.md"
    "$MEMORY_DIR/esp-singlenode-host-cluster-contention.md"
    "$MEMORY_DIR/esp-pause-image-mirror-ironrule.md"
    "$MEMORY_DIR/esp-k3s-containerd-image-isolation.md"
    "$MEMORY_DIR/esp-rd-docker-ssh-port-rotation.md"
    "$MEMORY_DIR/esp-traefik-502-networkpolicy-crossns.md"
    "$MEMORY_DIR/esp-redis-cluster-hostname-announce-023.md"
    "$MEMORY_DIR/esp-redis-cluster-nodes-conf-stale-ip.md"
    "$MEMORY_DIR/esp-redis-cluster-migration.md"
    "$MEMORY_DIR/esp-loadtest-probe-pitfalls.md"
    "$MEMORY_DIR/esp-jpa-findall-oom-trap.md"
    "$MEMORY_DIR/esp-sb-cache-starter-provided-trap.md"
    "$MEMORY_DIR/esp-go-dockerfile-goproxy.md"
    "$MEMORY_DIR/esp-registry-mirror-outage-2026-08-16.md"
    "$MEMORY_DIR/esp-port-isolation-edu.md"
    "$MEMORY_DIR/esp-unified-portal-gap.md"
  )
  local archive_srcs=()
  while IFS= read -r f; do archive_srcs+=("$f"); done < <(ls "$MEMORY_DIR"/archive/*.md 2>/dev/null | sort)
  local all_srcs=("${srcs[@]}" "${archive_srcs[@]}")
  local new_hash; new_hash=$(aggregate_hash "${all_srcs[@]}")
  local note="$ESP_DIR/关键坑记录.md"
  local old_hash; old_hash=$(read_source_hash "$note")
  if [[ "$MODE" != "force" && "$new_hash" == "$old_hash" ]]; then echo "SKIP: 关键坑记录.md (源未变)"; return; fi

  local tmp; tmp=$(mktemp)
  {
    echo "# 关键坑记录"
    echo
    echo "> 按编号记录 ESP 项目踩过的关键坑(跨会话仍需牢记的:根因+修复+预防)。"
    echo "> 源:memory/\`esp-*.md\` 活跃坑铁律 + memory/archive/ 历史一次性坑。"
    echo "> 本笔记由 \`scripts/sync_obsidian.sh\` 自动同步。"
    echo
    echo "## 活跃坑铁律(跨会话牢记)"
    echo
    for f in "${srcs[@]}"; do
      echo "### $(basename "$f" .md)"
      echo
      echo "源: \`memory/$(basename "$f")\`"
      echo
      # 去掉 frontmatter,取正文(限 60 行防过长)
      awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$f" | head -60
      echo
      echo "---"
      echo
    done
    echo "## 历史一次性坑(archive,按需查阅)"
    echo
    for f in "${archive_srcs[@]}"; do
      echo "### $(basename "$f" .md)"
      echo
      echo "源: \`memory/archive/$(basename "$f")\`"
      echo
      awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$f" | head -40
      echo
      echo "---"
      echo
    done
    echo "关联:[[00-ESP平台总览]]、[[上下文工程与铁律]]、[[进度同步]]。"
  } > "$tmp"
  write_note "关键坑记录.md" "ESP, 坑, Redis, 运维, 故障复盘, 铁律" \
    "memory/esp-*.md + memory/archive/*.md" "$new_hash" "$tmp"
  rm -f "$tmp"
}

sync_context_ironrules() {
  local srcs=(
    "$MEMORY_DIR/context-overflow-ironrules.md"
    "$MEMORY_DIR/engineering-ironrules.md"
    "$MEMORY_DIR/context-auto-compact-80.md"
    "$MEMORY_DIR/esp-context-engineering.md"
    "$MEMORY_DIR/esp-session-store-redis.md"
    "$MEMORY_DIR/esp-subagent-timeout-avoid.md"
    "$MEMORY_DIR/exec-mode-bypass.md"
    "$REPO/docs/design/architecture/engineering-ironrules.md"
  )
  local new_hash; new_hash=$(aggregate_hash "${srcs[@]}")
  local note="$ESP_DIR/上下文工程与铁律.md"
  local old_hash; old_hash=$(read_source_hash "$note")
  if [[ "$MODE" != "force" && "$new_hash" == "$old_hash" ]]; then echo "SKIP: 上下文工程与铁律.md (源未变)"; return; fi

  local tmp; tmp=$(mktemp)
  {
    echo "# 上下文工程与铁律"
    echo
    echo "> ESP 项目(及所有 Claude Code 工程)的上下文防溢出 + 工程铁律。"
    echo "> 源:memory \`context-overflow-ironrules\`(唯一权威副本)+ \`engineering-ironrules\` + \`docs/design/architecture/engineering-ironrules.md\`。"
    echo "> 本笔记由 \`scripts/sync_obsidian.sh\` 自动同步。"
    echo
    echo "## 一、上下文工程铁律(防溢出)"
    echo
    echo "### context-overflow-ironrules(权威副本)"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/context-overflow-ironrules.md"
    echo '```'
    echo
    echo "### context-auto-compact-80"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/context-auto-compact-80.md"
    echo '```'
    echo
    echo "### esp-context-engineering"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/esp-context-engineering.md"
    echo '```'
    echo
    echo "### esp-session-store-redis(Redis 跨会话运行态)"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/esp-session-store-redis.md"
    echo '```'
    echo
    echo "### esp-subagent-timeout-avoid"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/esp-subagent-timeout-avoid.md"
    echo '```'
    echo
    echo "## 二、工程铁律(所有项目)"
    echo
    echo "### engineering-ironrules(memory 权威副本)"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/engineering-ironrules.md"
    echo '```'
    echo
    echo "### engineering-ironrules(项目 docs/design/architecture 副本)"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$REPO/docs/design/architecture/engineering-ironrules.md"
    echo '```'
    echo
    echo "## 三、执行模式"
    echo
    echo '```markdown'
    awk 'BEGIN{f=0} /^---$/{f++; next} f>=2' "$MEMORY_DIR/exec-mode-bypass.md"
    echo '```'
    echo
    echo "关联:[[00-ESP平台总览]]、[[关键坑记录]]、[[如何持续同步到 Obsidian]]。"
  } > "$tmp"
  write_note "上下文工程与铁律.md" "ESP, 上下文工程, 铁律, 单节点, 共享基础设施" \
    "memory/{context-overflow-ironrules,engineering-ironrules,context-auto-compact-80,esp-context-engineering,esp-session-store-redis,esp-subagent-timeout-avoid,exec-mode-bypass}.md + docs/design/architecture/engineering-ironrules.md" \
    "$new_hash" "$tmp"
  rm -f "$tmp"
}

# 通用:把单个源文件镜像成一个笔记(源→笔记同名映射,正文=源全文)
mirror_source() {
  local src="$1" note_name="$2" tags="$3" title="$4" intro="$5"
  local new_hash; new_hash=$(aggregate_hash "$src")
  local note="$ESP_DIR/$note_name"
  local old_hash; old_hash=$(read_source_hash "$note")
  if [[ "$MODE" != "force" && "$new_hash" == "$old_hash" ]]; then echo "SKIP: $note_name (源未变)"; return; fi

  local tmp; tmp=$(mktemp)
  {
    echo "# $title"
    echo
    echo "> $intro"
    echo "> 源:\`$src\`。本笔记由 \`scripts/sync_obsidian.sh\` 自动同步。"
    echo
    echo '```markdown'
    cat "$src"
    echo '```'
  } > "$tmp"
  write_note "$note_name" "$tags" "$src" "$new_hash" "$tmp"
  rm -f "$tmp"
}

# 通用:把多个源文件聚合成一个笔记(每源一段)
aggregate_sources() {
  local note_name="$1" tags="$2" title="$3" intro="$4" source_desc="$5"
  shift 5
  local srcs=("$@")
  local new_hash; new_hash=$(aggregate_hash "${srcs[@]}")
  local note="$ESP_DIR/$note_name"
  local old_hash; old_hash=$(read_source_hash "$note")
  if [[ "$MODE" != "force" && "$new_hash" == "$old_hash" ]]; then echo "SKIP: $note_name (源未变)"; return; fi

  local tmp; tmp=$(mktemp)
  {
    echo "# $title"
    echo
    echo "> $intro"
    echo "> 本笔记由 \`scripts/sync_obsidian.sh\` 自动同步;源变才更新。"
    echo
    for f in "${srcs[@]}"; do
      if [[ -f "$f" ]]; then
        echo "## $(basename "$f")"
        echo
        echo "源:\`$f\`"
        echo
        echo '```markdown'
        cat "$f"
        echo '```'
        echo
      fi
    done
  } > "$tmp"
  write_note "$note_name" "$tags" "$source_desc" "$new_hash" "$tmp"
  rm -f "$tmp"
}

sync_design() {
  # docs/design/ 全部 ADR + 设计文档,按主题分组镜像/聚合
  local D="$REPO/docs/design"

  # 单文件镜像(大文档,独立笔记)
  mirror_source "$D/01-需求分析文档-SRS.md" "需求与产品-SRS.md" "ESP, 需求, SRS" \
    "需求分析文档 SRS" "ESP 项目需求分析文档(源 \`docs/design/01-需求分析文档-SRS.md\`)。"
  mirror_source "$D/02-系统架构设计文档-SAD.md" "系统架构设计-SAD.md" "ESP, 架构, SAD, TOGAF" \
    "系统架构设计文档 SAD" "ESP 项目系统架构设计文档(源 \`docs/design/02-系统架构设计文档-SAD.md\`)。"
  mirror_source "$D/03-详细设计文档-SDD.md" "详细设计SDD.md" "ESP, SDD, 详细设计" \
    "详细设计文档 SDD" "ESP 项目详细设计文档(源 \`docs/design/03-详细设计文档-SDD.md\`)。"
  mirror_source "$D/架构评分与重构方向.md" "架构评分与重构方向.md" "ESP, 架构, 评分, TOGAF" \
    "架构评分与重构方向" "ESP 项目架构评分与重构方向(源 \`docs/design/架构评分与重构方向.md\`)。"
  mirror_source "$D/platform-infra-multitenant.md" "平台底座与多租户隔离.md" "ESP, 平台底座, 多租户, 基础设施" \
    "平台底座与多租户隔离" "ESP 平台底座多租户隔离方案(源 \`docs/design/platform-infra-multitenant.md\`)。"
  mirror_source "$D/redis-cluster-migration-plan.md" "Redis Cluster 与 warm-up.md" "ESP, Redis, 缓存, warm-up" \
    "Redis Cluster 迁移与 warm-up" "Redis Cluster 迁移方案(源 \`docs/design/redis-cluster-migration-plan.md\`)。"

  # ADR 聚合
  aggregate_sources "架构决策记录ADR.md" "ESP, ADR, 架构决策" \
    "架构决策记录(ADR)" "ESP 项目关键架构决策(ADR),按主题归并。" \
    "docs/design/adr-*.md" \
    "$D/adr-db-logical-sharding.md" \
    "$D/adr-gitops-argocd.md" \
    "$D/adr-new-services.md" \
    "$D/adr-schema-per-service.md" \
    "$D/adr-seata-at-to-saga.md" \
    "$D/adr-security-scanning.md" \
    "$D/adr-writing-coach.md"

  # 多区域 / Pulsar
  aggregate_sources "事件驱动与Pulsar设计.md" "ESP, Pulsar, 事件驱动, 多区域" \
    "事件驱动与 Pulsar 设计" "Pulsar 事件驱动 + 多区域同步方案。" \
    "docs/design/pulsar-geo-replication-plan.md + 多区域数据同步方案.md" \
    "$D/pulsar-geo-replication-plan.md" \
    "$D/多区域数据同步方案.md"

  # 缓存与可观测性
  aggregate_sources "缓存与可观测性设计.md" "ESP, 缓存, 可观测性, OpenTelemetry, Redis" \
    "缓存与可观测性设计" "多级缓存 + 日志系统架构。" \
    "docs/design/多级缓存架构对齐.md + 日志系统架构.md" \
    "$D/多级缓存架构对齐.md" \
    "$D/日志系统架构.md"

  # 监控与可观测性部署
  aggregate_sources "监控与可观测性部署.md" "ESP, 监控, 可观测性, OTel, Prometheus" \
    "监控与可观测性部署" "OTel Collector 分层 + traceId 链路。" \
    "docs/design/p2-4-otel-collector-tiering.md + p1-5-traceid-map.md + p1-6-cachekey-userid-map.md" \
    "$D/p2-4-otel-collector-tiering.md" \
    "$D/p1-5-traceid-map.md" \
    "$D/p1-6-cachekey-userid-map.md"

  # 事务安全与 GitOps
  aggregate_sources "事务安全与GitOps.md" "ESP, Seata, Saga, 安全扫描, GitOps, ArgoCD" \
    "事务安全与 GitOps" "Seata→Saga 演进 + GitOps + 安全扫描。" \
    "docs/design/adr-seata-at-to-saga.md + adr-gitops-argocd.md + adr-security-scanning.md" \
    "$D/adr-seata-at-to-saga.md" \
    "$D/adr-gitops-argocd.md" \
    "$D/adr-security-scanning.md"

  # 新服务引入
  aggregate_sources "新服务引入设计.md" "ESP, 新服务, ADR, PRD, 服务矩阵" \
    "新服务引入设计" "新服务 ADR + PRD + 各服务 SDD/SRS。" \
    "docs/design/adr-new-services.md + prd-new-services.md + sdd-*/srs-*" \
    "$D/adr-new-services.md" \
    "$D/prd-new-services.md" \
    "$D/sdd-vocab-srs-go.md" "$D/sdd-reading-go.md" "$D/sdd-listening-rs.md" \
    "$D/sdd-pronunciation-rs.md" "$D/sdd-assessment-rs.md" "$D/sdd-gamification-go.md" \
    "$D/sdd-llm-orchestrator-rs.md" \
    "$D/srs-vocab-srs-go.md" "$D/srs-reading-go.md" "$D/srs-listening-rs.md" \
    "$D/srs-pronunciation-rs.md" "$D/srs-assessment-rs.md" "$D/srs-gamification-go.md" \
    "$D/srs-llm-orchestrator-rs.md" "$D/srs-writing-coach.md"

  # 数据模型与 Schema
  aggregate_sources "数据模型与Schema.md" "ESP, 数据模型, Schema, ER图" \
    "数据模型与 Schema" "ER 图 + schema-per-service + SQL。" \
    "docs/design/schema*.sql + schema-per-service-phase* + architecture/er-diagrams.md" \
    "$D/schema.sql" \
    "$D/schema-per-service-phase0.sql" \
    "$D/schema-per-service-phase1.sql" \
    "$D/schema-per-service-phase1-plan.md" \
    "$D/crdb-schema-aux-services.sql" \
    "$REPO/docs/design/architecture/er-diagrams.md"

  # 种子数据与压测脚本
  aggregate_sources "种子数据与压测脚本.md" "ESP, 种子数据, 压测, Locust, 混沌" \
    "种子数据与压测脚本" "分布式 Locust + learning-worker 拆分 + 种子脚本。" \
    "docs/design/p2-*.md + scripts/seed_3b.py" \
    "$D/p2-2-learning-worker-split.md" \
    "$D/p2-3-distributed-locust.md" \
    "$D/p2-4-otel-collector-tiering.md" \
    "$REPO/scripts/seed_3b.py"

  # DB 访问与吞吐优化 + 多节点容量
  aggregate_sources "DB访问与多节点容量.md" "ESP, DB, 吞吐, 多节点, 容量" \
    "DB 访问吞吐优化与多节点容量验证" "DB 访问吞吐优化 + 多节点容量验证 + CockroachDB 对比。" \
    "docs/design/db-access-throughput-optimization.md + multi-node-capacity-validation-plan.md + cockroachdb-vs-tidb-multi-region-plan.md" \
    "$D/db-access-throughput-optimization.md" \
    "$D/multi-node-capacity-validation-plan.md" \
    "$D/cockroachdb-vs-tidb-multi-region-plan.md"

  # 审计与缺口
  aggregate_sources "审计与缺口登记.md" "ESP, 审计, 缺口, 技术债" \
    "审计缺口登记与技术债" "审计缺口解决登记 + 技术债登记 + 共享代码审计。" \
    "docs/design/audit-gap-resolution-register.md + architecture/tech-debt-register.md + architecture/shared-code-audit-2026-08-15.md" \
    "$D/audit-gap-resolution-register.md" \
    "$REPO/docs/design/architecture/tech-debt-register.md" \
    "$REPO/docs/design/architecture/shared-code-audit-2026-08-15.md"

  # 架构治理 TOGAF(architecture 子目录聚合)
  aggregate_sources "架构治理与TOGAF.md" "ESP, TOGAF, 架构治理, 架构板" \
    "架构治理与 TOGAF" "TOGAF 评估 + 架构板章程 + 干系人关注 + 价值流 + 业务能力 + 业务事件 + 应用数据矩阵。" \
    "docs/design/architecture/{togaf-assessment,architecture-board-charter,stakeholder-concerns,value-streams,business-capability-map,business-event-catalog,app-data-matrix,optimization-plan,multi-node-topology,adr-index}.md" \
    "$REPO/docs/design/architecture/togaf-assessment.md" \
    "$REPO/docs/design/architecture/architecture-board-charter.md" \
    "$REPO/docs/design/architecture/stakeholder-concerns.md" \
    "$REPO/docs/design/architecture/value-streams.md" \
    "$REPO/docs/design/architecture/business-capability-map.md" \
    "$REPO/docs/design/architecture/business-event-catalog.md" \
    "$REPO/docs/design/architecture/app-data-matrix.md" \
    "$REPO/docs/design/architecture/optimization-plan.md" \
    "$REPO/docs/design/architecture/multi-node-topology.md" \
    "$REPO/docs/design/architecture/adr-index.md"

  # SDD assessment 补充
  aggregate_sources "SDD补充评估.md" "ESP, SDD, 评估" \
    "SDD 补充评估" "SDD 评估 + 听力/发音 SRS。" \
    "docs/design/sdd-assessment-rs.md + srs-listening-rs.md + srs-pronunciation-rs.md + sdd-listening-rs.md + sdd-pronunciation-rs.md" \
    "$D/sdd-assessment-rs.md" \
    "$D/srs-assessment-rs.md" \
    "$D/sdd-listening-rs.md" "$D/sdd-pronunciation-rs.md" \
    "$D/srs-listening-rs.md" "$D/srs-pronunciation-rs.md"
}

sync_test() {
  local T="$REPO/docs/test"
  # 测试体系(00 + 01-04)
  aggregate_sources "测试体系.md" "ESP, 测试, 压测, 单元, 集成" \
    "测试体系" "测试总览 + 单元/集成/性能/混沌测试报告。" \
    "docs/test/00 + 01-04" \
    "$T/00-测试总览.md" \
    "$T/01-单元测试报告.md" \
    "$T/02-集成测试报告.md" \
    "$T/03-性能与压测报告.md" \
    "$T/04-混沌测试报告.md" \
    "$T/chaos-plan.md"

  # 测试报告集(05-08)
  aggregate_sources "测试报告集.md" "ESP, 测试, 报告, trace, 安全, 前端" \
    "测试报告集" "监控告警 + traceId 实测 + 安全测试 + 前端核查。" \
    "docs/test/05-08" \
    "$T/05-监控告警部署说明.md" \
    "$T/05-traceId实测.md" \
    "$T/06-安全测试报告.md" \
    "$T/08-前端核查报告.md"

  # 人工测试用例与操作指南(07 + 09)
  aggregate_sources "人工测试用例与操作指南.md" "ESP, 测试, 人工测试, 用例, 操作指南" \
    "人工测试用例与操作指南" "人工测试用例 + 操作指南。" \
    "docs/test/07 + 09" \
    "$T/07-人工测试用例.md" \
    "$T/09-人工测试操作指南.md"
}

sync_overview_index() {
  # 00-ESP平台总览.md 是手维护索引,这里只刷新"笔记清单"段(保留用户手写部分)
  local src="$REPO/docs/PROGRESS.md"
  local new_hash; new_hash=$(aggregate_hash "$src" "$ESP_DIR")
  local note="$ESP_DIR/00-ESP平台总览.md"
  # 总览不强制 hash(手维护),仅 --force 时刷新笔记清单
  if [[ "$MODE" != "force" ]]; then echo "SKIP: 00-ESP平台总览.md (手维护索引,仅 --force 刷新清单)"; return; fi
  echo "NOTE: 00-ESP平台总览.md 为手维护索引,请手动核对笔记清单。当前 ESP平台/ 目录笔记:"
  ls -1 "$ESP_DIR"/*.md 2>/dev/null | xargs -n1 basename | sed 's/^/  - /'
}

# ---------- 主流程 ----------
echo "=========================================="
echo "ESP → Obsidian 增量同步"
echo "  仓库: $REPO"
echo "  Vault: $ESP_DIR"
echo "  模式: $MODE"
echo "=========================================="
echo

echo "[1/7] 进度同步 (PROGRESS.md)"
sync_progress
echo
echo "[2/7] 进度归档集"
sync_progress_archive
echo
echo "[3/7] 关键坑记录 (memory 铁律)"
sync_pitfalls
echo
echo "[4/7] 上下文工程与铁律"
sync_context_ironrules
echo
echo "[5/7] 设计文档 (docs/design/*)"
sync_design
echo
echo "[6/7] 测试文档 (docs/test/*)"
sync_test
echo
echo "[7/7] 总览索引"
sync_overview_index
echo

echo "=========================================="
echo "同步完成。当前 ESP平台/ 目录笔记数:"
ls -1 "$ESP_DIR"/*.md 2>/dev/null | wc -l | tr -d ' '
echo "=========================================="
