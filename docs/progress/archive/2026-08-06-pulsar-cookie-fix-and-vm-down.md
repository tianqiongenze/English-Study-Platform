# Pulsar Bookie Cookie 根因 E 彻底闭合 + VM 网络面中断(2026-08-06)

## 主题
pulsar-bk-0/1 cookie instanceId 不一致(根因 E 复发)→ 3 bookie 修复全 Running → broker 自愈途中遇 Rancher Desktop VM 网络面挂掉中断。

## 集群状态(VM 挂前最后观测 16:30)
- pulsar-bk 3/3 全 Running(bk cookie 根治)
- pulsar-broker-0/1 1/1 Ready(restart 停 84/82 已稳定);broker-2 起到一半 VM 挂
- pulsar-zk 3/3、tidb Ready、redis-cluster-new 6/6
- esp-prod 10 deploy 全 1/1

## 根因
bk-0/bk-1 本地 cookie instanceId `2311bbae-d401-46e9-b76d-d0b4bedbe163` ≠ ZK `/ledgers/INSTANCEID` = `f97edb1d-64a6-4d0c-ab17-cc4811fc9adf`(本地旧、ZK 新)。
- ZK 元数据在某次重建后 instanceId 变为 f97edb1d,但 bk-0/bk-1 本地 VERSION cookie 未同步更新。
- bk-2 正常(instanceId 一致)。

## 修复实操(3 步,已执行成功)

### Step 1:重写本地 VERSION cookie
- `kubectl scale sts pulsar-bk --replicas=0` 释放 PVC。
- 部署临时 pod `bk-pvc-cleaner`(busybox:1.36,挂 6 个 PVC:bk-data/journal-pulsar-bk-{0,1})。
- 写 bk-0/bk-1 的 `ledgers/current/VERSION` + `journal/current/VERSION`,内容:
  ```
  5
  bookieHost: "pulsar-bk-N.pulsar-bk:3181"
  journalDir: "data/bookkeeper/journal"
  ledgerDirs: "1\tdata/bookkeeper/ledgers"
  instanceId: "f97edb1d-64a6-4d0c-ab17-cc4811fc9adf"
  ```
- chown 10000:0,清 DIRTY 标记。
- **踩坑**:第一次只删 VERSION 不重写 → bookie 报 `Not all the new directories are empty`(LegacyCookieValidation.verifyDirsForNewEnvironment,旧 ledger 数据还在但无 cookie 触发空目录校验)→ **必须重写正确值,不能只删除**。

### Step 2:写 ZK cookie 节点
- bk-0/bk-1 本地 VERSION 校验过后,卡在 `Cookie for this bookie is not stored in metadata store`(ZK `/ledgers/cookies/` 只有 bk-2)。
- **失败尝试**:
  - `bookkeeper shell cookie_create -b <host> -cf <file>` → 异常退出(参数语法问题)。
  - `pulsar zookeeper-shell -c 'create <path> <data> world:anyone:cdrwa'` → `InvalidACL: does not have the form scheme:id:perm`(shell 的 ACL 解析有 bug,多行 data 也破坏 shell 解析)。
- **正确方法**:在 `pulsar-zk-0` pod 内用 python3 + `kazoo` 库(kazoo 2.10.0 已预装):
  ```python
  from kazoo.client import KazooClient
  from kazoo.security import make_acl
  zk = KazooClient(hosts='localhost:2181', timeout=30); zk.start(timeout=30)
  acl = [make_acl('world', 'anyone', all=True)]
  zk.create('/ledgers/cookies/pulsar-bk-0.pulsar-bk:3181', data, acl=acl, makepath=True)
  ```
- **误操作**:中途在 bk-2 跑 `bookkeeper shell bookieformat -deleteCookie` 误删了 bk-2 的 ZK cookie。bk-2 重启后靠本地 VERSION(还在且匹配)自愈重写 ZK cookie,**无数据丢失**。教训:bookieformat -deleteCookie 会删 ZK cookie,慎用。

### Step 3:scale bk→3 + 删 pod 触发重启
- `kubectl scale sts pulsar-bk --replicas=3` → bk-0/bk-1 处于 CrashLoop back-off,需 `kubectl delete pod pulsar-bk-0 pulsar-bk-1 --force --grace-period=0` 触发立即重启拉新 ZK cookie。
- 3 bookie 全 1/1 Running,ZK cookies = `[bk-0, bk-1, bk-2]` 三者齐全。
- `kubectl rollout restart sts pulsar-broker` → broker-0/1 1/1 Ready(restart 计数停止增长)。

## 关键文件/路径
- ZK instanceId:`/ledgers/INSTANCEID`
- ZK cookie:`/ledgers/cookies/<bookieHost>:3181`
- 本地 VERSION:`<ledgerDir>/current/VERSION` + `<journalDir>/current/VERSION`
- bk sts env:`PULSAR_PREFIX_bookkeeperMetadataServiceUri=zk+null://pulsar-zk-0..:2181/ledgers`、`advertisedAddress=pulsar-bk-N.pulsar-bk`、`allowStorageExpansion=true`
- bk volumeMounts:`bk-data`→`/pulsar/data/bookkeeper`、`bk-journal`→`/pulsar/data/bookkeeper/journal`
- bk PVC:bk-data-pulsar-bk-N(2Gi)、bk-journal-pulsar-bk-N(1Gi),local-path

## 根因 E 防复发要点
instanceId 变化(ZK 元数据重建)后,所有 bookie 的本地 VERSION + ZK cookie 必须同步更新:
1. 先确认 ZK 当前 instanceId(`get /ledgers/INSTANCEID`)。
2. 重写所有 bookie 本地 VERSION(用 ZK 当前 instanceId),不能只删不写。
3. ZK cookie 节点缺失的,用 kazoo make_acl 写入(shell ACL 有 bug)。
4. scale bk→3 + force delete pod 触发重启。
5. 等 3 bookie Running + ZK cookies 齐全 → rollout restart broker。

## 中断:Rancher Desktop VM 网络面挂掉(16:55)
- 现象:`kubectl` 全 refused(6443 无监听)、`rdctl shell` SSH `Connection reset by peer`、qemu 低 CPU 1.5%(非负载卡死,是 guest 网络栈/SSH daemon 挂)。
- **非 k3s 配置问题**,是 VM guest 网络面异常。
- 已执行恢复:`rdctl shutdown`(powerdown qemu 成功)→ `open -a "Rancher Desktop"` → `rdctl start --kubernetes.enabled=true`。
- 状态(17:11):limactl hostagent 在跑,VM 仍 Stopped,qemu 未稳定拉起。等待 app 冷启动完成。
- 单节点铁律遵守:未 Factory Reset、未 pkill、未动 k3s 配置。

## 恢复后下一步(VM 起来后)
1. 验证 pulsar-broker-0/1/2 全 Ready(broker-2 预计自愈继续拉起;若 notReady 则 `rollout restart sts pulsar-broker`)。
2. 冒烟 nacos/seata/bff/learning/teacher。
3. 补写 Redis 锚点(VM 挂期间 Redis 不可写,PROGRESS.md 已落盘本地)。
4. 推 schema Phase 0:13 新表按 7 schema 建 + esp_learning 独立 DB(用户催办)。
