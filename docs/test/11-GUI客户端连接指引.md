# GUI 客户端连接指引(DBeaver EE + RedisInsight)

> 2026-08-17 建立。目标:从 mac 本机用 GUI 查看集群内 CRDB / Redis Cluster 数据。
> 铁律十二 WTV:连接参数以「当前部署清单/运行态」为准,不再有 TiDB(已彻底下电,2026-08-17)。

## 0. 前提

- Rancher Desktop VM(runner `lima-rancher-desktop`)运行,kubectl context 正常。
- 已应用 `deploy/k8s/46-db-nodeports.yaml` 暴露本地入口:
  - CRDB PG `127.0.0.1:32627`(HTTP UI `127.0.0.1:32628`)
  - Redis `127.0.0.1:30201`(仅 TCP 可达性参考,集群浏览走 RedisInsight 隧道)

## 1. CRDB(CockroachDB) —— DBeaver EE

新建 PostgreSQL 连接(driver: PostgreSQL / CockroachDB):

| 项 | 值 |
|---|---|
| Host | `127.0.0.1` |
| Port | `32627` |
| Database | `esp`(其余库:`esp_learning` `esp_reading` `esp_pronunciation` `esp_listening` `esp_assessment` `esp_llm` `esp_vocab` `esp_content` `esp_community` `esp_shared` `esp_teacher`) |
| User | `root` |
| Password | 空 |
| SSL | disable |

要点:
- 库 `esp` 下 schema:`public`(search_path 默认,含 users/业务表)与 `esp_auth`(认证表,`esp_auth.users` 1M 行)。
- 用户查询带前缀:`SELECT * FROM public.users LIMIT 3`(等价裸 `users`)。

## 2. Redis Cluster —— RedisInsight(brew 已装)

### 2.1 隧道前提(唯一配置点)

Redis 各节点 `cluster-announce-hostname` = pod FQDN,mac 无法解析直达。方案:**RedisInsight SSH 隧道 → RD-VM → 集群内网**。VM 的 `/etc/hosts` 已注入 FQDN→pod IP 映射(见 `scripts/redis-vm-hosts.sh`,pod IP 变化后重跑即可)。

### 2.2 新建连接

1. 打开 RedisInsight(Applications)。
2. 左下 **Add Redis Database** → 展开 **Set up from scratch**。
3. 连接方式选 **SSH tunnel**:
   | 项 | 值 |
   |---|---|
   | Tunnel host | `127.0.0.1` |
   | Tunnel port | `59195`(**轮转**:VM 重启后查 `~/.ssh/config` 的 `Host rd-vm-127` Port;或 `LIMA_HOME=$HOME/.lima limactl list` 看当前端口) |
   | Tunnel user | `joelgong` |
   | Auth | Private key → `~/Library/Application Support/rancher-desktop/lima/_config/user` |
4. 数据库地址:
   | 项 | 值 |
   |---|---|
   | Host | `redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local` |
   | Port | `6379` |
   | Password | 空 |
5. Test connection 通过后保存。RedisInsight 自动识别 Cluster(6 节点),可浏览全部键。

### 2.3 备选(仅数据抽查)

```bash
ssh rd-vm-127 'getent hosts redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local'
```

## 3. 排障

- CRDB 32627 不通:`kubectl -n esp-crdb get svc db-crdb-np`;VM 重启后重新 `bash scripts/redis-vm-hosts.sh`。
- RedisInsight 连接失败:校验 SSH 端口(轮转)、key 文件权限(600)。
- TiDB 一律不连(已下电)。`scripts/db_query.sh` 默认 CRDB。