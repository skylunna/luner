# Contributing to ai-gateway

感谢你对 `ai-gateway` 的贡献！我们遵循 Go 社区标准与 [Conventional Commits](https://www.conventionalcommits.org/)。

## 🛠️ 本地开发
```bash
make run      # 启动网关
make test     # 运行测试（含 race detection）
make lint     # 静态检查
make build    # 编译二进制
```

## 📝 提交规范
- ```feat```: 新功能
- ```fix```: Bug 修复
- ```docs```: 文档更新
- ```refactor```: 代码重构（无功能变更）
- ```test```: 测试相关

## 🐛 提交 Issue / PR
- 使用模板提交
- 关联相关 Issue
- 保持 PR 原子化（一个 PR 解决一个问题）
- 更新 ```docs/ROADMAP.md``` 若涉及长期规划


#### `ROADMAP.md`
```markdown
# 🗺️ Roadmap

## v0.3.0 (计划中)
- [ ] 本地 LRU 缓存（支持 `prompt+model+temp=0` 命中）
- [ ] 滑动窗口限流器（Token/分钟维度）
- [ ] 流式响应 `usage` 解析（SSE `data: [DONE]` 触发）

## v0.4.0
- [ ] 健康检查感知降级（自动剔除超时 Provider）
- [ ] K8s Helm Chart 与 `ServiceMonitor` 模板
- [ ] 插件化中间件系统（Request/Response Hook）

## v1.0.0
- [ ] 语义化版本稳定承诺
- [ ] 官方文档站（MkDocs / Hugo）
- [ ] 生产级压测基准报告

> 💡 欢迎提交 `RFC` 或认领 `good first issue` 参与共建。
```