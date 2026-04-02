# my-monitor

监控 API 网关（New-API / One-API）的请求质量，检测 TTFT 超长和失败率过高时通过飞书 Webhook 告警。

**检查频率：**
- 工作日 10:00–22:00：每 5 分钟检查一次
- 其余时间：每小时检查一次

---

## 快速启动

```bash
BASE_URL=https://your-api-host \
API_KEY=your-api-key \
FEISHU_WEBHOOK=https://open.feishu.cn/... \
docker-compose up -d
```

---

## 配置参数

配置文件为 `config.yaml`，环境变量优先级高于配置文件。

| 参数 | 环境变量 | 默认值 | 说明 |
|------|----------|--------|------|
| `base_url` | `BASE_URL` | — | API 网关地址，如 `https://your-api-host` |
| `api_key` | `API_KEY` | — | 管理员 API Key（**必填**） |
| `feishu_webhook` | `FEISHU_WEBHOOK` | — | 飞书机器人 Webhook URL，为空则不发送告警 |
| `window_minutes` | — | `5` | 每次检查的统计时间窗口（分钟） |
| `ttft_threshold_secs` | — | `10.0` | TTFT 慢请求阈值（秒），超过则计入慢请求 |
| `ttft_alert_percent` | — | `30.0` | 慢 TTFT 占流式请求的比例超过此值时触发告警（%） |
| `failure_rate_percent` | — | `10.0` | 失败率超过此值时触发告警（%），失败定义为 quota == 0 |
| `min_requests` | — | `5` | 窗口内请求数低于此值时不触发告警，避免样本过少误报 |

### 示例 config.yaml

```yaml
base_url: "https://your-api-host"
api_key: "sk-xxxx"
feishu_webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/xxxx"

window_minutes: 5
ttft_threshold_secs: 10.0
ttft_alert_percent: 30.0
failure_rate_percent: 10.0
min_requests: 5
```
