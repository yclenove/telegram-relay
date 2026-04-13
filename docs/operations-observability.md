# 平台运维与可观测实践

## 监控指标

当前服务提供：

- `/healthz`：实例存活探针。
- `/metrics`：基础请求统计（总数、成功、失败）。
- `/api/v2/dashboard`：业务统计（事件、任务、机器人、规则）。

建议接入 Prometheus：

1. 对 `/metrics` 做抓取。
2. 对 `/api/v2/dashboard` 做定时采样（可通过 exporter 二次转换）。

## 告警建议

- `events_failed` 在 5 分钟内增量 > 0 时告警。
- `jobs_pending` 持续上升超过阈值时告警。
- PostgreSQL 不可达时立即告警（应用日志出现 `ping db failed`）。

## 故障演练清单

1. **Telegram 出网中断**
   - 预期：`dispatch_jobs` 进入重试，`events` 不会丢失。
2. **规则配置错误**
   - 预期：回落到默认 destination；若无默认 destination 则明确失败日志。
3. **数据库短暂抖动**
   - 预期：服务报错但不中断，恢复后继续轮询发送。
4. **高并发冲击**
   - 预期：限流生效（429），系统保持可用。

## 灰度切换建议

1. 保留 `/api/v1/notify`，先灰度接入 `/api/v2/notify`。
2. 双轨对比 sent/failed 成功率至少 24 小时。
3. 达标后将上游默认切换到 v2，再逐步下线旧链路。
