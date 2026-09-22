# 历史开发与验收归档

本目录保留各阶段的原始计划、Gate 记录、旧部署流程和视觉证据，便于追溯与回归检查。除归档说明、链接及明确的导航修正外，不改写原始基线、日期、任务状态或测试结论。

当前使用请返回[文档导航](../README.md)、[能力状态](../CURRENT_STATUS.md)和[部署手册](../guides/FULL_DEPLOYMENT_RUNBOOK_ZH.md)；未完成项目已单独汇总在[待办](../ROADMAP.md)。归档不是销项，也不是生产批准。

## 改版前设计

- [v0.7 English](design/overall-design-v0.7.md) / [中文版](design/overall-design-v0.7-zh.md)：2026-09-15 被空间改版取代；保留原状态和决策内容，链接按归档位置调整。

## 阶段索引

| 阶段 | 计划/基线 | 验收与补充材料 |
|---|---|---|
| Phase 0 | [基线清单](phase0/PHASE0_BASELINE.md) | [运行报告](phase0/PHASE0_RUNTIME_REPORT.md) |
| Phase 1 | [实施计划（含未关闭项）](phase1/PHASE1_IMPLEMENTATION_PLAN.md) | [Gate B](phase1/PHASE1_GATE_B.md)、[C](phase1/PHASE1_GATE_C.md)、[D](phase1/PHASE1_GATE_D.md)、[截图基线](phase1/PHASE1_UI_EVIDENCE.md)；Gate A 记录在实施计划中 |
| Phase 2 | [实施计划](phase2/PHASE2_IMPLEMENTATION_PLAN.md) | [Gate A](phase2/PHASE2_GATE_A.md)、[B](phase2/PHASE2_GATE_B.md)、[C](phase2/PHASE2_GATE_C.md)、[D](phase2/PHASE2_GATE_D.md) |
| Phase 3 | [实施计划](phase3/PHASE3_IMPLEMENTATION_PLAN.md) | [Gate A](phase3/PHASE3_GATE_A.md)、[B](phase3/PHASE3_GATE_B.md)、[C](phase3/PHASE3_GATE_C.md)、[D](phase3/PHASE3_GATE_D.md)、[旧 Phase 0→3 升级](phase3/PHASE0_TO_PHASE3_UPGRADE.md) |
| Phase 4 | [实施计划](phase4/PHASE4_IMPLEMENTATION_PLAN.md) | [Gate A](phase4/PHASE4_GATE_A.md)、[B](phase4/PHASE4_GATE_B.md)、[C](phase4/PHASE4_GATE_C.md)、[D](phase4/PHASE4_GATE_D.md)、[旧运行/升级指南](phase4/PHASE4_OPERATIONS.md) |
| Phase 5 | [实施计划](phase5/PHASE5_IMPLEMENTATION_PLAN.md) | [Gate A](phase5/PHASE5_GATE_A.md)、[B](phase5/PHASE5_GATE_B.md)、[C](phase5/PHASE5_GATE_C.md)、[D](phase5/PHASE5_GATE_D.md) |

## 图像与使用限制

- Phase 1 的四张截图随[截图说明](phase1/PHASE1_UI_EVIDENCE.md)保留，不能作为当前界面的新验收证据。
- 旧架构图：[早期图稿](architecture/internal-kb-architecture.png)、[v1 图稿](architecture/internal-kb-architecture-v1.png)。[保留的 v0.4 图](../assets/internal-kb-architecture-v0.4.png)属于旧设计；新版总体设计使用 Mermaid，不以旧图表示新空间架构。
- 旧文档中的 `make phase0-*`、`phase3-*`、`phase4-*` 用于解释历史环境，不是新部署的推荐顺序。尤其不能将旧回滚建议套用于未经验证的新上游数据库版本。
- 静态验收脚本仍引用这些记录；保留原任务 ID 和证据结构，以便历史契约检查继续运行。
